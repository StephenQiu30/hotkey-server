package auth

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
)

// Repository defines the persistence interface for auth operations.
type Repository interface {
	// ExistsByEmail returns true if a user with the given email exists.
	ExistsByEmail(ctx context.Context, email string) bool

	// Create inserts a new user and returns the created User.
	Create(ctx context.Context, email, passwordHash, displayName string) (dto.User, error)

	// GetByEmail retrieves a user by email. Returns nil if not found.
	GetByEmail(ctx context.Context, email string) (*dto.User, error)

	// GetByID retrieves a user by ID. Returns nil if not found.
	GetByID(ctx context.Context, id int64) (*dto.User, error)
}
