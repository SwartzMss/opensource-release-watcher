package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"opensource-release-watcher/backend/internal/api"
	"opensource-release-watcher/backend/internal/checker"
	"opensource-release-watcher/backend/internal/config"
	"opensource-release-watcher/backend/internal/github"
	"opensource-release-watcher/backend/internal/gitrepo"
	"opensource-release-watcher/backend/internal/notifier"
	"opensource-release-watcher/backend/internal/osv"
	"opensource-release-watcher/backend/internal/scheduler"
	"opensource-release-watcher/backend/internal/security"
	"opensource-release-watcher/backend/internal/service"
	"opensource-release-watcher/backend/internal/storage"
)

func main() {
	cfg := config.Load()
	if cwd, err := os.Getwd(); err == nil {
		log.Printf("starting opensource-release-watcher server addr=%s db=%s check_interval=%s cwd=%s", cfg.ServerAddr, cfg.DBPath, cfg.CheckInterval, cwd)
	} else {
		log.Printf("starting opensource-release-watcher server addr=%s db=%s check_interval=%s cwd=unknown", cfg.ServerAddr, cfg.DBPath, cfg.CheckInterval)
	}
	if cfg.GitHubToken == "" {
		log.Printf("github token not configured; GitHub API requests will use anonymous rate limits")
	}

	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open storage: %v", err)
	}
	defer store.Close()

	githubClient := github.NewClient(cfg.GitHubToken)
	gitrepoResolver := gitrepo.New(githubClient)
	osvClient := osv.NewClient()
	securityChecker := security.New(gitrepoResolver, osvClient)
	var mailNotifier notifier.Notifier
	switch cfg.MailProvider {
	case "smtp", "exchange":
		log.Printf("mail provider configured provider=%s host=%s port=%d starttls=%t", cfg.MailProvider, cfg.SMTPMail.Host, cfg.SMTPMail.Port, cfg.SMTPMail.StartTLS)
		mailNotifier = notifier.NewSMTPMail(cfg.SMTPMail)
	case "graph":
		log.Printf("mail provider configured provider=graph")
		mailNotifier = notifier.NewGraphDelegatedMail(cfg.GraphMail)
	default:
		log.Printf("mail provider %q is not supported; falling back to graph", cfg.MailProvider)
		mailNotifier = notifier.NewGraphDelegatedMail(cfg.GraphMail)
	}
	releaseChecker := checker.New(githubClient)
	watcherService := service.New(
		store,
		releaseChecker,
		securityChecker,
		mailNotifier,
		cfg.CheckInterval,
		cfg.GitHubToken,
		os.Getenv("HTTP_PROXY"),
		os.Getenv("HTTPS_PROXY"),
		os.Getenv("NO_PROXY"),
	)
	scheduler.New(watcherService, cfg.CheckInterval).Start(context.Background())

	router := api.NewRouter(watcherService, cfg.Auth)
	handler := api.WithStaticFiles(router, cfg.StaticDir)
	if cfg.StaticDir != "" {
		log.Printf("serving frontend static files from %s", cfg.StaticDir)
	}
	log.Printf("opensource-release-watcher listening on %s", cfg.ServerAddr)
	if err := http.ListenAndServe(cfg.ServerAddr, handler); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
