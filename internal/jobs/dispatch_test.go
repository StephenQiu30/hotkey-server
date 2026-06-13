package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDispatchMarksEmailDeliveryFailed(t *testing.T) {
	repo := &fakeDeliveryRepo{}
	mailer := &fakeMailer{err: errors.New("smtp down")}
	resolver := &fakeEmailResolver{email: "user@example.com"}
	job := NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 1)
	if err == nil {
		t.Fatal("expected dispatch error")
	}
	if repo.lastStatus != "failed" {
		t.Fatalf("expected failed status, got %q", repo.lastStatus)
	}
}

func TestDispatchMarksEmailDeliverySuccess(t *testing.T) {
	repo := &fakeDeliveryRepo{}
	mailer := &fakeMailer{messageID: "msg-123"}
	resolver := &fakeEmailResolver{email: "user@example.com"}
	job := NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastStatus != "sent" {
		t.Fatalf("expected sent status, got %q", repo.lastStatus)
	}
	if repo.lastProviderMessageID != "msg-123" {
		t.Errorf("expected provider message ID msg-123, got %q", repo.lastProviderMessageID)
	}
}

func TestDispatchCreatesDeliveryRecord(t *testing.T) {
	repo := &fakeDeliveryRepo{}
	mailer := &fakeMailer{messageID: "msg-456"}
	resolver := &fakeEmailResolver{email: "user@example.com"}
	job := NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastNotificationID != 42 {
		t.Errorf("expected notification ID 42, got %d", repo.lastNotificationID)
	}
	if repo.lastRecipientEmail == "" {
		t.Error("expected recipient email to be set")
	}
}

func TestDispatchSetsSentAtOnSuccess(t *testing.T) {
	repo := &fakeDeliveryRepo{}
	mailer := &fakeMailer{messageID: "msg-789"}
	resolver := &fakeEmailResolver{email: "user@example.com"}
	job := NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastSentAt == nil {
		t.Fatal("expected sent_at to be set on success")
	}
}

func TestDispatchResolvesRecipientEmail(t *testing.T) {
	repo := &fakeDeliveryRepo{}
	mailer := &fakeMailer{messageID: "msg-real"}
	resolver := &fakeEmailResolver{email: "alice@example.com"}
	job := NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastRecipientEmail != "alice@example.com" {
		t.Errorf("expected recipient alice@example.com, got %q", repo.lastRecipientEmail)
	}
	if mailer.lastTo != "alice@example.com" {
		t.Errorf("expected mailer to alice@example.com, got %q", mailer.lastTo)
	}
}

func TestDispatchFailsWhenEmailResolutionFails(t *testing.T) {
	repo := &fakeDeliveryRepo{}
	mailer := &fakeMailer{messageID: "msg-never"}
	resolver := &fakeEmailResolver{err: errors.New("notification not found")}
	job := NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 99)
	if err == nil {
		t.Fatal("expected error when email resolution fails")
	}
	if repo.lastRecipientEmail != "" {
		t.Errorf("expected no delivery record created, got email %q", repo.lastRecipientEmail)
	}
}

// fakes for testing

type fakeDeliveryRepo struct {
	lastNotificationID    int64
	lastRecipientEmail    string
	lastProvider          string
	lastProviderMessageID string
	lastStatus            string
	lastErrorMessage      string
	lastSentAt            *time.Time
}

func (r *fakeDeliveryRepo) CreateDelivery(_ context.Context, d EmailDelivery) error {
	r.lastNotificationID = d.NotificationID
	r.lastRecipientEmail = d.RecipientEmail
	r.lastProvider = d.Provider
	return nil
}

func (r *fakeDeliveryRepo) UpdateDeliveryStatus(_ context.Context, notificationID int64, status string, providerMsgID string, errMsg string) error {
	r.lastStatus = status
	r.lastProviderMessageID = providerMsgID
	r.lastErrorMessage = errMsg
	if status == "sent" {
		now := time.Now()
		r.lastSentAt = &now
	}
	return nil
}

func (r *fakeDeliveryRepo) GetPendingDeliveries(_ context.Context, limit int) ([]EmailDelivery, error) {
	return []EmailDelivery{
		{NotificationID: 1, RecipientEmail: "user@example.com", Provider: "smtp"},
	}, nil
}

type fakeMailer struct {
	messageID string
	lastTo    string
	err       error
}

func (m *fakeMailer) Send(_ context.Context, to, subject, body string) (string, error) {
	m.lastTo = to
	if m.err != nil {
		return "", m.err
	}
	return m.messageID, nil
}

type fakeEmailResolver struct {
	email string
	err   error
}

func (r *fakeEmailResolver) ResolveEmail(_ context.Context, _ int64) (string, error) {
	return r.email, r.err
}
