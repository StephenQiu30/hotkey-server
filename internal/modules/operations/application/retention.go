package application

import (
	"context"
	"fmt"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	"time"
)

type RetentionStore interface {
	ApplyRetention(context.Context, operationsdomain.RetentionPolicy, time.Time) (int64, error)
}
type RetentionService struct {
	store RetentionStore
	now   func() time.Time
}

func NewRetentionService(store RetentionStore) *RetentionService {
	return &RetentionService{store: store, now: func() time.Time { return time.Now().UTC() }}
}
func (service *RetentionService) Run(ctx context.Context, policy operationsdomain.RetentionPolicy) (operationsdomain.CleanupResult, error) {
	if service == nil || service.store == nil {
		return operationsdomain.CleanupResult{}, fmt.Errorf("retention store is required")
	}
	if err := policy.Validate(); err != nil {
		return operationsdomain.CleanupResult{}, err
	}
	cutoff := service.now().AddDate(0, 0, -policy.RetentionDays)
	affected, err := service.store.ApplyRetention(ctx, policy, cutoff)
	if err != nil {
		return operationsdomain.CleanupResult{}, err
	}
	return operationsdomain.CleanupResult{DataClass: policy.DataClass, Cutoff: cutoff, Affected: affected}, nil
}
