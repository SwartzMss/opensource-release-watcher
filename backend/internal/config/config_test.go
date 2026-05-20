package config

import (
	"testing"
)

func TestLoadReadsStaticDir(t *testing.T) {
	t.Setenv("STATIC_DIR", "./frontend/dist")
	t.Setenv("DB_PATH", "./test.db")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "password")
	t.Setenv("SESSION_SECRET", "secret")

	cfg := Load()

	if cfg.StaticDir != "./frontend/dist" {
		t.Fatalf("StaticDir = %q, want %q", cfg.StaticDir, "./frontend/dist")
	}
}
