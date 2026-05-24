package notifier

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"opensource-release-watcher/backend/internal/config"
)

func TestSMTPMailStatusRequiresHostAndFrom(t *testing.T) {
	mail := NewSMTPMail(config.SMTPMailConfig{})

	status, err := mail.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if status.Configured {
		t.Fatal("Configured = true, want false")
	}
	if status.Connected {
		t.Fatal("Connected = true, want false")
	}
	if !strings.Contains(status.Message, "SMTP_HOST") {
		t.Fatalf("Message = %q, want SMTP_HOST guidance", status.Message)
	}
}

func TestSMTPMailSendsPlainRelayMessage(t *testing.T) {
	addr, received, shutdown := startFakeSMTPServer(t)
	defer shutdown()

	host, portText, ok := strings.Cut(addr, ":")
	if !ok {
		t.Fatalf("addr = %q", addr)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	mail := NewSMTPMail(config.SMTPMailConfig{
		Host: host,
		Port: port,
		From: "watcher@example.com",
	})

	if err := mail.Send(Message{
		To:      []string{"alice@example.com"},
		Subject: "Release update",
		Body:    "protobuf has a new release",
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-received:
		for _, want := range []string{
			"From: watcher@example.com",
			"To: alice@example.com",
			"Subject: Release update",
			"protobuf has a new release",
		} {
			if !strings.Contains(data, want) {
				t.Fatalf("message missing %q:\n%s", want, data)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for smtp message")
	}
}

func startFakeSMTPServer(t *testing.T) (string, <-chan string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan string, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		write := func(line string) {
			_, _ = fmt.Fprintf(conn, "%s\r\n", line)
		}
		write("220 fake smtp")
		var data strings.Builder
		inData := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if inData {
				if line == "." {
					received <- data.String()
					write("250 queued")
					inData = false
					continue
				}
				data.WriteString(line)
				data.WriteString("\n")
				continue
			}
			upper := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				write("250-fake smtp")
				write("250 OK")
			case strings.HasPrefix(upper, "MAIL FROM:"):
				write("250 OK")
			case strings.HasPrefix(upper, "RCPT TO:"):
				write("250 OK")
			case upper == "DATA":
				write("354 end with dot")
				inData = true
			case upper == "QUIT":
				write("221 bye")
				return
			default:
				write("250 OK")
			}
		}
	}()

	shutdown := func() {
		_ = listener.Close()
		<-done
	}
	return listener.Addr().String(), received, shutdown
}
