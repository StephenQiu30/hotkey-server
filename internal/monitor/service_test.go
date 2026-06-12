package monitor

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepo is a test double for the monitor Repository.
type fakeRepo struct {
	monitors []Monitor
	nextID   int64
}

func (r *fakeRepo) Create(_ context.Context, userID int64, input CreateMonitorInput) (Monitor, error) {
	r.nextID++
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
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}
	r.monitors = append(r.monitors, m)
	return m, nil
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

func (r *fakeRepo) GetByID(_ context.Context, id int64) (Monitor, error) {
	for _, m := range r.monitors {
		if m.ID == id {
			return m, nil
		}
	}
	return Monitor{}, errors.New("not found")
}

func (r *fakeRepo) Update(_ context.Context, id int64, input UpdateMonitorInput) (Monitor, error) {
	for i, m := range r.monitors {
		if m.ID == id {
			if input.Name != "" {
				r.monitors[i].Name = input.Name
			}
			if input.PollIntervalMinutes != 0 {
				r.monitors[i].PollIntervalMinutes = input.PollIntervalMinutes
			}
			r.monitors[i].UpdatedAt = time.Now()
			return r.monitors[i], nil
		}
	}
	return Monitor{}, errors.New("not found")
}

func (r *fakeRepo) Deactivate(_ context.Context, id int64) error {
	for i, m := range r.monitors {
		if m.ID == id {
			r.monitors[i].Status = "inactive"
			r.monitors[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return errors.New("not found")
}

func TestCreateMonitorRejectsInvalidInterval(t *testing.T) {
	repo := &fakeRepo{}
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

func TestCreateMonitorAcceptsValidInterval(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	m, err := svc.Create(context.Background(), 1, CreateMonitorInput{
		Name:                "AI",
		QueryText:           "openai agent",
		PollIntervalMinutes: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.PollIntervalMinutes != 10 {
		t.Fatalf("expected interval 10, got %d", m.PollIntervalMinutes)
	}
}

func TestDeactivateMonitor(t *testing.T) {
	repo := &fakeRepo{
		monitors: []Monitor{{ID: 1, UserID: 1, Status: "active"}},
	}
	svc := NewService(repo)
	if err := svc.Deactivate(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.monitors[0].Status != "inactive" {
		t.Fatalf("expected inactive, got %s", repo.monitors[0].Status)
	}
}
