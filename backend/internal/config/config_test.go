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
