package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func WithStaticFiles(next http.Handler, staticDir string) http.Handler {
	staticDir = strings.TrimSpace(staticDir)
	if staticDir == "" {
		return next
	}
	staticRoot, err := filepath.Abs(staticDir)
	if err != nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") || req.URL.Path == "/healthz" || (req.Method != http.MethodGet && req.Method != http.MethodHead) {
			next.ServeHTTP(w, req)
			return
		}
		cleanPath := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(req.URL.Path, "/")))
		if cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		if cleanPath == "." {
			cleanPath = "index.html"
		}
		target := filepath.Join(staticRoot, cleanPath)
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			http.ServeFile(w, req, target)
			return
		}
		http.ServeFile(w, req, filepath.Join(staticRoot, "index.html"))
	})
}
