package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithStaticFilesServesAssetsAndFallsBackToIndex(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<div id=\"root\"></div>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(staticDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := WithStaticFiles(http.NotFoundHandler(), staticDir)

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if asset.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", asset.Code, http.StatusOK)
	}
	if body := asset.Body.String(); !strings.Contains(body, "console.log('ok')") {
		t.Fatalf("asset body = %q", body)
	}

	spaRoute := httptest.NewRecorder()
	handler.ServeHTTP(spaRoute, httptest.NewRequest(http.MethodGet, "/components/42", nil))
	if spaRoute.Code != http.StatusOK {
		t.Fatalf("spa route status = %d, want %d", spaRoute.Code, http.StatusOK)
	}
	if body := spaRoute.Body.String(); !strings.Contains(body, "root") {
		t.Fatalf("spa route body = %q", body)
	}
}

func TestWithStaticFilesLeavesAPIAndHealthRoutesToBackend(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-Backend", "yes")
		w.WriteHeader(http.StatusTeapot)
	})
	handler := WithStaticFiles(backend, staticDir)

	for _, path := range []string{"/api/components", "/healthz"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusTeapot {
			t.Fatalf("%s status = %d, want %d", path, recorder.Code, http.StatusTeapot)
		}
		if got := recorder.Header().Get("X-Backend"); got != "yes" {
			t.Fatalf("%s X-Backend = %q, want yes", path, got)
		}
	}
}

func TestWithStaticFilesRejectsPathTraversal(t *testing.T) {
	rootDir := t.TempDir()
	staticDir := filepath.Join(rootDir, "dist")
	if err := os.Mkdir(staticDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := WithStaticFiles(http.NotFoundHandler(), staticDir)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/../secret.txt", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("response leaked file content: %q", recorder.Body.String())
	}
}
