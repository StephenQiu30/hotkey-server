package faketrend

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// QueryService is a fake implementing trend.TrendQueryService.
type QueryService struct {
	Snapshots []trend.TrendPoint
	Err       error
}

func (s *QueryService) GetTopicTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Snapshots, nil
}

func (s *QueryService) GetMonitorTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Snapshots, nil
}
