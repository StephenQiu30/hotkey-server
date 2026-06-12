package trend

import (
	"testing"
	"time"
)

func TestComputeVelocityPositiveGrowth(t *testing.T) {
	velocity := ComputeVelocity(160, 100)
	if velocity <= 0 {
		t.Fatalf("expected positive velocity for growth, got %f", velocity)
	}
}

func TestComputeVelocityDecline(t *testing.T) {
	velocity := ComputeVelocity(50, 100)
	if velocity >= 0 {
		t.Fatalf("expected negative velocity for decline, got %f", velocity)
	}
}

func TestComputeVelocityFlat(t *testing.T) {
	velocity := ComputeVelocity(100, 100)
	if velocity != 0 {
		t.Fatalf("expected zero velocity for flat, got %f", velocity)
	}
}

func TestComputeVelocityZeroPrevious(t *testing.T) {
	velocity := ComputeVelocity(100, 0)
	// When previous is 0, new posts = 100% growth
	if velocity <= 0 {
		t.Fatalf("expected positive velocity when previous is 0, got %f", velocity)
	}
}

func TestDetermineTrendDirectionRising(t *testing.T) {
	dir := DetermineTrendDirection(0.5)
	if dir != "rising" {
		t.Fatalf("expected 'rising', got '%s'", dir)
	}
}

func TestDetermineTrendDirectionFalling(t *testing.T) {
	dir := DetermineTrendDirection(-0.3)
	if dir != "falling" {
		t.Fatalf("expected 'falling', got '%s'", dir)
	}
}

func TestDetermineTrendDirectionFlat(t *testing.T) {
	dir := DetermineTrendDirection(0.01)
	if dir != "flat" {
		t.Fatalf("expected 'flat', got '%s'", dir)
	}
}

func TestBuildTopicSnapshot(t *testing.T) {
	svc := NewService(nil)
	snap := svc.BuildTopicSnapshot(TopicSnapshotInput{
		TopicID:          1,
		PostCount:        10,
		UniqueAuthorCount: 5,
		EngagementSum:    500,
		HeatScore:        120.5,
		PreviousHeat:     100.0,
		SnapshotTime:     time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	})
	if snap.TopicID != 1 {
		t.Fatalf("expected topic ID 1, got %d", snap.TopicID)
	}
	if snap.PostCount != 10 {
		t.Fatalf("expected 10 posts, got %d", snap.PostCount)
	}
	if snap.HeatScore != 120.5 {
		t.Fatalf("expected heat 120.5, got %f", snap.HeatScore)
	}
	if snap.TrendVelocity == 0 {
		t.Fatal("expected non-zero trend velocity")
	}
	if snap.TrendDirection == "" {
		t.Fatal("expected non-empty trend direction")
	}
}

func TestBuildMonitorSnapshot(t *testing.T) {
	svc := NewService(nil)
	snap := svc.BuildMonitorSnapshot(MonitorSnapshotInput{
		MonitorID:       10,
		NewPostCount:    25,
		ActiveTopicCount: 3,
		TotalEngagement: 1500,
		TopTopicID:      5,
		SnapshotTime:    time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	})
	if snap.MonitorID != 10 {
		t.Fatalf("expected monitor ID 10, got %d", snap.MonitorID)
	}
	if snap.NewPostCount != 25 {
		t.Fatalf("expected 25 new posts, got %d", snap.NewPostCount)
	}
	if snap.ActiveTopicCount != 3 {
		t.Fatalf("expected 3 active topics, got %d", snap.ActiveTopicCount)
	}
}
