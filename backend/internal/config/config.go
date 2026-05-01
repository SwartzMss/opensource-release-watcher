package config

import (
	"os"
	"path/filepath"
	"time"
)

type GraphMailConfig struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	AccessToken  string
	RefreshToken string
}

type Config struct {
	ServerAddr    string
	DBPath        string
	GitHubToken   string
	CheckInterval time.Duration
	Auth          AuthConfig
	GraphMail     GraphMailConfig
}

type AuthConfig struct {
	Username string
	Password string
	Secret   string
}

func Load() Config {
	adminUsername := env("ADMIN_USERNAME", "admin")
	adminPassword := env("ADMIN_PASSWORD", "admin")
	dbPath := env("DB_PATH", "../data/watcher.db")
	if abs, err := filepath.Abs(dbPath); err == nil {
		dbPath = abs
	}
	return Config{
		ServerAddr:    env("SERVER_ADDR", ":8080"),
		DBPath:        dbPath,
		GitHubToken:   os.Getenv("GITHUB_TOKEN"),
		CheckInterval: durationEnv("CHECK_INTERVAL", 6*time.Hour),
		Auth: AuthConfig{
			Username: adminUsername,
			Password: adminPassword,
			Secret:   env("SESSION_SECRET", adminUsername+":"+adminPassword),
		},
		GraphMail: GraphMailConfig{
			TenantID:     os.Getenv("GRAPH_TENANT_ID"),
			ClientID:     os.Getenv("GRAPH_CLIENT_ID"),
			ClientSecret: os.Getenv("GRAPH_CLIENT_SECRET"),
			AccessToken:  os.Getenv("GRAPH_ACCESS_TOKEN"),
			RefreshToken: os.Getenv("GRAPH_REFRESH_TOKEN"),
		},
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
