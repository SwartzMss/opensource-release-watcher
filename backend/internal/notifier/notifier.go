package notifier

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"net/textproto"
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

type SMTPMail struct {
	cfg config.SMTPMailConfig
}

func NewSMTPMail(cfg config.SMTPMailConfig) *SMTPMail {
	return &SMTPMail{cfg: cfg}
}

func (s *SMTPMail) Status(ctx context.Context) (AuthStatus, error) {
	_ = ctx
	status := AuthStatus{
		Configured: s.configured(),
		Connected:  s.configured(),
	}
	if s.cfg.Host == "" {
		status.Message = "SMTP_HOST is required"
		return status, nil
	}
	if s.cfg.From == "" {
		status.Message = "SMTP_FROM is required"
		return status, nil
	}
	if s.cfg.Password != "" && s.cfg.Username == "" {
		status.Configured = false
		status.Connected = false
		status.Message = "SMTP_USERNAME is required when SMTP_PASSWORD is set"
	}
	return status, nil
}

func (s *SMTPMail) Send(message Message) error {
	if !s.configured() {
		return errors.New("SMTP_HOST and SMTP_FROM are required")
	}
	recipients := cleanRecipients(message.To)
	if len(recipients) == 0 {
		return nil
	}

	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.port()))
	dialer := net.Dialer{Timeout: 20 * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp create client: %w", err)
	}
	defer client.Close()

	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("smtp hello: %w", err)
	}
	if s.cfg.StartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(strings.TrimSpace(s.cfg.From)); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("smtp rcpt to %s: %w", recipient, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := writer.Write(buildSMTPMessage(s.cfg.From, recipients, message)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("smtp write message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("smtp finish message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit: %w", err)
	}
	return nil
}

func (s *SMTPMail) configured() bool {
	return strings.TrimSpace(s.cfg.Host) != "" && strings.TrimSpace(s.cfg.From) != "" && (s.cfg.Password == "" || s.cfg.Username != "")
}

func (s *SMTPMail) port() int {
	if s.cfg.Port > 0 {
		return s.cfg.Port
	}
	return 25
}

func cleanRecipients(values []string) []string {
	recipients := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			recipients = append(recipients, value)
		}
	}
	return recipients
}

func buildSMTPMessage(from string, recipients []string, message Message) []byte {
	var body strings.Builder
	headers := textproto.MIMEHeader{}
	headers.Set("From", sanitizeHeader(from))
	headers.Set("To", sanitizeHeader(strings.Join(recipients, ", ")))
	headers.Set("Subject", mime.QEncoding.Encode("utf-8", sanitizeHeader(message.Subject)))
	headers.Set("MIME-Version", "1.0")
	headers.Set("Content-Type", `text/plain; charset="utf-8"`)
	headers.Set("Content-Transfer-Encoding", "8bit")
	for key, values := range headers {
		for _, value := range values {
			body.WriteString(key)
			body.WriteString(": ")
			body.WriteString(value)
			body.WriteString("\r\n")
		}
	}
	body.WriteString("\r\n")
	body.WriteString(message.Body)
	body.WriteString("\r\n")
	return []byte(body.String())
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return strings.TrimSpace(value)
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
