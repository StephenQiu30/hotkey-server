package monitor

import "context"

// Repository defines the persistence interface for monitor operations.
type Repository interface {
	// Create inserts a new monitor and returns the created Monitor.
	Create(ctx context.Context, userID int64, input CreateMonitorInput) (Monitor, error)

	// GetByID retrieves a monitor by ID. Returns nil if not found.
	GetByID(ctx context.Context, id int64) (*Monitor, error)

	// ListByUser retrieves all monitors for a given user.
	ListByUser(ctx context.Context, userID int64) ([]Monitor, error)

	// Update modifies an existing monitor.
	Update(ctx context.Context, id int64, input UpdateMonitorInput) (Monitor, error)
}
