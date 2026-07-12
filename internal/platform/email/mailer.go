package email

import "context"

// Message represents an email to be sent.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// Mailer defines the interface for sending emails.
type Mailer interface {
	// Send delivers an email message. It must respect context cancellation
	// and deadlines.
	Send(ctx context.Context, msg Message) error
}
