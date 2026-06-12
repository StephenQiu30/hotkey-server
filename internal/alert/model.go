// Package alert implements rule-based alert evaluation for topic signals.
package alert

import "time"

// TopicSignal holds metrics for a topic at a point in time.
type TopicSignal struct {
	MonitorID    int64
	TopicID      int64
	Title        string
	CurrentHeat  float64
	PreviousHeat float64
	Velocity     float64
	HighHeatCount int
	IsFirstViral  bool
}

// Alert represents a generated alert event.
type Alert struct {
	ID           int64
	MonitorID    int64
	TopicID      int64
	AlertType    string
	Title        string
	Message      string
	Severity     string
	TriggerScore float64
	TriggerReason string
	CreatedAt    time.Time
}

// Rule defines a condition that evaluates a TopicSignal and may produce an Alert.
type Rule struct {
	Name      string
	AlertType string
	Evaluate  func(TopicSignal) *Alert
}
