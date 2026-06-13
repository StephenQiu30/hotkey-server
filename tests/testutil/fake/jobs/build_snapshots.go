package fakejobs

import (
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// TrendService is a fake implementing jobs.TrendSnapshotter.
type TrendService struct {
	Err error
}

func (s *TrendService) BuildTopicSnapshot(in trend.TopicSnapshotInput) trend.TopicSnapshot {
	return trend.TopicSnapshot{
		TopicID:          in.TopicID,
		SnapshotTime:     in.SnapshotTime,
		PostCount:        in.PostCount,
		UniqueAuthorCount: in.UniqueAuthorCount,
		EngagementSum:    in.EngagementSum,
		HeatScore:        in.HeatScore,
	}
}

func (s *TrendService) BuildMonitorSnapshot(in trend.MonitorSnapshotInput) trend.MonitorSnapshot {
	return trend.MonitorSnapshot{
		MonitorID:        in.MonitorID,
		SnapshotTime:     in.SnapshotTime,
		NewPostCount:     in.NewPostCount,
		ActiveTopicCount: in.ActiveTopicCount,
		TotalEngagement:  in.TotalEngagement,
		TopTopicID:       in.TopTopicID,
	}
}

// TopicProvider is a fake implementing jobs.TopicProvider.
type TopicProvider struct {
	Data []jobs.TopicData
	Err  error
}

func (p *TopicProvider) GetTopicDataForMonitor(_ int64) ([]jobs.TopicData, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	return p.Data, nil
}
