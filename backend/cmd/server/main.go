package main

import (
	"context"
	"log"
	"net/http"

	"opensource-release-watcher/backend/internal/api"
	"opensource-release-watcher/backend/internal/checker"
	"opensource-release-watcher/backend/internal/config"
	"opensource-release-watcher/backend/internal/github"
	"opensource-release-watcher/backend/internal/notifier"
	"opensource-release-watcher/backend/internal/scheduler"
	"opensource-release-watcher/backend/internal/service"
	"opensource-release-watcher/backend/internal/storage"
)

func main() {
	cfg := config.Load()

	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open storage: %v", err)
	}
	defer store.Close()

	githubClient := github.NewClient(cfg.GitHubToken)
	mailNotifier := notifier.NewGraphDelegatedMail(cfg.GraphMail)
	releaseChecker := checker.New(githubClient)
	watcherService := service.New(store, releaseChecker, mailNotifier)
	scheduler.New(watcherService, cfg.CheckInterval).Start(context.Background())

	router := api.NewRouter(watcherService, cfg.Auth)
	log.Printf("opensource-release-watcher listening on %s", cfg.ServerAddr)
	if err := http.ListenAndServe(cfg.ServerAddr, router); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
