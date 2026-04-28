package main

import (
	"context"
	"log"
	"net/http"

	"opensource-release-watcher/server/internal/api"
	"opensource-release-watcher/server/internal/checker"
	"opensource-release-watcher/server/internal/config"
	"opensource-release-watcher/server/internal/github"
	"opensource-release-watcher/server/internal/notifier"
	"opensource-release-watcher/server/internal/scheduler"
	"opensource-release-watcher/server/internal/service"
	"opensource-release-watcher/server/internal/storage"
)

func main() {
	cfg := config.Load()

	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open storage: %v", err)
	}
	defer store.Close()

	githubClient := github.NewClient(cfg.GitHubToken)
	mailer := notifier.NewSMTP(cfg.SMTP)
	releaseChecker := checker.New(githubClient)
	watcherService := service.New(store, releaseChecker, mailer)
	scheduler.New(watcherService, cfg.CheckInterval).Start(context.Background())

	router := api.NewRouter(watcherService)
	log.Printf("opensource-release-watcher listening on %s", cfg.ServerAddr)
	if err := http.ListenAndServe(cfg.ServerAddr, router); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
