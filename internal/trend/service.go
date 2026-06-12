// Package trend implements trend snapshot generation and velocity computation.
// It produces point-in-time snapshots for topics and monitors, and calculates
// trend velocity to determine rising/falling/flat direction.
package trend

import "time"

// TopicSnapshot represents a point-in-time snapshot of a topic.
type TopicSnapshot struct {
	TopicID          int64
	SnapshotTime     time.Time
	PostCount        int
	UniqueAuthorCount int
	EngagementSum    int
	HeatScore        float64
	TrendVelocity    float64
	TrendDirection   string
}

// MonitorSnapshot represents a point-in-time snapshot of a monitor.
type MonitorSnapshot struct {
	MonitorID        int64
	SnapshotTime     time.Time
	NewPostCount     int
	ActiveTopicCount int
	TotalEngagement  int
	TopTopicID       int64
}

// TopicSnapshotInput holds inputs for building a topic snapshot.
type TopicSnapshotInput struct {
	TopicID          int64
	PostCount        int
	UniqueAuthorCount int
	EngagementSum    int
	HeatScore        float64
	PreviousHeat     float64
	SnapshotTime     time.Time
}

// MonitorSnapshotInput holds inputs for building a monitor snapshot.
type MonitorSnapshotInput struct {
	MonitorID        int64
	NewPostCount     int
	ActiveTopicCount int
	TotalEngagement  int
	TopTopicID       int64
	SnapshotTime     time.Time
}

// Repository abstracts persistence for snapshot tables.
type Repository interface {
	SaveTopicSnapshot(snap TopicSnapshot) error
	SaveMonitorSnapshot(snap MonitorSnapshot) error
	GetPreviousTopicHeat(topicID int64) (float64, error)
}

// Service provides trend computation operations.
type Service struct {
	repo Repository
}

// NewService creates a trend Service with the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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

// BuildTopicSnapshot creates a topic snapshot with computed velocity and direction.
func (s *Service) BuildTopicSnapshot(in TopicSnapshotInput) TopicSnapshot {
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

// BuildMonitorSnapshot creates a monitor-level snapshot.
func (s *Service) BuildMonitorSnapshot(in MonitorSnapshotInput) MonitorSnapshot {
	return MonitorSnapshot{
		MonitorID:        in.MonitorID,
		SnapshotTime:     in.SnapshotTime,
		NewPostCount:     in.NewPostCount,
		ActiveTopicCount: in.ActiveTopicCount,
		TotalEngagement:  in.TotalEngagement,
		TopTopicID:       in.TopTopicID,
	}
}
