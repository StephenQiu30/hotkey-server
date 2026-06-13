package fakeserver

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// PostQueryService stubs content.PostQueryService for wiring tests.
type PostQueryService struct {
	Err error
}

func (s *PostQueryService) ListPostsByMonitor(_ int64, _, _ int) ([]content.PostSummary, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return nil, nil
}

// TopicQueryService stubs topic.TopicQueryService for wiring tests.
type TopicQueryService struct {
	Err error
}

func (s *TopicQueryService) ListByMonitor(_ int64) ([]topic.TopicSummary, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return nil, nil
}

// TrendQueryService stubs trend.TrendQueryService for wiring tests.
type TrendQueryService struct {
	Err error
}

func (s *TrendQueryService) GetTopicTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return nil, nil
}

func (s *TrendQueryService) GetMonitorTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return nil, nil
}
