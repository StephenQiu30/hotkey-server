package model

// TopicSummary is a display-oriented summary of a topic cluster.
type TopicSummary struct {
	ID             int64
	Title          string
	Summary        string
	CurrentHeat    float64
	TrendDirection string
	PostCount      int
}
