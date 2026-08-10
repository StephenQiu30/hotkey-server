package smtp

import (
	"context"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
)

func TestSendStripsHeaderControlCharacters(t *testing.T) {
	var captured Message
	mailer := NewMailer(config.SMTPConfig{
		Enabled:   true,
		Host:      "smtp.example.test",
		Port:      465,
		TLSMode:   "tls",
		Username:  "sender@example.test",
		Password:  "app-password",
		FromEmail: "sender@example.test",
	}, func(_ context.Context, message Message) error {
		captured = message
		return nil
	})

	err := mailer.Send(context.Background(), Message{
		To:      "victim@example.test\r\nBcc: injected@example.test",
		Subject: "hello\r\nCc: spy@example.test",
		Text:    "plain body\r\nX-Injected: 1",
		HTML:    "<p>html body</p>\r\nX-Injected-2: 1",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if strings.ContainsAny(captured.To, "\r\n") {
		t.Errorf("To still contains CR/LF: %q", captured.To)
	}
	if strings.ContainsAny(captured.Subject, "\r\n") {
		t.Errorf("Subject still contains CR/LF: %q", captured.Subject)
	}
}

func TestSendPreservesNormalContent(t *testing.T) {
	var captured Message
	mailer := NewMailer(config.SMTPConfig{
		Enabled:   true,
		Host:      "smtp.example.test",
		Port:      465,
		TLSMode:   "tls",
		Username:  "sender@example.test",
		Password:  "app-password",
		FromEmail: "sender@example.test",
	}, func(_ context.Context, message Message) error {
		captured = message
		return nil
	})

	expected := Message{
		To:      "recipient@example.test",
		Subject: "[HotKey] 热点事件更新",
		Text:    "正文第一行\n正文第二行",
		HTML:    "<p>正文</p>",
	}
	if err := mailer.Send(context.Background(), expected); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if captured != expected {
		t.Errorf("normal content was mutated:\n got %q\nwant %q", captured, expected)
	}
}
