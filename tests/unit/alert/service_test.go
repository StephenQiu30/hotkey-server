package alert_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/alert"
)

func TestEvaluateCreatesAlertWhenVelocityExceedsThreshold(t *testing.T) {
	svc := alert.NewService(alert.DefaultRules())
	alerts := svc.EvaluateTopic(alert.TopicSignal{
		MonitorID:   1,
		TopicID:     2,
		Title:       "openai agent launch",
		CurrentHeat: 180,
		PreviousHeat: 90,
		Velocity:    1.0,
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].AlertType != "velocity_spike" {
		t.Errorf("expected alert type velocity_spike, got %q", alerts[0].AlertType)
	}
	if alerts[0].MonitorID != 1 {
		t.Errorf("expected monitor ID 1, got %d", alerts[0].MonitorID)
	}
	if alerts[0].TopicID != 2 {
		t.Errorf("expected topic ID 2, got %d", alerts[0].TopicID)
	}
}

func TestEvaluateNoAlertWhenVelocityBelowThreshold(t *testing.T) {
	svc := alert.NewService(alert.DefaultRules())
	alerts := svc.EvaluateTopic(alert.TopicSignal{
		MonitorID:   1,
		TopicID:     2,
		Title:       "slow topic",
		CurrentHeat: 95,
		PreviousHeat: 90,
		Velocity:    0.05,
	})
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestEvaluateCreatesAlertForHighHeatContent(t *testing.T) {
	svc := alert.NewService(alert.DefaultRules())
	alerts := svc.EvaluateTopic(alert.TopicSignal{
		MonitorID:     1,
		TopicID:       3,
		Title:         "viral content burst",
		CurrentHeat:   500,
		PreviousHeat:  100,
		Velocity:      4.0,
		HighHeatCount: 12,
	})
	// Should trigger both velocity_spike and high_heat_burst
	if len(alerts) < 2 {
		t.Fatalf("expected at least 2 alerts, got %d", len(alerts))
	}
}

func TestEvaluateCreatesAlertForFirstViralContent(t *testing.T) {
	svc := alert.NewService(alert.DefaultRules())
	alerts := svc.EvaluateTopic(alert.TopicSignal{
		MonitorID:    1,
		TopicID:      4,
		Title:        "first viral hit",
		CurrentHeat:  200,
		PreviousHeat: 0,
		Velocity:     0,
		IsFirstViral: true,
	})
	found := false
	for _, a := range alerts {
		if a.AlertType == "first_viral" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected first_viral alert type")
	}
}

func TestDefaultRulesHasThreeRules(t *testing.T) {
	rules := alert.DefaultRules()
	if len(rules) != 3 {
		t.Fatalf("expected 3 default rules, got %d", len(rules))
	}
}
