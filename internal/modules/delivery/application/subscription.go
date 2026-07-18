package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/internal/shared/requestcontext"
)

type SubscriptionRepository interface {
	CreateSubscription(context.Context, domain.Subscription) (domain.Subscription, error)
	GetSubscription(context.Context, int64, int64) (domain.Subscription, error)
	ListSubscriptions(context.Context, int64, domain.SubscriptionListQuery) (domain.SubscriptionPage, error)
	UpdateSubscription(context.Context, domain.Subscription, int64) (domain.Subscription, error)
	RotateRSSToken(context.Context, int64, int64, int64, string) (domain.Subscription, error)
	DeleteSubscription(context.Context, int64, int64, int64) (domain.Subscription, error)
}

type TokenSource func() (string, error)

type SubscriptionDependencies struct {
	Runtime *database.Runtime
	Store   SubscriptionRepository
	Audit   operationsapplication.AuditWriter
	Token   TokenSource
}

type SubscriptionService struct {
	runtime *database.Runtime
	store   SubscriptionRepository
	audit   operationsapplication.AuditWriter
	token   TokenSource
}

func NewSubscriptionService(dependencies SubscriptionDependencies) (*SubscriptionService, error) {
	if dependencies.Runtime == nil || dependencies.Store == nil || dependencies.Audit == nil {
		return nil, fmt.Errorf("delivery subscription dependencies are required")
	}
	if dependencies.Token == nil {
		dependencies.Token = newRSSToken
	}
	return &SubscriptionService{runtime: dependencies.Runtime, store: dependencies.Store, audit: dependencies.Audit, token: dependencies.Token}, nil
}

type CreateSubscriptionInput struct {
	Subject    identitydomain.Subject
	MonitorID  *int64
	ReportType string
	Channel    domain.Channel
	Recipient  string
	Timezone   string
	Schedule   string
	Enabled    bool
}

type UpdateSubscriptionInput struct {
	Subject         identitydomain.Subject
	SubscriptionID  int64
	ExpectedVersion int64
	Recipient       *string
	Timezone        *string
	Schedule        *string
	Enabled         *bool
}

type RotateRSSTokenInput struct {
	Subject         identitydomain.Subject
	SubscriptionID  int64
	ExpectedVersion int64
}

type DeleteSubscriptionInput struct {
	Subject         identitydomain.Subject
	SubscriptionID  int64
	ExpectedVersion int64
}

// SubscriptionSecret is intentionally the only application result that can
// carry an RSS token. It is returned once at create/rotation time; persistence,
// logs, list and detail APIs use only the SHA-256 token hash.
type SubscriptionSecret struct {
	Subscription domain.Subscription
	RSSToken     string
}

func (service *SubscriptionService) Create(ctx context.Context, input CreateSubscriptionInput) (SubscriptionSecret, error) {
	if err := requireSubscriptionUser(input.Subject); err != nil {
		return SubscriptionSecret{}, err
	}
	reportType := strings.TrimSpace(input.ReportType)
	if reportType == "" {
		reportType = "daily"
	}
	channel := input.Channel
	if channel == "" {
		channel = domain.ChannelEmail
	}
	timezone := strings.TrimSpace(input.Timezone)
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	schedule := strings.TrimSpace(input.Schedule)
	if schedule == "" {
		schedule = "0 9 * * *"
	}
	subscription := domain.Subscription{UserID: input.Subject.UserID, MonitorID: input.MonitorID, ReportType: reportType, Channel: channel, Recipient: strings.TrimSpace(input.Recipient), Timezone: timezone, Schedule: schedule, Enabled: input.Enabled}
	secret := ""
	if subscription.Channel == domain.ChannelRSS {
		var err error
		secret, err = service.token()
		if err != nil {
			return SubscriptionSecret{}, fmt.Errorf("generate rss token: %w", err)
		}
		subscription.TokenHash = domain.TokenHash(secret)
	}
	if err := subscription.ValidateCreate(); err != nil {
		return SubscriptionSecret{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	created := domain.Subscription{}
	err := service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		value, err := service.store.CreateSubscription(transactionCtx, subscription)
		if err != nil {
			return err
		}
		created = value
		return service.audit.Write(transactionCtx, subscriptionAudit(transactionCtx, input.Subject, operationsdomain.ActionSubscriptionCreated, created, nil))
	})
	if err != nil {
		return SubscriptionSecret{}, deliverySubscriptionError(err)
	}
	return SubscriptionSecret{Subscription: created, RSSToken: secret}, nil
}

