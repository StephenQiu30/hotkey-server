package application

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type Repository interface {
	ListAfter(context.Context, domain.NotificationQuery) (domain.NotificationPage, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("notification repository is required")
	}
	return &Service{repository: repository}, nil
}

type ListInput struct {
	Role    domain.AudienceRole
	AfterID int64
	Limit   int
}

func (service *Service) ListAfter(ctx context.Context, input ListInput) (domain.NotificationPage, error) {
	query := (domain.NotificationQuery{Role: input.Role, AfterID: input.AfterID, Limit: input.Limit}).Normalized()
	if err := query.Validate(); err != nil {
		return domain.NotificationPage{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	page, err := service.repository.ListAfter(ctx, query)
	if err != nil {
		return domain.NotificationPage{}, err
	}
	return page, nil
}
