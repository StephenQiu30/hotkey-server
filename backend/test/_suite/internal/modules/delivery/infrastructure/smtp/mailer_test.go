package smtp

import (
	"context"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/test/smtpfixture"
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

func TestSMTPCredentialRotationPrechecksRollsBackAndRevokesOldPassword(t *testing.T) {
	server := smtpfixture.New(t)
	host, port := server.Endpoint()
	username := "rotation-sender@example.test"
	oldSecret := "synthetic-smtp-old-credential-0123456789"
	newSecret := "synthetic-smtp-new-credential-0123456789"
	invalidSecret := "synthetic-smtp-invalid-credential-012345"
	server.SetCredentials(map[string]string{username: oldSecret, username + ".next": newSecret})
	base := config.SMTPConfig{
		Enabled: true, Host: host, Port: port, TLSMode: "tls",
		FromEmail: username, FromName: "HotKey",
	}
	mailer := func(user, password string) *Mailer {
		t.Helper()
		configured := base
		configured.Username = user
		configured.Password = password
		return newMailerWithTLSConfig(configured, server.TLSConfig())
	}
	message := Message{To: "rotation-recipient@example.test", Subject: "rotation probe", Text: "probe", HTML: "<p>probe</p>"}
	send := func(candidate *Mailer) error {
		t.Helper()
		return candidate.Send(context.Background(), message)
	}

	oldMailer := mailer(username, oldSecret)
	newMailer := mailer(username+".next", newSecret)
	if err := send(oldMailer); err != nil {
		t.Fatalf("old SMTP credential baseline failed: %v", err)
	}
	if err := send(newMailer); err != nil {
		t.Fatalf("new SMTP credential preflight failed: %v", err)
	}
	if err := send(mailer(username+".next", invalidSecret)); err == nil {
		t.Fatal("invalid candidate SMTP credential unexpectedly passed preflight")
	} else if strings.Contains(err.Error(), invalidSecret) {
		t.Fatal("SMTP preflight error exposed candidate credential plaintext")
	}
	if err := send(oldMailer); err != nil {
		t.Fatalf("failed SMTP candidate changed the approved old credential: %v", err)
	}
	if err := send(newMailer); err != nil {
		t.Fatalf("rolled SMTP client did not use the preflighted new credential: %v", err)
	}

	server.SetCredentials(map[string]string{username + ".next": newSecret})
	if err := send(oldMailer); err == nil {
		t.Fatal("revoked old SMTP credential remained usable")
	} else if strings.Contains(err.Error(), oldSecret) {
		t.Fatal("SMTP revocation error exposed old credential plaintext")
	}
	if err := send(newMailer); err != nil {
		t.Fatalf("new SMTP credential failed after old credential revocation: %v", err)
	}
	if got, want := server.Deliveries(), 5; got != want {
		t.Fatalf("SMTP accepted deliveries = %d, want %d without failed-candidate or revoked duplicates", got, want)
	}
}
