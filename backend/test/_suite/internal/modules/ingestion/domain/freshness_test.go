package domain

import (
	"testing"
	"time"
)

func TestEligibleForNewEventUsesInclusiveSevenDayUTCWindow(t *testing.T) {
	t.Parallel()
	evaluatedAt := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	cutoff := evaluatedAt.UTC().Add(-NewEventFreshnessWindow)

	for _, test := range []struct {
		name        string
		publishedAt time.Time
		want        bool
	}{
		{name: "inside", publishedAt: cutoff.Add(time.Nanosecond), want: true},
		{name: "inclusive_boundary", publishedAt: cutoff, want: true},
		{name: "stale", publishedAt: cutoff.Add(-time.Nanosecond), want: false},
		{name: "future_clock_skew", publishedAt: evaluatedAt.Add(time.Minute), want: true},
		{name: "missing_published_time", publishedAt: time.Time{}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := EligibleForNewEvent(test.publishedAt, evaluatedAt); got != test.want {
				t.Fatalf("EligibleForNewEvent() = %t, want %t", got, test.want)
			}
		})
	}

	if EligibleForNewEvent(evaluatedAt, time.Time{}) {
		t.Fatal("zero evaluation time must fail closed")
	}
}
