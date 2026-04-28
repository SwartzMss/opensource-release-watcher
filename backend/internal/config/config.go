package config

import (
	"os"
	"strconv"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type Config struct {
	ServerAddr    string
	DBPath        string
	GitHubToken   string
	CheckInterval time.Duration
	Auth          AuthConfig
	SMTP          SMTPConfig
}

type AuthConfig struct {
	Username string
	Password string
	Secret   string
}

func Load() Config {
	adminUsername := env("ADMIN_USERNAME", "admin")
	adminPassword := env("ADMIN_PASSWORD", "admin")
	return Config{
		ServerAddr:    env("SERVER_ADDR", ":8080"),
		DBPath:        env("DB_PATH", "../data/watcher.db"),
		GitHubToken:   os.Getenv("GITHUB_TOKEN"),
		CheckInterval: durationEnv("CHECK_INTERVAL", 6*time.Hour),
		Auth: AuthConfig{
			Username: adminUsername,
			Password: adminPassword,
			Secret:   env("SESSION_SECRET", adminUsername+":"+adminPassword),
		},
		SMTP: SMTPConfig{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     intEnv("SMTP_PORT", 587),
			Username: os.Getenv("SMTP_USERNAME"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     os.Getenv("SMTP_FROM"),
		},
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
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
