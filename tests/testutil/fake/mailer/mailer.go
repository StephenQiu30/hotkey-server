package fakemailer

import (
	"context"
	"sync"

	"github.com/StephenQiu30/hotkey-server/internal/platform/email"
)

// Mailer is an in-memory fake implementing email.Mailer.
type Mailer struct {
	mu    sync.Mutex
	sent  []email.Message
}

// New returns a new FakeMailer ready for testing.
func New() *Mailer {
	return &Mailer{}
}

// Send stores the message in memory without delivering it.
func (m *Mailer) Send(_ context.Context, msg email.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

// Sent returns a copy of all messages sent so far.
func (m *Mailer) Sent() []email.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]email.Message, len(m.sent))
	copy(out, m.sent)
	return out
}

// Reset clears all captured messages.
func (m *Mailer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = nil
}
