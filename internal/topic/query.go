package topic

// TopicQueryService abstracts the read side for topic queries.
type TopicQueryService interface {
	ListByMonitor(monitorID int64) ([]TopicSummary, error)
}
