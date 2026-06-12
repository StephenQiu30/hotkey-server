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
	job := NewDispatchJob(repo, mailer)
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
	job := NewDispatchJob(repo, mailer)
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
	job := NewDispatchJob(repo, mailer)
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
	job := NewDispatchJob(repo, mailer)
	err := job.Run(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastSentAt == nil {
		t.Fatal("expected sent_at to be set on success")
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
	err       error
}

func (m *fakeMailer) Send(_ context.Context, to, subject, body string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.messageID, nil
}