func (service *SubscriptionService) List(ctx context.Context, subject identitydomain.Subject, query domain.SubscriptionListQuery) (domain.SubscriptionPage, error) {
	if err := requireSubscriptionUser(subject); err != nil {
		return domain.SubscriptionPage{}, err
	}
	if err := query.Validate(); err != nil {
		return domain.SubscriptionPage{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	page, err := service.store.ListSubscriptions(ctx, subject.UserID, query)
	return page, deliverySubscriptionError(err)
}

func (service *SubscriptionService) Get(ctx context.Context, subject identitydomain.Subject, subscriptionID int64) (domain.Subscription, error) {
	if err := requireSubscriptionUser(subject); err != nil {
		return domain.Subscription{}, err
	}
	if subscriptionID <= 0 {
		return domain.Subscription{}, invalidSubscriptionRequest()
	}
	subscription, err := service.store.GetSubscription(ctx, subscriptionID, subject.UserID)
	return subscription, deliverySubscriptionError(err)
}

func (service *SubscriptionService) Update(ctx context.Context, input UpdateSubscriptionInput) (domain.Subscription, error) {
	if err := requireSubscriptionUser(input.Subject); err != nil {
		return domain.Subscription{}, err
	}
	if input.SubscriptionID <= 0 || input.ExpectedVersion <= 0 || (input.Recipient == nil && input.Timezone == nil && input.Schedule == nil && input.Enabled == nil) {
		return domain.Subscription{}, invalidSubscriptionRequest()
	}
	current, err := service.store.GetSubscription(ctx, input.SubscriptionID, input.Subject.UserID)
	if err != nil {
		return domain.Subscription{}, deliverySubscriptionError(err)
	}
	next := current
	if input.Recipient != nil {
		if current.Channel != domain.ChannelEmail {
			return domain.Subscription{}, invalidSubscriptionRequest()
		}
		next.Recipient = strings.TrimSpace(*input.Recipient)
	}
	if input.Timezone != nil {
		next.Timezone = strings.TrimSpace(*input.Timezone)
	}
	if input.Schedule != nil {
		next.Schedule = strings.TrimSpace(*input.Schedule)
	}
	if input.Enabled != nil {
		next.Enabled = *input.Enabled
	}
	updated := domain.Subscription{}
	err = service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		value, err := service.store.UpdateSubscription(transactionCtx, next, input.ExpectedVersion)
		if err != nil {
			return err
		}
		updated = value
		return service.audit.Write(transactionCtx, subscriptionAudit(transactionCtx, input.Subject, operationsdomain.ActionSubscriptionUpdated, updated, &current))
	})
	if err != nil {
		return domain.Subscription{}, deliverySubscriptionError(err)
	}
	return updated, nil
}

