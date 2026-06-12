package monitor

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo is an in-memory implementation of Repository for testing.
type fakeRepo struct {
	monitors []Monitor
	nextID   int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{nextID: 1}
}

func (r *fakeRepo) Create(_ context.Context, userID int64, input CreateMonitorInput) (Monitor, error) {
	m := Monitor{
		ID:                  r.nextID,
		UserID:              userID,
		Name:                input.Name,
		QueryText:           input.QueryText,
		Language:            input.Language,
		Region:              input.Region,
		Status:              "active",
		PollIntervalMinutes: input.PollIntervalMinutes,
		AlertEnabled:        input.AlertEnabled,
	}
	r.nextID++
	r.monitors = append(r.monitors, m)
	return m, nil
}

func (r *fakeRepo) GetByID(_ context.Context, id int64) (*Monitor, error) {
	for _, m := range r.monitors {
		if m.ID == id {
			return &m, nil
		}
	}
	return nil, nil
}

func (r *fakeRepo) ListByUser(_ context.Context, userID int64) ([]Monitor, error) {
	var result []Monitor
	for _, m := range r.monitors {
		if m.UserID == userID {
			result = append(result, m)
		}
	}
	return result, nil
}

func (r *fakeRepo) Update(_ context.Context, id int64, input UpdateMonitorInput) (Monitor, error) {
	for i, m := range r.monitors {
		if m.ID == id {
			if input.Name != nil {
				r.monitors[i].Name = *input.Name
			}
			if input.Status != nil {
				r.monitors[i].Status = *input.Status
			}
			if input.PollIntervalMinutes != nil {
				r.monitors[i].PollIntervalMinutes = *input.PollIntervalMinutes
			}
			return r.monitors[i], nil
		}
	}
	return Monitor{}, ErrNotFound
}

func TestCreateMonitorRejectsInvalidInterval(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	_, err := svc.Create(context.Background(), 1, CreateMonitorInput{
		Name:                "AI",
		QueryText:           "openai agent",
		PollIntervalMinutes: 7,
	})
	if !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("expected ErrInvalidInterval, got %v", err)
	}
}

func TestCreateMonitorRejectsEmptyName(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	_, err := svc.Create(context.Background(), 1, CreateMonitorInput{
		Name:                "",
		QueryText:           "openai",
		PollIntervalMinutes: 10,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateMonitorRejectsEmptyQuery(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	_, err := svc.Create(context.Background(), 1, CreateMonitorInput{
		Name:                "AI",
		QueryText:           "",
		PollIntervalMinutes: 10,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateMonitorSuccess(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	m, err := svc.Create(context.Background(), 1, CreateMonitorInput{
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
	repo := newFakeRepo()
	svc := NewService(repo)
	_, _ = svc.Create(context.Background(), 1, CreateMonitorInput{
		Name: "M1", QueryText: "q1", PollIntervalMinutes: 5,
	})
	_, _ = svc.Create(context.Background(), 1, CreateMonitorInput{
		Name: "M2", QueryText: "q2", PollIntervalMinutes: 10,
	})
	_, _ = svc.Create(context.Background(), 2, CreateMonitorInput{
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
	repo := newFakeRepo()
	svc := NewService(repo)
	m, _ := svc.Create(context.Background(), 1, CreateMonitorInput{
		Name: "AI", QueryText: "openai", PollIntervalMinutes: 10,
	})

	status := "paused"
	updated, err := svc.Update(context.Background(), m.ID, UpdateMonitorInput{
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
	repo := newFakeRepo()
	svc := NewService(repo)
	status := "paused"
	_, err := svc.Update(context.Background(), 999, UpdateMonitorInput{
		Status: &status,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
