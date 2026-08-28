package queue

import (
	"testing"
	"time"
)

func TestEvaluateEventAlertsKindIsStableAndKnown(t *testing.T) {
	if KindEvaluateEventAlerts != "evaluate_event_alerts" {
		t.Fatalf("KindEvaluateEventAlerts = %q", KindEvaluateEventAlerts)
	}
	if !IsKnownKind(KindEvaluateEventAlerts) {
		t.Fatal("evaluate_event_alerts is not registered as a known kind")
	}
	job := Job{
		Kind: KindEvaluateEventAlerts, UniqueKey: "event-update-1", ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 5,
		Payload: Payload{EntityID: 1, EntityVersion: 1, InputHash: "event-update-hash"},
	}
	if err := job.Validate(); err != nil {
		t.Fatalf("evaluate_event_alerts job rejected: %v", err)
	}
}

func TestProjectUserNotificationKindIsStableAndKnown(t *testing.T) {
	if KindProjectUserNotification != "project_user_notification" || !IsKnownKind(KindProjectUserNotification) {
		t.Fatalf("project user notification kind = %q known=%t", KindProjectUserNotification, IsKnownKind(KindProjectUserNotification))
	}
	job := Job{
		Kind: KindProjectUserNotification, UniqueKey: "notification-outbox-1", ScheduledAt: time.Now().UTC(), MaxAttempts: 5, Priority: 6,
		Payload: Payload{EntityID: 1, EntityVersion: 1, InputHash: StableJobHash(KindProjectUserNotification, "1", "1")},
	}
	if err := job.Validate(); err != nil {
		t.Fatalf("project user notification job rejected: %v", err)
	}
}
