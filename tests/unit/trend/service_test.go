package trend_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// noopRepo implements trend.Repository as a no-op for unit tests.
type noopRepo struct{}

func (noopRepo) SaveTopicSnapshot(trend.TopicSnapshot) error { return nil }
func (noopRepo) SaveMonitorSnapshot(trend.MonitorSnapshot) error { return nil }
func (noopRepo) GetPreviousTopicHeat(int64) (float64, error) { return 0, nil }

func newNoopService() *trend.Service {
	return trend.NewService(noopRepo{})
}

func TestComputeVelocityPositiveGrowth(t *testing.T) {
	velocity := trend.ComputeVelocity(160, 100)
	if velocity <= 0 {
		t.Fatalf("expected positive velocity for growth, got %f", velocity)
	}
}

func TestComputeVelocityDecline(t *testing.T) {
	velocity := trend.ComputeVelocity(50, 100)
	if velocity >= 0 {
		t.Fatalf("expected negative velocity for decline, got %f", velocity)
	}
}

func TestComputeVelocityFlat(t *testing.T) {
	velocity := trend.ComputeVelocity(100, 100)
	if velocity != 0 {
		t.Fatalf("expected zero velocity for flat, got %f", velocity)
	}
}

func TestComputeVelocityZeroPrevious(t *testing.T) {
	velocity := trend.ComputeVelocity(100, 0)
	if velocity <= 0 {
		t.Fatalf("expected positive velocity when previous is 0, got %f", velocity)
	}
}

func TestDetermineTrendDirectionRising(t *testing.T) {
	dir := trend.DetermineTrendDirection(0.5)
	if dir != "rising" {
		t.Fatalf("expected 'rising', got '%s'", dir)
	}
}

func TestDetermineTrendDirectionFalling(t *testing.T) {
	dir := trend.DetermineTrendDirection(-0.3)
	if dir != "falling" {
		t.Fatalf("expected 'falling', got '%s'", dir)
	}
}

func TestDetermineTrendDirectionFlat(t *testing.T) {
	dir := trend.DetermineTrendDirection(0.01)
	if dir != "flat" {
		t.Fatalf("expected 'flat', got '%s'", dir)
	}
}

func TestBuildTopicSnapshot(t *testing.T) {
	svc := newNoopService()
	if err := svc.BuildTopicSnapshot(trend.TopicSnapshotInput{
		TopicID:           1,
		PostCount:         10,
		UniqueAuthorCount: 5,
		EngagementSum:     500,
		HeatScore:         120.5,
		PreviousHeat:      100.0,
		SnapshotTime:      time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("BuildTopicSnapshot failed: %v", err)
	}
}

func TestBuildMonitorSnapshot(t *testing.T) {
	svc := newNoopService()
	if err := svc.BuildMonitorSnapshot(trend.MonitorSnapshotInput{
		MonitorID:        10,
		NewPostCount:     25,
		ActiveTopicCount: 3,
		TotalEngagement:  1500,
		TopTopicID:       5,
		SnapshotTime:     time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("BuildMonitorSnapshot failed: %v", err)
	}
}
