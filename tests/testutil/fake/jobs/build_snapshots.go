package fakejobs

import (
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// TrendService is a fake implementing jobs.TrendSnapshotter.
type TrendService struct {
	Err error
}

func (s *TrendService) BuildTopicSnapshot(in trend.TopicSnapshotInput) error {
	if s.Err != nil {
		return s.Err
	}
	return nil
}

func (s *TrendService) BuildMonitorSnapshot(in trend.MonitorSnapshotInput) error {
	if s.Err != nil {
		return s.Err
	}
	return nil
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
