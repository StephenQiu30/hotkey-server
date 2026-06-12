package notify

import "context"

// Mailer abstracts email sending.
type Mailer interface {
	// Send sends an email and returns the provider message ID.
	Send(ctx context.Context, to, subject, body string) (string, error)
}
