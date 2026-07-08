package monitor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	fakemonitor "github.com/StephenQiu30/hotkey-server/tests/testutil/fake/monitor"
)

func TestCreateMonitorRejectsInvalidInterval(t *testing.T) {
	repo := &fakemonitor.Repo{NextID: 1}
	svc := monitor.NewService(repo, nil)
	_, err := svc.Create(context.Background(), 1, dto.CreateMonitorInput{
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
	svc := monitor.NewService(repo, nil)
	_, err := svc.Create(context.Background(), 1, dto.CreateMonitorInput{
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
	svc := monitor.NewService(repo, nil)
	_, err := svc.Create(context.Background(), 1, dto.CreateMonitorInput{
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
	svc := monitor.NewService(repo, nil)
	m, err := svc.Create(context.Background(), 1, dto.CreateMonitorInput{
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
	svc := monitor.NewService(repo, nil)
	_, _ = svc.Create(context.Background(), 1, dto.CreateMonitorInput{
		Name: "M1", QueryText: "q1", PollIntervalMinutes: 5,
	})
	_, _ = svc.Create(context.Background(), 1, dto.CreateMonitorInput{
		Name: "M2", QueryText: "q2", PollIntervalMinutes: 10,
	})
	_, _ = svc.Create(context.Background(), 2, dto.CreateMonitorInput{
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

func TestServiceListActive(t *testing.T) {
	repo := &fakemonitor.Repo{NextID: 1}
	svc := monitor.NewService(repo, nil)
	m1, _ := svc.Create(context.Background(), 1, dto.CreateMonitorInput{
		Name: "Active1", QueryText: "q1", PollIntervalMinutes: 5,
	})
	_, _ = svc.Create(context.Background(), 1, dto.CreateMonitorInput{
		Name: "Active2", QueryText: "q2", PollIntervalMinutes: 10,
	})
	_, _ = svc.Create(context.Background(), 2, dto.CreateMonitorInput{
		Name: "OtherUser", QueryText: "q3", PollIntervalMinutes: 15,
	})

	got, err := svc.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 active monitors (all are active by default), got %d", len(got))
	}

	status := "paused"
	_, _ = svc.Update(context.Background(), m1.ID, 1, dto.UpdateMonitorInput{
		Status: &status,
	})

	got, err = svc.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 active monitors after pausing one, got %d", len(got))
	}
	if got[0].Name != "Active2" && got[1].Name != "Active2" {
		t.Fatalf("expected Active2 to be among active monitors, got %+v", got)
	}
	if got[0].Name != "OtherUser" && got[1].Name != "OtherUser" {
		t.Fatalf("expected OtherUser to be among active monitors, got %+v", got)
	}
}

func TestUpdateMonitorStatus(t *testing.T) {
	repo := &fakemonitor.Repo{NextID: 1}
	svc := monitor.NewService(repo, nil)
	m, _ := svc.Create(context.Background(), 1, dto.CreateMonitorInput{
		Name: "AI", QueryText: "openai", PollIntervalMinutes: 10,
	})

	status := "paused"
	updated, err := svc.Update(context.Background(), m.ID, 1, dto.UpdateMonitorInput{
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
	svc := monitor.NewService(repo, nil)
	status := "paused"
	_, err := svc.Update(context.Background(), 999, 1, dto.UpdateMonitorInput{
		Status: &status,
	})
	if !errors.Is(err, monitor.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
