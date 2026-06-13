package monitor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	fakemonitor "github.com/StephenQiu30/hotkey-server/tests/testutil/fake/monitor"
)

func TestCreateMonitorRejectsInvalidInterval(t *testing.T) {
	repo := &fakemonitor.Repo{NextID: 1}
	svc := monitor.NewService(repo)
	_, err := svc.Create(context.Background(), 1, monitor.CreateMonitorInput{
		Name:                "AI",
		QueryText:           "openai agent",
		PollIntervalMinutes: 7,
	})
	if !errors.Is(err, monitor.ErrInvalidInterval) {
		t.Fatalf("expected ErrInvalidInterval, got %v", err)
	}
}

func TestCreateMonitorRejectsEmptyName(t *testing.T) {
	repo := &fakemonitor.Repo{NextID: 1}
	svc := monitor.NewService(repo)
	_, err := svc.Create(context.Background(), 1, monitor.CreateMonitorInput{
		Name:                "",
		QueryText:           "openai",
		PollIntervalMinutes: 10,
	})
	if !errors.Is(err, monitor.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateMonitorRejectsEmptyQuery(t *testing.T) {
	repo := &fakemonitor.Repo{NextID: 1}
	svc := monitor.NewService(repo)
	_, err := svc.Create(context.Background(), 1, monitor.CreateMonitorInput{
		Name:                "AI",
		QueryText:           "",
		PollIntervalMinutes: 10,
	})
	if !errors.Is(err, monitor.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateMonitorSuccess(t *testing.T) {
	repo := &fakemonitor.Repo{NextID: 1}
	svc := monitor.NewService(repo)
	m, err := svc.Create(context.Background(), 1, monitor.CreateMonitorInput{
		Name:                "AI News",
		QueryText:           "openai agent",
		Language:            "en",
		Region:              "global",
		PollIntervalMinutes: 10,
		AlertEnabled:        true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "AI News" {
		t.Fatalf("expected name AI News, got %s", m.Name)
	}
	if m.Status != "active" {
		t.Fatalf("expected status active, got %s", m.Status)
	}
}

func TestListMonitorsByUser(t *testing.T) {
	repo := &fakemonitor.Repo{NextID: 1}
	svc := monitor.NewService(repo)
	_, _ = svc.Create(context.Background(), 1, monitor.CreateMonitorInput{
		Name: "M1", QueryText: "q1", PollIntervalMinutes: 5,
	})
	_, _ = svc.Create(context.Background(), 1, monitor.CreateMonitorInput{
		Name: "M2", QueryText: "q2", PollIntervalMinutes: 10,
	})
	_, _ = svc.Create(context.Background(), 2, monitor.CreateMonitorInput{
		Name: "M3", QueryText: "q3", PollIntervalMinutes: 15,
	})

	monitors, err := svc.ListByUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}
}

func TestUpdateMonitorStatus(t *testing.T) {
	repo := &fakemonitor.Repo{NextID: 1}
	svc := monitor.NewService(repo)
	m, _ := svc.Create(context.Background(), 1, monitor.CreateMonitorInput{
		Name: "AI", QueryText: "openai", PollIntervalMinutes: 10,
	})

	status := "paused"
	updated, err := svc.Update(context.Background(), m.ID, monitor.UpdateMonitorInput{
		Status: &status,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != "paused" {
		t.Fatalf("expected status paused, got %s", updated.Status)
	}
}

func TestUpdateMonitorNotFound(t *testing.T) {
	repo := &fakemonitor.Repo{NextID: 1}
	svc := monitor.NewService(repo)
	status := "paused"
	_, err := svc.Update(context.Background(), 999, monitor.UpdateMonitorInput{
		Status: &status,
	})
	if !errors.Is(err, monitor.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
