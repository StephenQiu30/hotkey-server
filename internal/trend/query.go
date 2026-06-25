package trend

import "time"

// TrendPoint represents a single data point in a trend series.
type TrendPoint struct {
	Time           time.Time `json:"time"`
	HeatScore      float64   `json:"heat_score"`
	TrendVelocity  float64   `json:"trend_velocity"`
	TrendDirection string    `json:"trend_direction"`
}

// TrendQueryService abstracts the read side for trend queries.
type TrendQueryService interface {
	GetTopicTrends(topicID int64, since time.Time) ([]TrendPoint, error)
	GetMonitorTrends(monitorID int64, since time.Time) ([]TrendPoint, error)
}
