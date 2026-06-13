package faketopic

import (
	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// QueryService is a fake implementing topic.TopicQueryService.
type QueryService struct {
	Topics []topic.TopicSummary
	Err    error
}

func (s *QueryService) ListByMonitor(_ int64) ([]topic.TopicSummary, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Topics, nil
}
