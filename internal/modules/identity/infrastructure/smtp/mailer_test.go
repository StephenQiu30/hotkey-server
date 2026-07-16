package smtp

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
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
