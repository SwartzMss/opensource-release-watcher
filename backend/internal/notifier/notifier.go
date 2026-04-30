package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"opensource-release-watcher/backend/internal/config"
)

type Message struct {
	To      []string
	Subject string
	Body    string
}

type Notifier interface {
	Send(message Message) error
}

type AuthStatus struct {
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	Message    string `json:"message,omitempty"`
}

type StatusProvider interface {
	Status(ctx context.Context) (AuthStatus, error)
}

type GraphDelegatedMail struct {
	cfg        config.GraphMailConfig
	httpClient *http.Client
}

func NewGraphDelegatedMail(cfg config.GraphMailConfig) *GraphDelegatedMail {
	return &GraphDelegatedMail{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (g *GraphDelegatedMail) Status(ctx context.Context) (AuthStatus, error) {
	status := AuthStatus{
		Configured: g.configured(),
		Connected:  g.cfg.RefreshToken != "" || g.cfg.AccessToken != "",
	}
	if !status.Configured {
		status.Message = "GRAPH_CLIENT_ID is required"
		return status, nil
	}
	if !status.Connected {
		status.Message = "GRAPH_REFRESH_TOKEN or GRAPH_ACCESS_TOKEN is required"
	}
	return status, nil
}

func (g *GraphDelegatedMail) Send(message Message) error {
	if !g.configured() {
		return errors.New("GRAPH_CLIENT_ID is required")
	}
	if len(message.To) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	accessToken, err := g.accessToken(ctx)
	if err != nil {
		return err
	}
	return g.sendMail(ctx, accessToken, message)
}

func (g *GraphDelegatedMail) accessToken(ctx context.Context) (string, error) {
	if g.cfg.RefreshToken == "" {
		if g.cfg.AccessToken == "" {
			return "", errors.New("GRAPH_REFRESH_TOKEN or GRAPH_ACCESS_TOKEN is required")
		}
		return g.cfg.AccessToken, nil
	}
	token, err := g.tokenRequest(ctx, url.Values{
		"client_id":     {g.cfg.ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {g.cfg.RefreshToken},
		"scope":         {"offline_access Mail.Send User.Read"},
	})
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func (g *GraphDelegatedMail) tokenRequest(ctx context.Context, form url.Values) (*tokenResponse, error) {
	if g.cfg.ClientSecret != "" {
		form.Set("client_secret", g.cfg.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.tokenEndpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("microsoft token request returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload tokenResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.AccessToken == "" {
		return nil, errors.New("microsoft token response missing access_token")
	}
	return &payload, nil
}

func (g *GraphDelegatedMail) sendMail(ctx context.Context, token string, message Message) error {
	recipients := make([]map[string]map[string]string, 0, len(message.To))
	for _, address := range message.To {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		recipients = append(recipients, map[string]map[string]string{
			"emailAddress": {"address": address},
		})
	}
	if len(recipients) == 0 {
		return nil
	}

	payload := map[string]any{
		"message": map[string]any{
			"subject": message.Subject,
			"body": map[string]string{
				"contentType": "Text",
				"content":     message.Body,
			},
			"toRecipients": recipients,
		},
		"saveToSentItems": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://graph.microsoft.com/v1.0/me/sendMail", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("graph sendMail returned %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func (g *GraphDelegatedMail) configured() bool {
	return g.cfg.ClientID != ""
}

func (g *GraphDelegatedMail) tokenEndpoint() string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", url.PathEscape(g.tenant()))
}

func (g *GraphDelegatedMail) tenant() string {
	tenant := strings.TrimSpace(g.cfg.TenantID)
	if tenant == "" {
		return "common"
	}
	return tenant
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}
