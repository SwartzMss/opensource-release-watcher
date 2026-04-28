package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"opensource-release-watcher/backend/internal/config"
	"opensource-release-watcher/backend/internal/service"
	"opensource-release-watcher/backend/internal/storage"
)

type Router struct {
	service *service.Service
	mux     *http.ServeMux
	auth    config.AuthConfig
}

const sessionCookieName = "release_watcher_session"

func NewRouter(service *service.Service, auth config.AuthConfig) http.Handler {
	router := &Router{
		service: service,
		mux:     http.NewServeMux(),
		auth:    auth,
	}
	router.routes()
	return router
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if strings.HasPrefix(req.URL.Path, "/api/") && !r.isPublicAPI(req) && !r.authenticated(req) {
		writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	r.mux.ServeHTTP(w, req)
}

func (r *Router) routes() {
	r.mux.HandleFunc("POST /api/auth/login", r.login)
	r.mux.HandleFunc("POST /api/auth/logout", r.logout)
	r.mux.HandleFunc("GET /api/auth/me", r.me)
	r.mux.HandleFunc("GET /api/dashboard/summary", r.dashboardSummary)
	r.mux.HandleFunc("GET /api/components", r.listComponents)
	r.mux.HandleFunc("POST /api/components", r.createComponent)
	r.mux.HandleFunc("GET /api/components/{id}", r.getComponent)
	r.mux.HandleFunc("PUT /api/components/{id}", r.updateComponent)
	r.mux.HandleFunc("DELETE /api/components/{id}", r.deleteComponent)
	r.mux.HandleFunc("POST /api/components/{id}/check", r.checkComponent)
	r.mux.HandleFunc("GET /api/components/{id}/subscribers", r.listSubscribers)
	r.mux.HandleFunc("POST /api/components/{id}/subscribers", r.createSubscriber)
	r.mux.HandleFunc("PUT /api/subscribers/{id}", r.updateSubscriber)
	r.mux.HandleFunc("DELETE /api/subscribers/{id}", r.deleteSubscriber)
	r.mux.HandleFunc("POST /api/checks/run", r.runChecks)
	r.mux.HandleFunc("GET /api/system-runs", r.listSystemRuns)
	r.mux.HandleFunc("GET /api/check-records", r.listCheckRecords)
	r.mux.HandleFunc("GET /api/check-records/{id}", r.getCheckRecord)
	r.mux.HandleFunc("GET /api/notification-records", r.listNotificationRecords)
	r.mux.HandleFunc("GET /api/notification-records/{id}", r.getNotificationRecord)
	r.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, req *http.Request) {
		writeOK(w, map[string]string{"status": "ok"})
	})
}

func (r *Router) isPublicAPI(req *http.Request) bool {
	return req.URL.Path == "/api/auth/login"
}

func (r *Router) login(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, req, &payload) {
		return
	}
	if payload.Username != r.auth.Username || payload.Password != r.auth.Password {
		writeError(w, http.StatusUnauthorized, errors.New("invalid username or password"))
		return
	}
	expiresAt := time.Now().Add(24 * time.Hour)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    r.signSession(payload.Username, expiresAt),
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookie(req),
	})
	writeOK(w, map[string]string{"username": payload.Username})
}

func (r *Router) logout(w http.ResponseWriter, req *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookie(req),
	})
	writeOK(w, map[string]bool{"logged_out": true})
}

func (r *Router) me(w http.ResponseWriter, req *http.Request) {
	writeOK(w, map[string]string{"username": r.auth.Username})
}

