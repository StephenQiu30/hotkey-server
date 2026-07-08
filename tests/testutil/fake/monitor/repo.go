package fakemonitor

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/pkg"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

// Repo is an in-memory fake implementing service.MonitorRepository.
type Repo struct {
	Monitors []dto.Monitor
	NextID   int64
}

func (r *Repo) Create(_ context.Context, userID int64, input dto.CreateMonitorInput) (dto.Monitor, error) {
	r.NextID++
	now := time.Now()
	m := dto.Monitor{
		ID:                  r.NextID,
		UserID:              userID,
		Name:                input.Name,
		QueryText:           input.QueryText,
		Language:            input.Language,
		Region:              input.Region,
		Status:              "active",
		PollIntervalMinutes: input.PollIntervalMinutes,
		AlertEnabled:        input.AlertEnabled,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	r.Monitors = append(r.Monitors, m)
	return m, nil
}

func (r *Repo) GetByID(_ context.Context, id int64) (*dto.Monitor, error) {
	for i := range r.Monitors {
		if r.Monitors[i].ID == id {
			m := r.Monitors[i]
			return &m, nil
		}
	}
	return nil, nil
}

func (r *Repo) ListByUser(_ context.Context, userID int64) ([]dto.Monitor, error) {
	var out []dto.Monitor
	for _, m := range r.Monitors {
		if m.UserID == userID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *Repo) ListActive(_ context.Context) ([]dto.Monitor, error) {
	var out []dto.Monitor
	for _, m := range r.Monitors {
		if m.Status == "active" {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *Repo) Update(_ context.Context, id int64, userID int64, input dto.UpdateMonitorInput) (dto.Monitor, error) {
	for i := range r.Monitors {
		if r.Monitors[i].ID == id && r.Monitors[i].UserID == userID {
			if input.Name != nil {
				r.Monitors[i].Name = *input.Name
			}
			if input.QueryText != nil {
				r.Monitors[i].QueryText = *input.QueryText
			}
			if input.Status != nil {
				r.Monitors[i].Status = *input.Status
			}
			if input.PollIntervalMinutes != nil {
				r.Monitors[i].PollIntervalMinutes = *input.PollIntervalMinutes
			}
			r.Monitors[i].UpdatedAt = time.Now()
			return r.Monitors[i], nil
		}
	}
	return dto.Monitor{}, service.MonitorErrNotFound
}

func (r *Repo) SetQueryEmbedding(_ context.Context, id int64, _ pkg.Vector384) error {
	for i := range r.Monitors {
		if r.Monitors[i].ID == id {
			return nil
		}
	}
	return service.MonitorErrNotFound
}
