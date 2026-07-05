package topic

import "context"

// TopicQueryService abstracts the read side for topic queries.
type TopicQueryService interface {
	ListByMonitor(monitorID int64) ([]TopicSummary, error)
	// GetMonitorID returns the monitor ID that owns the given topic.
	GetMonitorID(ctx context.Context, topicID int64) (int64, error)
}
