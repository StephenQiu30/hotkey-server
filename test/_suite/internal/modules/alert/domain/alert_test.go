package domain

import (
	"math"
	"testing"
	"time"
)

func TestTriggerTypeForEventUpdateUsesTheV1ActionableMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind       string
		want       TriggerType
		actionable bool
	}{
		{kind: "event_created", want: TriggerNewEvent, actionable: true},
		{kind: "rising", want: TriggerRising, actionable: true},
		{kind: "reactivated", want: TriggerReactivated, actionable: true},
		{kind: "source_expansion", want: TriggerThresholdCrossed, actionable: true},
		{kind: "metric_changed", want: TriggerThresholdCrossed, actionable: true},
		{kind: "cooling", actionable: false},
		{kind: "unknown", actionable: false},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			got, actionable := TriggerTypeForEventUpdate(test.kind)
			if got != test.want || actionable != test.actionable {
				t.Fatalf("TriggerTypeForEventUpdate(%q) = %q/%v, want %q/%v", test.kind, got, actionable, test.want, test.actionable)
			}
		})
	}
}

func TestSeverityForScoreUsesAbsoluteV1Boundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		score float64
		want  Severity
	}{
		{score: 0, want: SeverityInfo},
		{score: 74.99, want: SeverityInfo},
		{score: 75, want: SeverityWarning},
		{score: 89.99, want: SeverityWarning},
		{score: 90, want: SeverityCritical},
		{score: 100, want: SeverityCritical},
	} {
		got, err := SeverityForScore(test.score)
		if err != nil || got != test.want {
			t.Fatalf("SeverityForScore(%v) = %q/%v, want %q", test.score, got, err, test.want)
		}
	}
	for _, invalid := range []float64{-0.01, 100.01, math.NaN(), math.Inf(1)} {
		if _, err := SeverityForScore(invalid); err == nil {
			t.Fatalf("SeverityForScore(%v) error = nil", invalid)
		}
	}
}

func TestOccurrenceFingerprintUsesTheFrozenOrderedV1Input(t *testing.T) {
	t.Parallel()
	input := FingerprintInput{
		MonitorConfigVersionID: 17,
		EventUpdateID:          41,
		TriggerType:            TriggerRising,
		PolicyVersion:          PolicyVersionV1,
	}
	want := "b78510be4bb517aa8b8bf09e4313d13c2ac4a3491f9e6daa19fdbe61ff32ddd7"
	got, err := OccurrenceFingerprint(input)
	if err != nil || got != want {
		t.Fatalf("OccurrenceFingerprint() = %q/%v, want %q", got, err, want)
	}

	changed := input
	changed.EventUpdateID++
	other, err := OccurrenceFingerprint(changed)
	if err != nil || other == got {
		t.Fatalf("changed input fingerprint = %q/%v, want a distinct value", other, err)
	}
	if _, err := OccurrenceFingerprint(FingerprintInput{}); err == nil {
		t.Fatal("OccurrenceFingerprint(empty) error = nil")
	}
}

func TestAlertStateMachineReopensOnlyAfterCooldownAndNeverReopensSuppressed(t *testing.T) {
	t.Parallel()
	allowed := map[[2]State]bool{
		{StateOpen, StateAcknowledged}:       true,
		{StateOpen, StateResolved}:           true,
		{StateOpen, StateSuppressed}:         true,
		{StateAcknowledged, StateResolved}:   true,
		{StateAcknowledged, StateSuppressed}: true,
	}
	states := []State{StateOpen, StateAcknowledged, StateResolved, StateSuppressed}
	for _, from := range states {
		for _, to := range states {
			if got := CanUserTransition(from, to); got != allowed[[2]State{from, to}] {
				t.Fatalf("CanUserTransition(%q, %q) = %v", from, to, got)
			}
		}
	}

	triggeredAt := time.Date(2026, 8, 4, 8, 0, 0, 123, time.FixedZone("CST", 8*60*60))
	cooldownUntil := CooldownUntil(triggeredAt)
	if want := triggeredAt.UTC().Add(time.Hour); !cooldownUntil.Equal(want) {
		t.Fatalf("CooldownUntil() = %s, want %s", cooldownUntil, want)
	}
	for _, state := range []State{StateAcknowledged, StateResolved} {
		if ShouldReopen(state, cooldownUntil, cooldownUntil.Add(-time.Nanosecond)) {
			t.Fatalf("%q reopened inside cooldown", state)
		}
		if !ShouldReopen(state, cooldownUntil, cooldownUntil) {
			t.Fatalf("%q did not reopen at cooldown boundary", state)
		}
	}
	if ShouldReopen(StateOpen, cooldownUntil, cooldownUntil.Add(time.Hour)) || ShouldReopen(StateSuppressed, cooldownUntil, cooldownUntil.Add(time.Hour)) {
		t.Fatal("open or suppressed thread was treated as an automatic reopen candidate")
	}
}