func (service *SubscriptionService) RotateRSSToken(ctx context.Context, input RotateRSSTokenInput) (SubscriptionSecret, error) {
	if err := requireSubscriptionUser(input.Subject); err != nil {
		return SubscriptionSecret{}, err
	}
	if input.SubscriptionID <= 0 || input.ExpectedVersion <= 0 {
		return SubscriptionSecret{}, invalidSubscriptionRequest()
	}
	current, err := service.store.GetSubscription(ctx, input.SubscriptionID, input.Subject.UserID)
	if err != nil {
		return SubscriptionSecret{}, deliverySubscriptionError(err)
	}
	if current.Channel != domain.ChannelRSS {
		return SubscriptionSecret{}, invalidSubscriptionRequest()
	}
	secret, err := service.token()
	if err != nil {
		return SubscriptionSecret{}, fmt.Errorf("generate rss token: %w", err)
	}
	updated := domain.Subscription{}
	err = service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		value, err := service.store.RotateRSSToken(transactionCtx, input.SubscriptionID, input.Subject.UserID, input.ExpectedVersion, domain.TokenHash(secret))
		if err != nil {
			return err
		}
		updated = value
		return service.audit.Write(transactionCtx, subscriptionAudit(transactionCtx, input.Subject, operationsdomain.ActionSubscriptionTokenRotated, updated, &current))
	})
	if err != nil {
		return SubscriptionSecret{}, deliverySubscriptionError(err)
	}
	return SubscriptionSecret{Subscription: updated, RSSToken: secret}, nil
}

// Delete soft-deletes a disabled subscription. Delivery history remains
// immutable, while ordinary reads stop returning the deleted configuration.
func (service *SubscriptionService) Delete(ctx context.Context, input DeleteSubscriptionInput) (domain.Subscription, error) {
	if err := requireSubscriptionUser(input.Subject); err != nil {
		return domain.Subscription{}, err
	}
	if input.SubscriptionID <= 0 || input.ExpectedVersion <= 0 {
		return domain.Subscription{}, invalidSubscriptionRequest()
	}
	current, err := service.store.GetSubscription(ctx, input.SubscriptionID, input.Subject.UserID)
	if err != nil {
		return domain.Subscription{}, deliverySubscriptionError(err)
	}
	if current.Enabled {
		return domain.Subscription{}, sharederrors.New(sharederrors.CodeConflict, 409, "disable the subscription before deleting it")
	}
	deleted := domain.Subscription{}
	err = service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		value, err := service.store.DeleteSubscription(transactionCtx, input.SubscriptionID, input.Subject.UserID, input.ExpectedVersion)
		if err != nil {
			return err
		}
		deleted = value
		return service.audit.Write(transactionCtx, subscriptionAudit(transactionCtx, input.Subject, operationsdomain.ActionSubscriptionDeleted, deleted, &current))
	})
	if err != nil {
		return domain.Subscription{}, deliverySubscriptionError(err)
	}
	return deleted, nil
}

func requireSubscriptionUser(subject identitydomain.Subject) error {
	if subject.UserID <= 0 || subject.SessionID <= 0 || !subject.Role.Valid() {
		return sharederrors.New(sharederrors.CodeUnauthenticated, 401, "")
	}
	return nil
}

func invalidSubscriptionRequest() error {
	return sharederrors.New(sharederrors.CodeInvalidRequest, 400, "invalid subscription request")
}

func deliverySubscriptionError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	switch {
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.New(sharederrors.CodeNotFound, 404, "subscription not found")
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		return invalidSubscriptionRequest()
	case errors.Is(err, sharedrepository.ErrConflict):
		return sharederrors.New(sharederrors.CodeConflict, 409, "subscription version conflict")
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.New(sharederrors.CodeUnavailable, 503, "subscription service unavailable")
	default:
		return err
	}
}

func subscriptionAudit(ctx context.Context, subject identitydomain.Subject, action operationsdomain.AuditAction, next domain.Subscription, previous *domain.Subscription) operationsdomain.AuditEntry {
	after := map[string]any{"subscription_version": next.Version, "enabled": next.Enabled}
	var before map[string]any
	if previous != nil {
		before = map[string]any{"subscription_version": previous.Version, "enabled": previous.Enabled}
	}
	return operationsdomain.AuditEntry{ActorType: "user", ActorID: subject.UserID, Action: action, ResourceType: "report_subscription", ResourceID: next.ID, RequestID: requestcontext.RequestID(ctx), TraceID: requestcontext.TraceID(ctx), Before: before, After: after, Result: operationsdomain.AuditResultSuccess}
}

func newRSSToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