func (r *Router) listComponents(w http.ResponseWriter, req *http.Request) {
	opts := listOptions(req)
	items, total, err := r.service.ListComponents(req.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writePage(w, items, total, opts)
}

func (r *Router) createComponent(w http.ResponseWriter, req *http.Request) {
	var payload componentPayload
	if !decode(w, req, &payload) {
		return
	}
	item := payload.Component
	item.Enabled = true
	if payload.Enabled != nil {
		item.Enabled = *payload.Enabled
	}
	normalizeComponent(&item)
	if err := validateComponent(item); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := r.service.CreateComponent(req.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeStatus(w, http.StatusCreated, item)
}

func (r *Router) getComponent(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	item, err := r.service.GetComponent(req.Context(), id)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeOK(w, item)
}

func (r *Router) updateComponent(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	var item storage.Component
	if !decode(w, req, &item) {
		return
	}
	item.ID = id
	normalizeComponent(&item)
	if err := validateComponent(item); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := r.service.UpdateComponent(req.Context(), &item); err != nil {
		writeStorageError(w, err)
		return
	}
	writeOK(w, item)
}

func (r *Router) deleteComponent(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	if err := r.service.DeleteComponent(req.Context(), id); err != nil {
		writeStorageError(w, err)
		return
	}
	writeOK(w, map[string]bool{"deleted": true})
}

func (r *Router) checkComponent(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	record, err := r.service.CheckComponent(req.Context(), id)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeOK(w, record)
}

func (r *Router) listSubscribers(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	items, err := r.service.ListSubscribers(req.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeOK(w, items)
}

func (r *Router) createSubscriber(w http.ResponseWriter, req *http.Request) {
	componentID, ok := pathID(w, req)
	if !ok {
		return
	}
	var item storage.Subscriber
	if !decode(w, req, &item) {
		return
	}
	item.ComponentID = componentID
	item.Enabled = true
	if err := validateSubscriber(item); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := r.service.CreateSubscriber(req.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeStatus(w, http.StatusCreated, item)
}

func (r *Router) updateSubscriber(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	var item storage.Subscriber
	if !decode(w, req, &item) {
		return
	}
	item.ID = id
	if err := validateSubscriber(item); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := r.service.UpdateSubscriber(req.Context(), &item); err != nil {
		writeStorageError(w, err)
		return
	}
	writeOK(w, item)
}

func (r *Router) deleteSubscriber(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	if err := r.service.DeleteSubscriber(req.Context(), id); err != nil {
		writeStorageError(w, err)
		return
	}
	writeOK(w, map[string]bool{"deleted": true})
}

func (r *Router) runChecks(w http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(req.Context(), 5*60*1000*1000*1000)
	defer cancel()
	run, err := r.service.RunChecks(ctx, "manual")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeOK(w, run)
}

func (r *Router) listSystemRuns(w http.ResponseWriter, req *http.Request) {
	opts := listOptions(req)
	items, total, err := r.service.ListSystemRuns(req.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writePage(w, items, total, opts)
}

func (r *Router) listCheckRecords(w http.ResponseWriter, req *http.Request) {
	opts := listOptions(req)
	items, total, err := r.service.ListCheckRecords(req.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writePage(w, items, total, opts)
}

func (r *Router) getCheckRecord(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	item, err := r.service.GetCheckRecord(req.Context(), id)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeOK(w, item)
}

func (r *Router) listNotificationRecords(w http.ResponseWriter, req *http.Request) {
	opts := listOptions(req)
	items, total, err := r.service.ListNotificationRecords(req.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writePage(w, items, total, opts)
}

func (r *Router) getNotificationRecord(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	item, err := r.service.GetNotificationRecord(req.Context(), id)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeOK(w, item)
}

func (r *Router) dashboardSummary(w http.ResponseWriter, req *http.Request) {
	summary, err := r.service.DashboardSummary(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeOK(w, summary)
}

type response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type pageData struct {
	Items    any `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

type componentPayload struct {
	storage.Component
	Enabled *bool `json:"enabled"`
}

func writeOK(w http.ResponseWriter, data any) {
	writeStatus(w, http.StatusOK, data)
}

func writeStatus(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response{Code: 0, Message: "ok", Data: data})
}

func writePage(w http.ResponseWriter, items any, total int, opts storage.ListOptions) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	pageSize := opts.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	writeOK(w, pageData{Items: items, Total: total, Page: page, PageSize: pageSize})
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response{Code: status, Message: err.Error(), Data: nil})
}

func writeStorageError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func (r *Router) authenticated(req *http.Request) bool {
	cookie, err := req.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	username, expiresAt, ok := r.parseSession(cookie.Value)
	return ok && username == r.auth.Username && time.Now().Before(expiresAt)
}

func (r *Router) signSession(username string, expiresAt time.Time) string {
	payload := fmt.Sprintf("%s|%d", username, expiresAt.Unix())
	signature := r.sessionSignature(payload)
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + signature))
}

func (r *Router) parseSession(value string) (string, time.Time, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", time.Time{}, false
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 {
		return "", time.Time{}, false
	}
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", time.Time{}, false
	}
	payload := parts[0] + "|" + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(r.sessionSignature(payload))) {
		return "", time.Time{}, false
	}
	return parts[0], time.Unix(expiresUnix, 0), true
}

func (r *Router) sessionSignature(payload string) string {
	mac := hmac.New(sha256.New, []byte(r.auth.Secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func secureCookie(req *http.Request) bool {
	return req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https"
}

func decode(w http.ResponseWriter, req *http.Request, out any) bool {
	defer req.Body.Close()
	if err := json.NewDecoder(req.Body).Decode(out); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, req *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid id"))
		return 0, false
	}
	return id, true
}

func listOptions(req *http.Request) storage.ListOptions {
	query := req.URL.Query()
	opts := storage.ListOptions{
		Page:        intQuery(query.Get("page"), 1),
		PageSize:    intQuery(query.Get("page_size"), 20),
		Keyword:     query.Get("keyword"),
		Status:      query.Get("status"),
		ComponentID: int64(intQuery(query.Get("component_id"), 0)),
	}
	if value := query.Get("enabled"); value != "" {
		enabled := value == "true" || value == "1"
		opts.Enabled = &enabled
	}
	if value := query.Get("has_update"); value != "" {
		hasUpdate := value == "true" || value == "1"
		opts.HasUpdate = &hasUpdate
	}
	return opts
}

func intQuery(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func validateComponent(item storage.Component) error {
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(item.RepoOwner) == "" {
		return errors.New("repo_owner is required")
	}
	if strings.TrimSpace(item.RepoName) == "" {
		return errors.New("repo_name is required")
	}
	if strings.TrimSpace(item.CurrentVersion) == "" {
		return errors.New("current_version is required")
	}
	if strings.TrimSpace(item.OwnerEmail) == "" {
		return errors.New("owner_email is required")
	}
	if item.CheckStrategy == "" {
		item.CheckStrategy = "release_first"
	}
	if item.CheckStrategy != "release_first" && item.CheckStrategy != "tag_only" {
		return errors.New("check_strategy must be release_first or tag_only")
	}
	return nil
}

func normalizeComponent(item *storage.Component) {
	if item.CheckStrategy == "" {
		item.CheckStrategy = "release_first"
	}
}

func validateSubscriber(item storage.Subscriber) error {
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(item.Email) == "" {
		return errors.New("email is required")
	}
	return nil
}
