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
