package smtp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

func TestMailerSanitizesSMTPFailuresAndNeverExposesVerificationCode(t *testing.T) {
	mailer := NewMailer(Config{
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
