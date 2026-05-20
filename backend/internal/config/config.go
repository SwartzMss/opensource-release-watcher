package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
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
	StaticDir     string
	Auth          AuthConfig
	GraphMail     GraphMailConfig
}

type AuthConfig struct {
	Username    string
	Password    string
	Secret      string
	IdleTimeout time.Duration
}

func Load() Config {
	loadDotEnvFiles()
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
		StaticDir:     os.Getenv("STATIC_DIR"),
		Auth: AuthConfig{
			Username:    adminUsername,
			Password:    adminPassword,
			Secret:      env("SESSION_SECRET", adminUsername+":"+adminPassword),
			IdleTimeout: durationEnv("SESSION_IDLE_TIMEOUT", 10*time.Minute),
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

func loadDotEnvFiles() {
	seen := map[string]struct{}{}
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for {
		candidate := filepath.Join(dir, ".env")
		if _, ok := seen[candidate]; !ok {
			seen[candidate] = struct{}{}
			_ = loadDotEnvFile(candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func loadDotEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if len(value) >= 2 {
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) || (strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
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
