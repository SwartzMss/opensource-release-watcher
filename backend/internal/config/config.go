package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
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

type SMTPMailConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	StartTLS bool
}

type Config struct {
	ServerAddr    string
	DBPath        string
	GitHubToken   string
	CheckInterval time.Duration
	StaticDir     string
	Auth          AuthConfig
	MailProvider  string
	GraphMail     GraphMailConfig
	SMTPMail      SMTPMailConfig
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
		StaticDir:     resolveStaticDir(os.Getenv("STATIC_DIR")),
		Auth: AuthConfig{
			Username:    adminUsername,
			Password:    adminPassword,
			Secret:      env("SESSION_SECRET", adminUsername+":"+adminPassword),
			IdleTimeout: durationEnv("SESSION_IDLE_TIMEOUT", 10*time.Minute),
		},
		MailProvider: strings.ToLower(env("MAIL_PROVIDER", "graph")),
		GraphMail: GraphMailConfig{
			TenantID:     os.Getenv("GRAPH_TENANT_ID"),
			ClientID:     os.Getenv("GRAPH_CLIENT_ID"),
			ClientSecret: os.Getenv("GRAPH_CLIENT_SECRET"),
			AccessToken:  os.Getenv("GRAPH_ACCESS_TOKEN"),
			RefreshToken: os.Getenv("GRAPH_REFRESH_TOKEN"),
		},
		SMTPMail: SMTPMailConfig{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     intEnv("SMTP_PORT", 25),
			Username: os.Getenv("SMTP_USERNAME"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     os.Getenv("SMTP_FROM"),
			StartTLS: boolEnv("SMTP_STARTTLS", false),
		},
	}
}

func resolveStaticDir(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	candidates := make([]string, 0, 4)
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, value))
		if strings.EqualFold(filepath.Base(cwd), "bin") {
			candidates = append(candidates, filepath.Join(filepath.Dir(cwd), value))
		}
	}
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(executableDir, value))
		if strings.EqualFold(filepath.Base(executableDir), "bin") {
			candidates = append(candidates, filepath.Join(filepath.Dir(executableDir), value))
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return filepath.Clean(candidate)
		}
	}
	if len(candidates) > 0 {
		if abs, err := filepath.Abs(candidates[0]); err == nil {
			return abs
		}
		return filepath.Clean(candidates[0])
	}
	return value
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

func boolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
