// Package jobs implements background job orchestration.
package jobs

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

type TopicData struct {
	TopicID           int64
	PostCount         int
	UniqueAuthorCount int
	EngagementSum     int
	HeatScore         float64
	PreviousHeat      float64
}

type TopicProvider interface {
	GetTopicDataForMonitor(monitorID int64) ([]TopicData, error)
}

type TrendSnapshotter interface {
	BuildTopicSnapshot(in trend.TopicSnapshotInput) trend.TopicSnapshot
	BuildMonitorSnapshot(in trend.MonitorSnapshotInput) trend.MonitorSnapshot
}

type BuildSnapshotsInput struct {
	MonitorID    int64
	SnapshotTime time.Time
}

type BuildSnapshotsResult struct {
	TopicSnapshotCount   int
	MonitorSnapshotCount int
	TopTopicID           int64
}

type BuildSnapshotsJob struct {
	trend   TrendSnapshotter
	topics  TopicProvider
}

func NewBuildSnapshotsJob(trend TrendSnapshotter, topics TopicProvider) *BuildSnapshotsJob {
	return &BuildSnapshotsJob{trend: trend, topics: topics}
}

// Run executes the snapshot job for a monitor.
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
