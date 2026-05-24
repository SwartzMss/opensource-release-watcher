package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsStaticDir(t *testing.T) {
	t.Setenv("STATIC_DIR", "./frontend/dist")
	t.Setenv("DB_PATH", "./test.db")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "password")
	t.Setenv("SESSION_SECRET", "secret")

	cfg := Load()

	want, err := filepath.Abs("./frontend/dist")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StaticDir != want {
		t.Fatalf("StaticDir = %q, want %q", cfg.StaticDir, want)
	}
}

func TestLoadResolvesStaticDirWhenStartedFromReleaseBin(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	staticDir := filepath.Join(root, "frontend", "dist")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(binDir)
	t.Setenv("STATIC_DIR", "./frontend/dist")
	t.Setenv("DB_PATH", "./test.db")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "password")
	t.Setenv("SESSION_SECRET", "secret")

	cfg := Load()

	if cfg.StaticDir != staticDir {
		t.Fatalf("StaticDir = %q, want %q", cfg.StaticDir, staticDir)
	}
}

func TestLoadReadsSMTPMailConfig(t *testing.T) {
	t.Setenv("DB_PATH", "./test.db")
	t.Setenv("MAIL_PROVIDER", "smtp")
	t.Setenv("SMTP_HOST", "exchange-relay.internal")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_USERNAME", "watcher")
	t.Setenv("SMTP_PASSWORD", "secret")
	t.Setenv("SMTP_FROM", "watcher@example.com")
	t.Setenv("SMTP_STARTTLS", "true")

	cfg := Load()

	if cfg.MailProvider != "smtp" {
		t.Fatalf("MailProvider = %q, want smtp", cfg.MailProvider)
	}
	if cfg.SMTPMail.Host != "exchange-relay.internal" {
		t.Fatalf("SMTPMail.Host = %q", cfg.SMTPMail.Host)
	}
	if cfg.SMTPMail.Port != 2525 {
		t.Fatalf("SMTPMail.Port = %d, want 2525", cfg.SMTPMail.Port)
	}
	if cfg.SMTPMail.Username != "watcher" {
		t.Fatalf("SMTPMail.Username = %q", cfg.SMTPMail.Username)
	}
	if cfg.SMTPMail.Password != "secret" {
		t.Fatalf("SMTPMail.Password = %q", cfg.SMTPMail.Password)
	}
	if cfg.SMTPMail.From != "watcher@example.com" {
		t.Fatalf("SMTPMail.From = %q", cfg.SMTPMail.From)
	}
	if !cfg.SMTPMail.StartTLS {
		t.Fatal("SMTPMail.StartTLS = false, want true")
	}
}
