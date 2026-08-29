package smtp

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/StephenQiu30/hotkey-server/backend/test/smtpfixture"
)

func TestMailerSanitizesSMTPFailuresAndNeverExposesVerificationCode(t *testing.T) {
	mailer := NewMailer(Config{
		Enabled:   true,
		Host:      "smtp.163.com",
		Port:      465,
		TLSMode:   "tls",
		Username:  "sender@163.com",
		Password:  "smtp-app-password",
		FromEmail: "sender@163.com",
	}, func(context.Context, Message) error {
		return errors.New("smtp 535 auth failed for smtp-app-password")
	})

	err := mailer.SendVerificationCode(context.Background(), domain.VerificationPurposeRegistration, "receiver@example.test", "123456")
	var appError *sharederrors.AppError
	if !errors.As(err, &appError) || appError.Code != sharederrors.CodeUnavailable {
		t.Fatalf("SendVerificationCode() error = %v, want CodeUnavailable", err)
	}
	for _, sensitive := range []string{"smtp-app-password", "123456", "535"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("SendVerificationCode() leaked %q in %q", sensitive, err)
		}
	}
}

func TestMailerDisabledDoesNotSendVerificationCode(t *testing.T) {
	var sends int
	mailer := NewMailer(Config{
		Host:      "smtp.163.com",
		Port:      465,
		TLSMode:   "tls",
		FromEmail: "sender@163.com",
	}, func(context.Context, Message) error {
		sends++
		return nil
	})

	err := mailer.SendVerificationCode(context.Background(), domain.VerificationPurposeRegistration, "receiver@example.test", "123456")
	var appError *sharederrors.AppError
	if !errors.As(err, &appError) || appError.Code != sharederrors.CodeUnavailable {
		t.Fatalf("disabled SendVerificationCode() error = %v, want CodeUnavailable", err)
	}
	if sends != 0 {
		t.Fatalf("disabled mailer sends = %d, want 0", sends)
	}
}

func TestMailerCancelsDirectTLSHandshakeWithCallerContext(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for TLS handshake fixture: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener address: %v", err)
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		t.Fatalf("parse listener port: %v", err)
	}
	mailer := NewMailer(Config{Enabled: true, Host: host, Port: port, TLSMode: "tls", FromEmail: "sender@example.test"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- mailer.SendVerificationCode(ctx, domain.VerificationPurposeRegistration, "receiver@example.test", "123456")
	}()
	connection := <-accepted
	t.Cleanup(func() { _ = connection.Close() })
	cancel()

	select {
	case err := <-result:
		if appError := new(sharederrors.AppError); !errors.As(err, &appError) || appError.Code != sharederrors.CodeUnavailable {
			t.Fatalf("SendVerificationCode() error = %v, want sanitized CodeUnavailable", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("direct TLS handshake did not stop when caller context was cancelled")
	}
}

func TestIdentitySMTPCredentialRotationPrechecksRollsBackAndRevokesOldPassword(t *testing.T) {
	server := smtpfixture.New(t)
	host, port := server.Endpoint()
	username := "identity-rotation@example.test"
	oldSecret := "synthetic-identity-smtp-old-credential-012345"
	newSecret := "synthetic-identity-smtp-new-credential-012345"
	invalidSecret := "synthetic-identity-smtp-invalid-credential-012345"
	server.SetCredentials(map[string]string{username: oldSecret, username + ".next": newSecret})
	base := Config{Enabled: true, Host: host, Port: port, TLSMode: "tls", FromEmail: username, FromName: "HotKey"}
	mailer := func(user, password string) *Mailer {
		t.Helper()
		configured := base
		configured.Username = user
		configured.Password = password
		return newMailerWithTLSConfig(configured, server.TLSConfig())
	}
	send := func(candidate *Mailer) error {
		t.Helper()
		return candidate.SendVerificationCode(context.Background(), domain.VerificationPurposeRegistration, "rotation-recipient@example.test", "123456")
	}

	oldMailer := mailer(username, oldSecret)
	newMailer := mailer(username+".next", newSecret)
	if err := send(oldMailer); err != nil {
		t.Fatalf("old identity SMTP credential baseline failed: %v", err)
	}
	if err := send(newMailer); err != nil {
		t.Fatalf("new identity SMTP credential preflight failed: %v", err)
	}
	if err := send(mailer(username+".next", invalidSecret)); err == nil {
		t.Fatal("invalid identity SMTP candidate unexpectedly passed preflight")
	} else if strings.Contains(err.Error(), invalidSecret) {
		t.Fatal("identity SMTP preflight error exposed candidate credential plaintext")
	}
	if err := send(oldMailer); err != nil {
		t.Fatalf("failed identity SMTP candidate changed approved old credential: %v", err)
	}
	if err := send(newMailer); err != nil {
		t.Fatalf("rolled identity SMTP client did not use new credential: %v", err)
	}

	server.SetCredentials(map[string]string{username + ".next": newSecret})
	if err := send(oldMailer); err == nil {
		t.Fatal("revoked old identity SMTP credential remained usable")
	} else if strings.Contains(err.Error(), oldSecret) {
		t.Fatal("identity SMTP revocation error exposed old credential plaintext")
	}
	if err := send(newMailer); err != nil {
		t.Fatalf("new identity SMTP credential failed after revocation: %v", err)
	}
	if got, want := server.Deliveries(), 5; got != want {
		t.Fatalf("identity SMTP accepted deliveries = %d, want %d without failed-candidate or revoked duplicates", got, want)
	}
}
