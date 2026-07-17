package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type Repository struct{ runtime *database.Runtime }

var _ interface {
	SaveSubscription(context.Context, domain.Subscription) error
	CreateDelivery(context.Context, domain.Delivery) (bool, error)
	UpdateDelivery(context.Context, domain.Delivery) error
	AppendAttempt(context.Context, int64, int, string, int, string) error
} = (*Repository)(nil)

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

func (repository *Repository) SaveSubscription(ctx context.Context, subscription domain.Subscription) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if err := subscription.Validate(); err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	_, err := repository.runtime.SQL.ExecContext(ctx, `
INSERT INTO report_subscriptions (id, version, user_id, report_type, channel, recipient, rss_token_hash, timezone, schedule, enabled)
VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), $8, $9, $10)
ON CONFLICT (id) DO UPDATE SET version = EXCLUDED.version, report_type = EXCLUDED.report_type,
channel = EXCLUDED.channel, recipient = EXCLUDED.recipient, rss_token_hash = EXCLUDED.rss_token_hash,
timezone = EXCLUDED.timezone, schedule = EXCLUDED.schedule, enabled = EXCLUDED.enabled, updated_at = now()`,
		subscription.ID, subscription.Version, subscription.UserID, subscription.ReportType, subscription.Channel,
		subscription.Recipient, subscription.TokenHash, subscription.Timezone, subscription.Schedule, subscription.Enabled)
	return sharedrepository.MapError(err)
}

func (repository *Repository) CreateDelivery(ctx context.Context, delivery domain.Delivery) (bool, error) {
	if repository == nil || repository.runtime == nil {
		return false, sharedrepository.ErrUnavailable
	}
	if err := delivery.Validate(); err != nil {
		return false, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	var id int64
	err := repository.runtime.SQL.QueryRowContext(ctx, `
INSERT INTO report_deliveries (id, report_id, subscription_id, idempotency_key, status, next_attempt_at, succeeded_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (report_id, subscription_id) DO NOTHING RETURNING id`,
		delivery.ID, delivery.ReportID, delivery.SubscriptionID, delivery.IdempotencyKey, delivery.Status,
		delivery.NextAttemptAt, delivery.SucceededAt).Scan(&id)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, sharedrepository.MapError(err)
	}
	return false, nil
}

func (repository *Repository) UpdateDelivery(ctx context.Context, delivery domain.Delivery) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if err := delivery.Validate(); err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	result, err := repository.runtime.SQL.ExecContext(ctx, `
UPDATE report_deliveries SET status = $1, next_attempt_at = $2, succeeded_at = $3, updated_at = now()
WHERE id = $4 AND report_id = $5 AND subscription_id = $6`, delivery.Status, delivery.NextAttemptAt, delivery.SucceededAt,
		delivery.ID, delivery.ReportID, delivery.SubscriptionID)
	if err != nil {
		return sharedrepository.MapError(err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return sharedrepository.ErrNotFound
	}
	return nil
}

func (repository *Repository) AppendAttempt(ctx context.Context, deliveryID int64, attemptNo int, status string, responseCode int, message string) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if deliveryID <= 0 || attemptNo <= 0 || message == "" && status == "" {
		return fmt.Errorf("%w: invalid delivery attempt", sharedrepository.ErrInvalidInput)
	}
	_, err := repository.runtime.SQL.ExecContext(ctx, `
INSERT INTO delivery_attempts (delivery_id, attempt_no, started_at, finished_at, status, response_code, error)
VALUES ($1, $2, now(), now(), $3, NULLIF($4, 0), NULLIF($5, ''))`, deliveryID, attemptNo, status, responseCode, message)
	return sharedrepository.MapError(err)
}
