package jobs

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// mockTrendService implements the methods needed by BuildSnapshotsJob.
type mockTrendService struct {
	topicSnaps   []trend.TopicSnapshot
	monitorSnaps []trend.MonitorSnapshot
}

func (m *mockTrendService) BuildTopicSnapshot(in trend.TopicSnapshotInput) trend.TopicSnapshot {
	snap := trend.TopicSnapshot{
		TopicID:           in.TopicID,
		SnapshotTime:      in.SnapshotTime,
		PostCount:         in.PostCount,
		UniqueAuthorCount: in.UniqueAuthorCount,
		EngagementSum:     in.EngagementSum,
		HeatScore:         in.HeatScore,
		TrendVelocity:     trend.ComputeVelocity(in.HeatScore, in.PreviousHeat),
		TrendDirection:    trend.DetermineTrendDirection(trend.ComputeVelocity(in.HeatScore, in.PreviousHeat)),
	}
	m.topicSnaps = append(m.topicSnaps, snap)
	return snap
}

func (m *mockTrendService) BuildMonitorSnapshot(in trend.MonitorSnapshotInput) trend.MonitorSnapshot {
	snap := trend.MonitorSnapshot{
		MonitorID:        in.MonitorID,
		SnapshotTime:     in.SnapshotTime,
		NewPostCount:     in.NewPostCount,
		ActiveTopicCount: in.ActiveTopicCount,
		TotalEngagement:  in.TotalEngagement,
		TopTopicID:       in.TopTopicID,
	}
	m.monitorSnaps = append(m.monitorSnaps, snap)
	return snap
}

// mockTopicProvider provides topic data for snapshot building.
type mockTopicProvider struct {
	topics []TopicData
}

func (m *mockTopicProvider) GetTopicDataForMonitor(monitorID int64) ([]TopicData, error) {
	return m.topics, nil
}

func TestBuildSnapshotsJobCreatesTopicSnapshots(t *testing.T) {
	mockTrend := &mockTrendService{}
	mockTopics := &mockTopicProvider{
		topics: []TopicData{
			{
				TopicID:           1,
				PostCount:         10,
				UniqueAuthorCount: 5,
				EngagementSum:     500,
				HeatScore:         120.0,
				PreviousHeat:      100.0,
			},
		},
	}

	job := NewBuildSnapshotsJob(mockTrend, mockTopics)
	result, err := job.Run(BuildSnapshotsInput{
		MonitorID:    10,
		SnapshotTime: time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TopicSnapshotCount != 1 {
		t.Fatalf("expected 1 topic snapshot, got %d", result.TopicSnapshotCount)
	}
	if result.MonitorSnapshotCount != 1 {
		t.Fatalf("expected 1 monitor snapshot, got %d", result.MonitorSnapshotCount)
	}
}

func TestBuildSnapshotsJobNoTopics(t *testing.T) {
	mockTrend := &mockTrendService{}
	mockTopics := &mockTopicProvider{topics: nil}

	job := NewBuildSnapshotsJob(mockTrend, mockTopics)
	result, err := job.Run(BuildSnapshotsInput{
		MonitorID:    10,
		SnapshotTime: time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TopicSnapshotCount != 0 {
		t.Fatalf("expected 0 topic snapshots, got %d", result.TopicSnapshotCount)
	}
}

func TestBuildSnapshotsJobMultipleTopics(t *testing.T) {
	mockTrend := &mockTrendService{}
	mockTopics := &mockTopicProvider{
		topics: []TopicData{
			{TopicID: 1, PostCount: 10, EngagementSum: 500, HeatScore: 120, PreviousHeat: 100},
			{TopicID: 2, PostCount: 5, EngagementSum: 200, HeatScore: 80, PreviousHeat: 90},
			{TopicID: 3, PostCount: 15, EngagementSum: 800, HeatScore: 200, PreviousHeat: 150},
		},
	}

	job := NewBuildSnapshotsJob(mockTrend, mockTopics)
	result, err := job.Run(BuildSnapshotsInput{
		MonitorID:    10,
		SnapshotTime: time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TopicSnapshotCount != 3 {
		t.Fatalf("expected 3 topic snapshots, got %d", result.TopicSnapshotCount)
	}
}
