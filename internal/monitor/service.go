package monitor

import "context"

// Service provides keyword monitor operations.
type Service struct {
	repo Repository
}

// NewService creates a new monitor Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates and creates a new keyword monitor.
func (s *Service) Create(ctx context.Context, userID int64, input CreateMonitorInput) (Monitor, error) {
	if input.Name == "" || input.QueryText == "" {
		return Monitor{}, ErrInvalidInput
	}
	if _, ok := AllowedIntervals[input.PollIntervalMinutes]; !ok {
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

// GetByID retrieves a monitor by ID.
func (s *Service) GetByID(ctx context.Context, id int64) (Monitor, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Monitor{}, err
	}
	if m == nil {
		return Monitor{}, ErrNotFound
	}
	return *m, nil
}

// ListByUser retrieves all monitors for a user.
func (s *Service) ListByUser(ctx context.Context, userID int64) ([]Monitor, error) {
	return s.repo.ListByUser(ctx, userID)
}

// Update modifies an existing monitor owned by the given user.
func (s *Service) Update(ctx context.Context, id int64, userID int64, input UpdateMonitorInput) (Monitor, error) {
	if input.PollIntervalMinutes != nil {
		if _, ok := AllowedIntervals[*input.PollIntervalMinutes]; !ok {
			return Monitor{}, ErrInvalidInterval
		}
	}
	return s.repo.Update(ctx, id, userID, input)
}
