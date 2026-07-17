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
	CreateSubscription(context.Context, domain.Subscription) (domain.Subscription, error)
	GetSubscription(context.Context, int64, int64) (domain.Subscription, error)
	ListSubscriptions(context.Context, int64) ([]domain.Subscription, error)
	UpdateSubscription(context.Context, domain.Subscription, int64) (domain.Subscription, error)
	RotateRSSToken(context.Context, int64, int64, int64, string) (domain.Subscription, error)
	CreateDelivery(context.Context, domain.Delivery) (bool, error)
	GetDelivery(context.Context, int64) (domain.Delivery, error)
	ClaimDelivery(context.Context, int64) (domain.Delivery, error)
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
	_, err := deliveryQueryerFor(ctx, repository.runtime).ExecContext(ctx, `
INSERT INTO report_subscriptions (id, version, user_id, monitor_id, report_type, channel, recipient, rss_token_hash, timezone, schedule, enabled)
VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9, $10, $11)
ON CONFLICT (id) DO UPDATE SET version = EXCLUDED.version, report_type = EXCLUDED.report_type,
monitor_id = EXCLUDED.monitor_id, channel = EXCLUDED.channel, recipient = EXCLUDED.recipient, rss_token_hash = EXCLUDED.rss_token_hash,
timezone = EXCLUDED.timezone, schedule = EXCLUDED.schedule, enabled = EXCLUDED.enabled, updated_at = now()`,
		subscription.ID, subscription.Version, subscription.UserID, subscription.MonitorID, subscription.ReportType, subscription.Channel,
		subscription.Recipient, subscription.TokenHash, subscription.Timezone, subscription.Schedule, subscription.Enabled)
	return sharedrepository.MapError(err)
}

func (repository *Repository) CreateSubscription(ctx context.Context, subscription domain.Subscription) (domain.Subscription, error) {
	if repository == nil || repository.runtime == nil {
		return domain.Subscription{}, sharedrepository.ErrUnavailable
	}
	if err := subscription.ValidateCreate(); err != nil {
		return domain.Subscription{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return scanSubscription(deliveryQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
INSERT INTO report_subscriptions (user_id, monitor_id, report_type, channel, recipient, rss_token_hash, timezone, schedule, enabled)
VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7, $8, $9)
RETURNING `+subscriptionColumns,
		subscription.UserID, subscription.MonitorID, subscription.ReportType, subscription.Channel, subscription.Recipient,
		subscription.TokenHash, subscription.Timezone, subscription.Schedule, subscription.Enabled))
}

func (repository *Repository) GetSubscription(ctx context.Context, subscriptionID, userID int64) (domain.Subscription, error) {
	if repository == nil || repository.runtime == nil || subscriptionID <= 0 || userID <= 0 {
		return domain.Subscription{}, sharedrepository.ErrInvalidInput
	}
	return scanSubscription(deliveryQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `SELECT `+subscriptionColumns+` FROM report_subscriptions WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, subscriptionID, userID))
}

func (repository *Repository) ListSubscriptions(ctx context.Context, userID int64) ([]domain.Subscription, error) {
	if repository == nil || repository.runtime == nil || userID <= 0 {
		return nil, sharedrepository.ErrInvalidInput
	}
	rows, err := deliveryQueryerFor(ctx, repository.runtime).QueryContext(ctx, `SELECT `+subscriptionColumns+` FROM report_subscriptions WHERE user_id = $1 AND deleted_at IS NULL ORDER BY id DESC`, userID)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	items := make([]domain.Subscription, 0)
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, subscription)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return items, nil
}

