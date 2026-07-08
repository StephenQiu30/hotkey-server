package trend_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/service"
)

func TestComputeVelocityPositiveGrowth(t *testing.T) {
	velocity := service.ComputeVelocity(160, 100)
	if velocity <= 0 {
		t.Fatalf("expected positive velocity for growth, got %f", velocity)
	}
}

func TestComputeVelocityDecline(t *testing.T) {
	velocity := service.ComputeVelocity(50, 100)
	if velocity >= 0 {
		t.Fatalf("expected negative velocity for decline, got %f", velocity)
	}
}

func TestComputeVelocityFlat(t *testing.T) {
	velocity := service.ComputeVelocity(100, 100)
	if velocity != 0 {
		t.Fatalf("expected zero velocity for flat, got %f", velocity)
	}
}

func TestComputeVelocityZeroPrevious(t *testing.T) {
	velocity := service.ComputeVelocity(100, 0)
	if velocity <= 0 {
		t.Fatalf("expected positive velocity when previous is 0, got %f", velocity)
	}
}

func TestDetermineTrendDirectionRising(t *testing.T) {
	dir := service.DetermineTrendDirection(0.5)
	if dir != "rising" {
		t.Fatalf("expected 'rising', got '%s'", dir)
	}
}

func TestDetermineTrendDirectionFalling(t *testing.T) {
	dir := service.DetermineTrendDirection(-0.3)
	if dir != "falling" {
		t.Fatalf("expected 'falling', got '%s'", dir)
	}
}

func TestDetermineTrendDirectionFlat(t *testing.T) {
	dir := service.DetermineTrendDirection(0.01)
	if dir != "flat" {
		t.Fatalf("expected 'flat', got '%s'", dir)
	}
}

func TestBuildTopicSnapshot(t *testing.T) {
	snap := service.BuildTopicSnapshot(service.TopicSnapshotInput{
		TopicID:           1,
		PostCount:         10,
		UniqueAuthorCount: 5,
		EngagementSum:     500,
		HeatScore:         120.5,
		PreviousHeat:      100.0,
		SnapshotTime:      time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	})
	if snap.TopicID != 1 {
		t.Fatalf("expected TopicID 1, got %d", snap.TopicID)
	}
	if snap.TrendDirection != "rising" {
		t.Fatalf("expected 'rising', got '%s'", snap.TrendDirection)
	}
}

func TestBuildMonitorSnapshot(t *testing.T) {
	snap := service.BuildMonitorSnapshot(service.MonitorSnapshotInput{
		MonitorID:        10,
		NewPostCount:     25,
		ActiveTopicCount: 3,
		TotalEngagement:  1500,
		TopTopicID:       5,
		SnapshotTime:     time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	})
	if snap.MonitorID != 10 {
		t.Fatalf("expected MonitorID 10, got %d", snap.MonitorID)
	}
	if snap.NewPostCount != 25 {
		t.Fatalf("expected NewPostCount 25, got %d", snap.NewPostCount)
	}
}
