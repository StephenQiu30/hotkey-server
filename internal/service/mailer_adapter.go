package service

import (
	"context"
	"log"

	"github.com/StephenQiu30/hotkey-server/internal/platform/email"
)

// emailMailerAdapter adapts email.Mailer (Send(ctx, Message) error) to the
// service.Mailer interface (Send(ctx, to, subject, body string) (string, error)).
// When the underlying email.Mailer is nil (SMTP not configured), Send is a no-op.
type emailMailerAdapter struct {
	mailer email.Mailer
}

// NewEmailMailerAdapter wraps an email.Mailer as a service.Mailer.
// If m is nil, the returned Mailer is a safe no-op.
func NewEmailMailerAdapter(m email.Mailer) Mailer {
	return &emailMailerAdapter{mailer: m}
}

// Send sends an email via the underlying email.Mailer. When the underlying
// mailer is nil (SMTP not configured), it logs a message and returns nil.
// The returned ID string is empty because the SMTP provider does not return
// a message ID synchronously.
func (a *emailMailerAdapter) Send(ctx context.Context, to, subject, body string) (string, error) {
	if a.mailer == nil {
		log.Printf("mailer: SMTP not configured, skipping email to %s", maskEmail(to))
		return "", nil
	}
	msg := email.Message{
		To:      to,
		Subject: subject,
		Text:    body,
		HTML:    body,
	}
	if err := a.mailer.Send(ctx, msg); err != nil {
		return "", err
	}
	return "", nil
}

// maskEmail returns a masked version of an email for safe logging.
func maskEmail(email string) string {
	if len(email) < 3 {
		return "***"
	}
	at := len(email)
	for i := 0; i < len(email); i++ {
		if email[i] == '@' {
			at = i
			break
		}
	}
	if at < 3 {
		return email[:1] + "***@" + email[at+1:]
	}
	return email[:3] + "***@" + email[at+1:]
}
