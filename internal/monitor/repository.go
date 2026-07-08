package monitor

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

// Repository defines the persistence interface for monitor operations.
type Repository interface {
	// Create inserts a new monitor and returns the created Monitor.
	Create(ctx context.Context, userID int64, input CreateMonitorInput) (Monitor, error)

	// GetByID retrieves a monitor by ID. Returns nil if not found.
	GetByID(ctx context.Context, id int64) (*Monitor, error)

	// ListByUser retrieves all monitors for a given user.
	ListByUser(ctx context.Context, userID int64) ([]Monitor, error)

	// ListActive retrieves all active monitors regardless of user.
	ListActive(ctx context.Context) ([]Monitor, error)

	// Update modifies an existing monitor owned by the given user.
	Update(ctx context.Context, id int64, userID int64, input UpdateMonitorInput) (Monitor, error)

	// SetQueryEmbedding stores the embedding vector for a monitor's query text.
	SetQueryEmbedding(ctx context.Context, id int64, emb pkg.Vector384) error
}
