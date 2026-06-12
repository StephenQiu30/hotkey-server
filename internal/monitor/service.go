package monitor

import (
	"context"
	"errors"
)

var (
	ErrInvalidInterval = errors.New("poll interval must be one of: 5, 10, 15, 30 minutes")
	ErrNotFound        = errors.New("monitor not found")
)

var allowedIntervals = map[int]struct{}{5: {}, 10: {}, 15: {}, 30: {}}

type CreateMonitorInput struct {
	Name                string `json:"name"`
	QueryText           string `json:"query_text"`
	Language            string `json:"language"`
	Region              string `json:"region"`
	PollIntervalMinutes int    `json:"poll_interval_minutes"`
	AlertEnabled        bool   `json:"alert_enabled"`
}

type UpdateMonitorInput struct {
	Name                string `json:"name,omitempty"`
	PollIntervalMinutes int    `json:"poll_interval_minutes,omitempty"`
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, userID int64, input CreateMonitorInput) (Monitor, error) {
	if _, ok := allowedIntervals[input.PollIntervalMinutes]; !ok {
		return Monitor{}, ErrInvalidInterval
	}
	if input.Language == "" {
		input.Language = "en"
	}
	if input.Region == "" {
		input.Region = "global"
	}
	return s.repo.Create(ctx, userID, input)
}

func (s *Service) ListByUser(ctx context.Context, userID int64) ([]Monitor, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *Service) GetByID(ctx context.Context, id int64) (Monitor, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateMonitorInput) (Monitor, error) {
	if input.PollIntervalMinutes != 0 {
		if _, ok := allowedIntervals[input.PollIntervalMinutes]; !ok {
			return Monitor{}, ErrInvalidInterval
		}
	}
	return s.repo.Update(ctx, id, input)
}

func (s *Service) Deactivate(ctx context.Context, id int64) error {
	return s.repo.Deactivate(ctx, id)
}
