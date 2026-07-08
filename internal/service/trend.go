package service

import "time"

// TopicSnapshot represents a point-in-time snapshot of a topic's metrics.
type TopicSnapshot struct {
	TopicID           int64
	SnapshotTime      time.Time
	PostCount         int
	UniqueAuthorCount int
	EngagementSum     int
	HeatScore         float64
	TrendVelocity     float64
	TrendDirection    string
}

// MonitorSnapshot represents a point-in-time snapshot of a monitor's metrics.
type MonitorSnapshot struct {
	MonitorID        int64
	SnapshotTime     time.Time
	NewPostCount     int
	ActiveTopicCount int
	TotalEngagement  int
	TopTopicID       int64
}

// TopicSnapshotInput groups inputs for BuildTopicSnapshot.
type TopicSnapshotInput struct {
	TopicID           int64
	PostCount         int
	UniqueAuthorCount int
	EngagementSum     int
	HeatScore         float64
	PreviousHeat      float64
	SnapshotTime      time.Time
}

// MonitorSnapshotInput groups inputs for BuildMonitorSnapshot.
type MonitorSnapshotInput struct {
	MonitorID        int64
	NewPostCount     int
	ActiveTopicCount int
	TotalEngagement  int
	TopTopicID       int64
	SnapshotTime     time.Time
}

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

// ComputeVelocity returns the rate of change between previous and current values.
// Formula: (current - previous) / previous. Returns 1.0 when previous is 0.
func ComputeVelocity(current, previous float64) float64 {
	if previous == 0 {
		if current > 0 {
			return 1.0
		}
		return 0
	}
	return (current - previous) / previous
}

// DetermineTrendDirection maps velocity to a direction label.
// velocity > 0.05 => "rising", velocity < -0.05 => "falling", else "flat".
func DetermineTrendDirection(velocity float64) string {
	if velocity > 0.05 {
		return "rising"
	}
	if velocity < -0.05 {
		return "falling"
	}
	return "flat"
}

// BuildTopicSnapshot computes velocity and direction from input values.
func BuildTopicSnapshot(in TopicSnapshotInput) TopicSnapshot {
	velocity := ComputeVelocity(in.HeatScore, in.PreviousHeat)
	direction := DetermineTrendDirection(velocity)

	return TopicSnapshot{
		TopicID:           in.TopicID,
		SnapshotTime:      in.SnapshotTime,
		PostCount:         in.PostCount,
		UniqueAuthorCount: in.UniqueAuthorCount,
		EngagementSum:     in.EngagementSum,
		HeatScore:         in.HeatScore,
		TrendVelocity:     velocity,
		TrendDirection:    direction,
	}
}

// BuildMonitorSnapshot creates a monitor snapshot from input values.
func BuildMonitorSnapshot(in MonitorSnapshotInput) MonitorSnapshot {
	return MonitorSnapshot{
		MonitorID:        in.MonitorID,
		SnapshotTime:     in.SnapshotTime,
		NewPostCount:     in.NewPostCount,
		ActiveTopicCount: in.ActiveTopicCount,
		TotalEngagement:  in.TotalEngagement,
		TopTopicID:       in.TopTopicID,
	}
}