func (repository *Repository) UpdateSubscription(ctx context.Context, subscription domain.Subscription, expectedVersion int64) (domain.Subscription, error) {
	if repository == nil || repository.runtime == nil || expectedVersion <= 0 {
		return domain.Subscription{}, sharedrepository.ErrInvalidInput
	}
	if err := subscription.Validate(); err != nil {
		return domain.Subscription{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	next, err := scanSubscription(deliveryQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
UPDATE report_subscriptions
SET recipient = NULLIF($1, ''), timezone = $2, schedule = $3, enabled = $4, version = version + 1, updated_at = now()
WHERE id = $5 AND user_id = $6 AND version = $7 AND deleted_at IS NULL
RETURNING `+subscriptionColumns, subscription.Recipient, subscription.Timezone, subscription.Schedule, subscription.Enabled, subscription.ID, subscription.UserID, expectedVersion))
	if errors.Is(err, sharedrepository.ErrNotFound) {
		return domain.Subscription{}, sharedrepository.ErrConflict
	}
	return next, err
}

func (repository *Repository) RotateRSSToken(ctx context.Context, subscriptionID, userID, expectedVersion int64, tokenHash string) (domain.Subscription, error) {
	if repository == nil || repository.runtime == nil || subscriptionID <= 0 || userID <= 0 || expectedVersion <= 0 {
		return domain.Subscription{}, sharedrepository.ErrInvalidInput
	}
	candidate := domain.Subscription{ID: subscriptionID, Version: expectedVersion, UserID: userID, ReportType: "daily", Channel: domain.ChannelRSS, TokenHash: tokenHash, Timezone: "UTC", Schedule: "0 0 * * *"}
	if err := candidate.Validate(); err != nil {
		return domain.Subscription{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	next, err := scanSubscription(deliveryQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
UPDATE report_subscriptions
SET rss_token_hash = $1, version = version + 1, updated_at = now()
WHERE id = $2 AND user_id = $3 AND version = $4 AND channel = 'rss' AND deleted_at IS NULL
RETURNING `+subscriptionColumns, tokenHash, subscriptionID, userID, expectedVersion))
	if errors.Is(err, sharedrepository.ErrNotFound) {
		return domain.Subscription{}, sharedrepository.ErrConflict
	}
	return next, err
}

func (repository *Repository) CreateDelivery(ctx context.Context, delivery domain.Delivery) (bool, error) {
	if repository == nil || repository.runtime == nil {
		return false, sharedrepository.ErrUnavailable
	}
	validation := delivery
	if validation.ID <= 0 {
		validation.ID = 1
	}
	if err := validation.Validate(); err != nil {
		return false, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	var id int64
	insertID := delivery.ID
	if insertID <= 0 {
		insertID = 0
	}
	err := repository.runtime.SQL.QueryRowContext(ctx, `
INSERT INTO report_deliveries (id, report_id, subscription_id, idempotency_key, status, next_attempt_at, succeeded_at)
VALUES (NULLIF($1, 0), $2, $3, $4, $5, $6, $7)
ON CONFLICT (report_id, subscription_id) DO NOTHING RETURNING id`,
		insertID, delivery.ReportID, delivery.SubscriptionID, delivery.IdempotencyKey, delivery.Status,
		delivery.NextAttemptAt, delivery.SucceededAt).Scan(&id)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, sharedrepository.MapError(err)
	}
	return false, nil
}

func (repository *Repository) GetDelivery(ctx context.Context, deliveryID int64) (domain.Delivery, error) {
	if repository == nil || repository.runtime == nil || deliveryID <= 0 {
		return domain.Delivery{}, sharedrepository.ErrInvalidInput
	}
	var delivery domain.Delivery
	var next, succeeded sql.NullTime
	err := repository.runtime.SQL.QueryRowContext(ctx, `SELECT id, report_id, subscription_id, idempotency_key, status, next_attempt_at, succeeded_at FROM report_deliveries WHERE id = $1`, deliveryID).Scan(&delivery.ID, &delivery.ReportID, &delivery.SubscriptionID, &delivery.IdempotencyKey, &delivery.Status, &next, &succeeded)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Delivery{}, sharedrepository.ErrNotFound
		}
		return domain.Delivery{}, sharedrepository.MapError(err)
	}
	if next.Valid {
		value := next.Time.UTC()
		delivery.NextAttemptAt = &value
	}
	if succeeded.Valid {
		value := succeeded.Time.UTC()
		delivery.SucceededAt = &value
	}
	return delivery, nil
}

func (repository *Repository) ClaimDelivery(ctx context.Context, deliveryID int64) (domain.Delivery, error) {
	if repository == nil || repository.runtime == nil || deliveryID <= 0 {
		return domain.Delivery{}, sharedrepository.ErrInvalidInput
	}
	var delivery domain.Delivery
	var next, succeeded sql.NullTime
	err := repository.runtime.SQL.QueryRowContext(ctx, `
UPDATE report_deliveries SET status = 'claimed', updated_at = now()
WHERE id = $1 AND status IN ('queued','retrying') AND (next_attempt_at IS NULL OR next_attempt_at <= now())
RETURNING id, report_id, subscription_id, idempotency_key, status, next_attempt_at, succeeded_at`, deliveryID).Scan(&delivery.ID, &delivery.ReportID, &delivery.SubscriptionID, &delivery.IdempotencyKey, &delivery.Status, &next, &succeeded)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Delivery{}, sharedrepository.ErrConflict
		}
		return domain.Delivery{}, sharedrepository.MapError(err)
	}
	if next.Valid {
		value := next.Time.UTC()
		delivery.NextAttemptAt = &value
	}
	if succeeded.Valid {
		value := succeeded.Time.UTC()
		delivery.SucceededAt = &value
	}
	return delivery, nil
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

const subscriptionColumns = `id, version, user_id, monitor_id, report_type, channel, COALESCE(recipient, ''), COALESCE(rss_token_hash, ''), timezone, schedule, enabled`

type deliveryRow interface {
	Scan(...any) error
}

type deliveryQueryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func deliveryQueryerFor(ctx context.Context, runtime *database.Runtime) deliveryQueryer {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return transaction.SQL
	}
	return runtime.SQL
}

func scanSubscription(row deliveryRow) (domain.Subscription, error) {
	var subscription domain.Subscription
	var monitorID sql.NullInt64
	var channel string
	if err := row.Scan(&subscription.ID, &subscription.Version, &subscription.UserID, &monitorID, &subscription.ReportType, &channel, &subscription.Recipient, &subscription.TokenHash, &subscription.Timezone, &subscription.Schedule, &subscription.Enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Subscription{}, sharedrepository.ErrNotFound
		}
		return domain.Subscription{}, sharedrepository.MapError(err)
	}
	subscription.Channel = domain.Channel(channel)
	if monitorID.Valid {
		value := monitorID.Int64
		subscription.MonitorID = &value
	}
	return subscription, nil
}
