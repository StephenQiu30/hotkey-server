package jobs_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/tests/testutil/fake/jobs"
)

func TestBuildSnapshotsJobCreatesTopicSnapshots(t *testing.T) {
	mockTrend := &fakejobs.TrendService{}
	mockTopics := &fakejobs.TopicProvider{
		Data: []jobs.TopicData{
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

	job := jobs.NewBuildSnapshotsJob(mockTrend, mockTopics)
	result, err := job.Run(jobs.BuildSnapshotsInput{
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
	mockTrend := &fakejobs.TrendService{}
	mockTopics := &fakejobs.TopicProvider{Data: nil}

	job := jobs.NewBuildSnapshotsJob(mockTrend, mockTopics)
	result, err := job.Run(jobs.BuildSnapshotsInput{
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
	mockTrend := &fakejobs.TrendService{}
	mockTopics := &fakejobs.TopicProvider{
		Data: []jobs.TopicData{
			{TopicID: 1, PostCount: 10, EngagementSum: 500, HeatScore: 120, PreviousHeat: 100},
			{TopicID: 2, PostCount: 5, EngagementSum: 200, HeatScore: 80, PreviousHeat: 90},
			{TopicID: 3, PostCount: 15, EngagementSum: 800, HeatScore: 200, PreviousHeat: 150},
		},
	}

	job := jobs.NewBuildSnapshotsJob(mockTrend, mockTopics)
	result, err := job.Run(jobs.BuildSnapshotsInput{
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
