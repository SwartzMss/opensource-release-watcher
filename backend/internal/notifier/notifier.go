package notifier

import (
	"fmt"
	"net/smtp"
	"strings"

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

type SMTP struct {
	cfg config.SMTPConfig
}

func NewSMTP(cfg config.SMTPConfig) *SMTP {
	return &SMTP{cfg: cfg}
}

func (s *SMTP) Send(message Message) error {
	if s.cfg.Host == "" || s.cfg.From == "" {
		return nil
	}
	if len(message.To) == 0 {
		return nil
	}
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	raw := strings.Join([]string{
		"From: " + s.cfg.From,
		"To: " + strings.Join(message.To, ", "),
		"Subject: " + message.Subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		message.Body,
	}, "\r\n")
	return smtp.SendMail(addr, auth, s.cfg.From, message.To, []byte(raw))
}
