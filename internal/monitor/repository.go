package monitor

import "context"

type Repository interface {
	Create(ctx context.Context, userID int64, input CreateMonitorInput) (Monitor, error)
	ListByUser(ctx context.Context, userID int64) ([]Monitor, error)
	GetByID(ctx context.Context, id int64) (Monitor, error)
	Update(ctx context.Context, id int64, input UpdateMonitorInput) (Monitor, error)
	Deactivate(ctx context.Context, id int64) error
}
