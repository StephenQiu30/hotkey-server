package auth

import "context"

// Repository defines the interface for user persistence.
type Repository interface {
	ExistsByEmail(ctx context.Context, email string) bool
	Create(ctx context.Context, email, passwordHash, displayName string) (User, error)
	FindByEmail(ctx context.Context, email string) (User, string, error) // returns user + password_hash
}
