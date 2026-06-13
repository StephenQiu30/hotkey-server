package jobs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/tests/testutil/fake/jobs"
)

func TestDispatchMarksEmailDeliveryFailed(t *testing.T) {
	repo := &fakejobs.DeliveryRepo{}
	mailer := &fakejobs.Mailer{Err: errors.New("smtp down")}
	resolver := &fakejobs.EmailResolver{Email: "user@example.com"}
	job := jobs.NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 1)
	if err == nil {
		t.Fatal("expected dispatch error")
	}
	if repo.LastStatus != "failed" {
		t.Fatalf("expected failed status, got %q", repo.LastStatus)
	}
}

func TestDispatchMarksEmailDeliverySuccess(t *testing.T) {
	repo := &fakejobs.DeliveryRepo{}
	mailer := &fakejobs.Mailer{MessageID: "msg-123"}
	resolver := &fakejobs.EmailResolver{Email: "user@example.com"}
	job := jobs.NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.LastStatus != "sent" {
		t.Fatalf("expected sent status, got %q", repo.LastStatus)
	}
	if repo.LastProviderMessageID != "msg-123" {
		t.Errorf("expected provider message ID msg-123, got %q", repo.LastProviderMessageID)
	}
}

func TestDispatchCreatesDeliveryRecord(t *testing.T) {
	repo := &fakejobs.DeliveryRepo{}
	mailer := &fakejobs.Mailer{MessageID: "msg-456"}
	resolver := &fakejobs.EmailResolver{Email: "user@example.com"}
	job := jobs.NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.LastNotificationID != 42 {
		t.Errorf("expected notification ID 42, got %d", repo.LastNotificationID)
	}
	if repo.LastRecipientEmail == "" {
		t.Error("expected recipient email to be set")
	}
}

func TestDispatchSetsSentAtOnSuccess(t *testing.T) {
	repo := &fakejobs.DeliveryRepo{}
	mailer := &fakejobs.Mailer{MessageID: "msg-789"}
	resolver := &fakejobs.EmailResolver{Email: "user@example.com"}
	job := jobs.NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.LastSentAt == nil {
		t.Fatal("expected sent_at to be set on success")
	}
}

func TestDispatchResolvesRecipientEmail(t *testing.T) {
	repo := &fakejobs.DeliveryRepo{}
	mailer := &fakejobs.Mailer{MessageID: "msg-real"}
	resolver := &fakejobs.EmailResolver{Email: "alice@example.com"}
	job := jobs.NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.LastRecipientEmail != "alice@example.com" {
		t.Errorf("expected recipient alice@example.com, got %q", repo.LastRecipientEmail)
	}
	if mailer.LastTo != "alice@example.com" {
		t.Errorf("expected mailer to alice@example.com, got %q", mailer.LastTo)
	}
}

func TestDispatchFailsWhenEmailResolutionFails(t *testing.T) {
	repo := &fakejobs.DeliveryRepo{}
	mailer := &fakejobs.Mailer{MessageID: "msg-never"}
	resolver := &fakejobs.EmailResolver{Err: errors.New("notification not found")}
	job := jobs.NewDispatchJob(repo, mailer, resolver)
	err := job.Run(context.Background(), 99)
	if err == nil {
		t.Fatal("expected error when email resolution fails")
	}
	if repo.LastRecipientEmail != "" {
		t.Errorf("expected no delivery record created, got email %q", repo.LastRecipientEmail)
	}
}
