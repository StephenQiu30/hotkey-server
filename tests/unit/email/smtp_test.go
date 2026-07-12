package email_test

import (
	"context"
	"crypto/tls"
	"net/smtp"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/email"
	"github.com/StephenQiu30/hotkey-server/tests/testutil/fake/mailer"
)

func TestRegistrationCodeMessage(t *testing.T) {
	msg := email.RegistrationCodeMessage("user@example.com", "123456")
	if msg.To != "user@example.com" {
		t.Fatalf("expected To user@example.com, got %s", msg.To)
	}
	if !strings.Contains(msg.HTML, "123456") {
		t.Fatal("expected HTML to contain the verification code")
	}
	if !strings.Contains(msg.Text, "123456") {
		t.Fatal("expected Text to contain the verification code")
	}
	if !strings.Contains(msg.HTML, "10") && !strings.Contains(msg.HTML, "10分钟") {
		t.Fatal("expected HTML to mention 10-minute validity")
	}
	if !strings.Contains(msg.Text, "10") && !strings.Contains(msg.Text, "10分钟") {
		t.Fatal("expected Text to mention 10-minute validity")
	}
	if strings.Contains(msg.HTML, "Token") || strings.Contains(msg.HTML, "Password") {
		t.Fatal("should not leak sensitive terms in email")
	}
}

func TestPasswordResetCodeMessage(t *testing.T) {
	msg := email.PasswordResetCodeMessage("user@example.com", "654321")
	if msg.To != "user@example.com" {
		t.Fatalf("expected To user@example.com, got %s", msg.To)
	}
	if !strings.Contains(msg.HTML, "654321") {
		t.Fatal("expected HTML to contain the reset code")
	}
	if !strings.Contains(msg.Text, "654321") {
		t.Fatal("expected Text to contain the reset code")
	}
	if !strings.Contains(msg.HTML, "10") && !strings.Contains(msg.HTML, "10分钟") {
		t.Fatal("expected HTML to mention 10-minute validity")
	}
}

func TestPasswordChangedMessage(t *testing.T) {
	msg := email.PasswordChangedMessage("user@example.com", "TestUser")
	if msg.To != "user@example.com" {
		t.Fatalf("expected To user@example.com, got %s", msg.To)
	}
	if !strings.Contains(msg.HTML, "TestUser") {
		t.Fatal("expected HTML to contain the display name")
	}
	if !strings.Contains(msg.Text, "TestUser") {
		t.Fatal("expected Text to contain the display name")
	}
}

func TestTemplateEscapesDisplayName(t *testing.T) {
	msg := email.PasswordChangedMessage("u@example.com", `<script>alert(1)</script>`)
	if strings.Contains(msg.HTML, "<script>") {
		t.Fatal("unescaped HTML in message")
	}
	if msg.Text == "" {
		t.Fatal("missing text alternative")
	}
}

func TestMessageHasTextAlternative(t *testing.T) {
	msgs := []email.Message{
		email.RegistrationCodeMessage("a@b.com", "123456"),
		email.PasswordResetCodeMessage("a@b.com", "654321"),
		email.PasswordChangedMessage("a@b.com", "User"),
	}
	for i, msg := range msgs {
		if msg.HTML == "" {
			t.Fatalf("msg[%d] missing HTML body", i)
		}
		if msg.Text == "" {
			t.Fatalf("msg[%d] missing plain text alternative", i)
		}
	}
}

func TestFakeMailer(t *testing.T) {
	m := fakemailer.New()
	ctx := context.Background()

	if err := m.Send(ctx, email.Message{To: "a@b.com", Subject: "Test", HTML: "<p>Hi</p>", Text: "Hi"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Sent()) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(m.Sent()))
	}
	if m.Sent()[0].To != "a@b.com" {
		t.Fatalf("expected To a@b.com, got %s", m.Sent()[0].To)
	}
}

func TestSMTPMailer_TLSConfig(t *testing.T) {
	// Verify that the SMTPMailer uses correct TLS config implicitly via
	// the dialer. We create a mailer and check its server name configuration.
	cfg := email.SMTPConfig{
		Host:     "smtp.163.com",
		Port:     465,
		Username: "test@163.com",
		AuthCode: "authcode",
		FromName: "Test",
	}
	m := email.NewSMTPMailer(cfg)

	// Verify TLS config by checking exported dialer
	dialer := m.TLSDialer()
	if dialer.ServerName != "smtp.163.com" {
		t.Fatalf("expected ServerName smtp.163.com, got %s", dialer.ServerName)
	}
	if dialer.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected MinVersion TLS 1.2, got %d", dialer.MinVersion)
	}
}

func TestSMTPMailer_Auth(t *testing.T) {
	cfg := email.SMTPConfig{
		Host:     "smtp.163.com",
		Port:     465,
		Username: "test@163.com",
		AuthCode: "secret-auth-code",
		FromName: "Test",
	}
	m := email.NewSMTPMailer(cfg)

	auth := m.SMTPAuth()
	// PlainAuth should be usable for 163 authorization code auth
	if _, ok := auth.(smtp.Auth); !ok {
		t.Fatal("expected smtp.Auth implementation")
	}
}

func TestSMTPMailer_SendContextDeadline(t *testing.T) {
	// Test that the Send method respects context cancellation.
	// Use an unreachable address so dial fails under cancelled context.
	cfg := email.SMTPConfig{
		Host:     "192.0.2.1", // TEST-NET, unreachable
		Port:     465,
		Username: "test@example.com",
		AuthCode: "auth",
		FromName: "Test",
		From:     "test@example.com",
	}
	m := email.NewSMTPMailer(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := m.Send(ctx, email.Message{To: "a@b.com", Subject: "T", HTML: "<p>X</p>", Text: "X"})
	if err == nil {
		t.Fatal("expected error due to cancelled context")
	}
}

func TestSMTPMailer_CredentialsNotLeaked(t *testing.T) {
	cfg := email.SMTPConfig{
		Host:     "192.0.2.1",
		Port:     465,
		Username: "secret-user@163.com",
		AuthCode: "super-secret-auth-code-12345",
		FromName: "Test",
		From:     "secret-user@163.com",
	}
	m := email.NewSMTPMailer(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.Send(ctx, email.Message{To: "a@b.com", Subject: "T", HTML: "<p>X</p>", Text: "X"})
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "secret-user") || strings.Contains(errStr, "super-secret-auth-code-12345") || strings.Contains(errStr, "secret-auth-code") {
			t.Fatal("credentials leaked in error message")
		}
	}
}
