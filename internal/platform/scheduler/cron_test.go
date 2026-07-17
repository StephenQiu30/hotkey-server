package scheduler

import (
	"testing"
	"time"
)

func TestDueAndUniqueKeyAreDeterministic(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	if !IsDue(DueSource{ID: 1, NextPoll: now.Add(-time.Minute)}, now) {
		t.Fatal("due source was skipped")
	}
	left := UniqueKey("collect_source", 1, 2, now, now.Add(time.Hour))
	right := UniqueKey("collect_source", 1, 2, now, now.Add(time.Hour))
	if left == "" || left != right {
		t.Fatalf("unstable key %q/%q", left, right)
	}
}
