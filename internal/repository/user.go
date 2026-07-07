package repository

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
)

type UserRepository interface {
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	Create(ctx context.Context, email, passwordHash, displayName string) (model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	GetByID(ctx context.Context, id int64) (*model.User, error)
}
