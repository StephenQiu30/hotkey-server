package application

import (
	"context"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type UserNotificationDTO struct {
	ID              int64
	Version         int64
	OutboxEventID   int64
	UserID          int64
	MonitorID       int64
	EventType       string
	ResourceType    string
	ResourceID      int64
	ResourceVersion int64
	OccurredAt      time.Time
	Title           string
	Summary         string
	ResourceStatus  string
	DeepLink        string
	CreatedAt       time.Time
}

type ListUserNotificationsQuery struct {
	UserID    int64
	MonitorID *int64
	AfterID   int64
	Limit     int
}

type ListUserNotificationsResult struct {
	Items       []UserNotificationDTO
	NextAfterID int64
}

type RecordNotificationDeliveryAttemptCommand struct {
	UserNotificationID int64
	UserID             int64
	Channel            string
	DeliveryTargetKey  string
	Status             string
	ProviderMessageID  string
	ResponseCode       *int
	ErrorCode          string
	AttemptedAt        time.Time
}

type RecordNotificationDeliveryAttemptResult struct {
	DeliveryAttemptID int64
	AttemptNo         int
}

type ProjectUserNotificationCommand struct {
	OutboxEventID int64
	OutboxVersion int64
}

type ProjectUserNotificationResult struct {
	UserNotificationID int64
	Created            bool
}

type Repository interface {
	ListUserNotifications(context.Context, ListUserNotificationsQuery) (ListUserNotificationsResult, error)
	RecordDeliveryAttempt(context.Context, RecordNotificationDeliveryAttemptCommand) (RecordNotificationDeliveryAttemptResult, error)
	ProjectUserNotification(context.Context, ProjectUserNotificationCommand) (ProjectUserNotificationResult, error)
}

func (service *Service) ProjectUserNotification(ctx context.Context, command ProjectUserNotificationCommand) (ProjectUserNotificationResult, error) {
	if service == nil || service.repository == nil {
		return ProjectUserNotificationResult{}, sharedrepository.ErrUnavailable
	}
	if command.OutboxEventID <= 0 || command.OutboxVersion != 1 {
		return ProjectUserNotificationResult{}, sharedrepository.ErrInvalidInput
	}
	result, err := service.repository.ProjectUserNotification(ctx, command)
	if err != nil {
		return ProjectUserNotificationResult{}, err
	}
	if result.UserNotificationID <= 0 {
		return ProjectUserNotificationResult{}, sharedrepository.ErrConstraint
	}
	return result, nil
}

type Service struct{ repository Repository }

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("user notification repository is required")
	}
	return &Service{repository: repository}, nil
}

func (service *Service) ListUserNotifications(ctx context.Context, query ListUserNotificationsQuery) (ListUserNotificationsResult, error) {
	normalized := domain.UserNotificationQuery{
		UserID: query.UserID, MonitorID: query.MonitorID, AfterID: query.AfterID, Limit: query.Limit,
	}.Normalized()
	if err := normalized.Validate(); err != nil {
		return ListUserNotificationsResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	result, err := service.repository.ListUserNotifications(ctx, ListUserNotificationsQuery{
		UserID: normalized.UserID, MonitorID: normalized.MonitorID, AfterID: normalized.AfterID, Limit: normalized.Limit,
	})
	if err != nil {
		return ListUserNotificationsResult{}, err
	}
	if result.NextAfterID < normalized.AfterID || len(result.Items) > normalized.Limit {
		return ListUserNotificationsResult{}, sharedrepository.ErrConstraint
	}
	for _, item := range result.Items {
		if item.UserID != normalized.UserID {
			return ListUserNotificationsResult{}, sharedrepository.ErrConstraint
		}
		if normalized.MonitorID != nil && item.MonitorID != *normalized.MonitorID {
			return ListUserNotificationsResult{}, sharedrepository.ErrConstraint
		}
		if err := ValidateUserNotificationDTO(item); err != nil {
			return ListUserNotificationsResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
		}
	}
	return result, nil
}

func (service *Service) RecordDeliveryAttempt(ctx context.Context, command RecordNotificationDeliveryAttemptCommand) (RecordNotificationDeliveryAttemptResult, error) {
	if err := ValidateNotificationDeliveryAttemptCommand(command); err != nil {
		return RecordNotificationDeliveryAttemptResult{}, err
	}
	result, err := service.repository.RecordDeliveryAttempt(ctx, command)
	if err != nil {
		return RecordNotificationDeliveryAttemptResult{}, err
	}
	if result.DeliveryAttemptID <= 0 || result.AttemptNo <= 0 {
		return RecordNotificationDeliveryAttemptResult{}, sharedrepository.ErrConstraint
	}
	return result, nil
}

func ValidateNotificationDeliveryAttemptCommand(command RecordNotificationDeliveryAttemptCommand) error {
	attempt := domain.NotificationDeliveryAttempt{
		UserNotificationID: command.UserNotificationID,
		Channel:            domain.DeliveryChannel(command.Channel),
		DeliveryTargetKey:  command.DeliveryTargetKey,
		Status:             domain.DeliveryStatus(command.Status),
		ProviderMessageID:  command.ProviderMessageID,
		ResponseCode:       command.ResponseCode,
		ErrorCode:          command.ErrorCode,
		AttemptedAt:        command.AttemptedAt,
	}
	if command.UserID <= 0 {
		return sharedrepository.ErrInvalidInput
	}
	if err := attempt.Validate(); err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return nil
}

func ValidateUserNotificationDTO(item UserNotificationDTO) error {
	return (domain.UserNotification{
		ID: item.ID, Version: item.Version, OutboxEventID: item.OutboxEventID, UserID: item.UserID,
		MonitorID: item.MonitorID, EventType: domain.UserNotificationEventType(item.EventType),
		ResourceType: item.ResourceType, ResourceID: item.ResourceID, ResourceVersion: item.ResourceVersion,
		OccurredAt: item.OccurredAt, Title: item.Title, Summary: item.Summary, ResourceStatus: item.ResourceStatus,
		DeepLink: item.DeepLink, CreatedAt: item.CreatedAt,
	}).Validate()
}
