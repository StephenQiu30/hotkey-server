// Package jobs implements background job orchestration for the hotkey-server.
// BuildSnapshotsJob coordinates topic and monitor snapshot generation.
package jobs

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// TopicData holds pre-aggregated data for a single topic snapshot.
type TopicData struct {
	TopicID           int64
	PostCount         int
	UniqueAuthorCount int
	EngagementSum     int
	HeatScore         float64
	PreviousHeat      float64
}

// TopicProvider abstracts fetching topic data for snapshot building.
type TopicProvider interface {
	GetTopicDataForMonitor(monitorID int64) ([]TopicData, error)
}

// TrendSnapshotter abstracts the trend service methods used by the job.
type TrendSnapshotter interface {
	BuildTopicSnapshot(in trend.TopicSnapshotInput) trend.TopicSnapshot
	BuildMonitorSnapshot(in trend.MonitorSnapshotInput) trend.MonitorSnapshot
}

// BuildSnapshotsInput holds the parameters for a snapshot run.
type BuildSnapshotsInput struct {
	MonitorID    int64
	SnapshotTime time.Time
}

// BuildSnapshotsResult holds the outcome of a snapshot run.
type BuildSnapshotsResult struct {
	TopicSnapshotCount   int
	MonitorSnapshotCount int
	TopTopicID           int64
}

// BuildSnapshotsJob orchestrates snapshot generation for a monitor.
type BuildSnapshotsJob struct {
	trend   TrendSnapshotter
	topics  TopicProvider
}

// NewBuildSnapshotsJob creates a BuildSnapshotsJob.
func NewBuildSnapshotsJob(trend TrendSnapshotter, topics TopicProvider) *BuildSnapshotsJob {
	return &BuildSnapshotsJob{trend: trend, topics: topics}
}

// Run executes the snapshot job for the given monitor.
func (j *BuildSnapshotsJob) Run(in BuildSnapshotsInput) (BuildSnapshotsResult, error) {
	topicData, err := j.topics.GetTopicDataForMonitor(in.MonitorID)
	if err != nil {
		return BuildSnapshotsResult{}, err
	}

	var topTopicID int64
	var maxHeat float64
	totalEngagement := 0

	for _, td := range topicData {
		j.trend.BuildTopicSnapshot(trend.TopicSnapshotInput{
			TopicID:           td.TopicID,
			PostCount:         td.PostCount,
			UniqueAuthorCount: td.UniqueAuthorCount,
			EngagementSum:     td.EngagementSum,
			HeatScore:         td.HeatScore,
			PreviousHeat:      td.PreviousHeat,
			SnapshotTime:      in.SnapshotTime,
		})
		totalEngagement += td.EngagementSum
		if td.HeatScore > maxHeat {
			maxHeat = td.HeatScore
			topTopicID = td.TopicID
		}
	}

	j.trend.BuildMonitorSnapshot(trend.MonitorSnapshotInput{
		MonitorID:        in.MonitorID,
		NewPostCount:     0, // filled by caller or upstream
		ActiveTopicCount: len(topicData),
		TotalEngagement:  totalEngagement,
		TopTopicID:       topTopicID,
		SnapshotTime:     in.SnapshotTime,
	})

	return BuildSnapshotsResult{
		TopicSnapshotCount:   len(topicData),
		MonitorSnapshotCount: 1,
		TopTopicID:           topTopicID,
	}, nil
}
