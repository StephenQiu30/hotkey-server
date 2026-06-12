package alert

import "fmt"

// DefaultVelocityThreshold is the minimum velocity ratio to trigger a spike alert.
const DefaultVelocityThreshold = 0.5

// DefaultHighHeatCount is the minimum high-heat post count to trigger a burst alert.
const DefaultHighHeatCount = 10

// Service evaluates topic signals against alert rules.
type Service struct {
	rules []Rule
}

// NewService creates a new alert Service with the given rules.
func NewService(rules []Rule) *Service {
	return &Service{rules: rules}
}

// DefaultRules returns the built-in alert rules.
func DefaultRules() []Rule {
	return []Rule{
		{
			Name:      "velocity_spike",
			AlertType: "velocity_spike",
			Evaluate: func(sig TopicSignal) *Alert {
				if sig.Velocity < DefaultVelocityThreshold {
					return nil
				}
				return &Alert{
					MonitorID:     sig.MonitorID,
					TopicID:       sig.TopicID,
					AlertType:     "velocity_spike",
					Title:         fmt.Sprintf("Topic %q heating up", sig.Title),
					Message:       fmt.Sprintf("Heat velocity %.2f exceeds threshold", sig.Velocity),
					Severity:      "warning",
					TriggerScore:  sig.Velocity,
					TriggerReason: "velocity_spike",
				}
			},
		},
		{
			Name:      "high_heat_burst",
			AlertType: "high_heat_burst",
			Evaluate: func(sig TopicSignal) *Alert {
				if sig.HighHeatCount < DefaultHighHeatCount {
					return nil
				}
				return &Alert{
					MonitorID:     sig.MonitorID,
					TopicID:       sig.TopicID,
					AlertType:     "high_heat_burst",
					Title:         fmt.Sprintf("High-heat burst in %q", sig.Title),
					Message:       fmt.Sprintf("%d high-heat posts detected", sig.HighHeatCount),
					Severity:      "critical",
					TriggerScore:  float64(sig.HighHeatCount),
					TriggerReason: "high_heat_burst",
				}
			},
		},
		{
			Name:      "first_viral",
			AlertType: "first_viral",
			Evaluate: func(sig TopicSignal) *Alert {
				if !sig.IsFirstViral {
					return nil
				}
				return &Alert{
					MonitorID:     sig.MonitorID,
					TopicID:       sig.TopicID,
					AlertType:     "first_viral",
					Title:         fmt.Sprintf("First viral content in %q", sig.Title),
					Message:       "First high-spread content detected for this topic",
					Severity:      "info",
					TriggerScore:  sig.CurrentHeat,
					TriggerReason: "first_viral",
				}
			},
		},
	}
}

// EvaluateTopic evaluates a TopicSignal against all rules and returns triggered alerts.
func (s *Service) EvaluateTopic(sig TopicSignal) []Alert {
	var alerts []Alert
	for _, rule := range s.rules {
		if alert := rule.Evaluate(sig); alert != nil {
			alerts = append(alerts, *alert)
		}
	}
	return alerts
}
