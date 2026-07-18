package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"strings"

	deliveryapplication "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// Message materializes a published report snapshot for the email Job. It
// joins only delivery-owned facts and never returns token hashes or SMTP
// credentials.
func (repository *Repository) Message(ctx context.Context, deliveryID int64) (deliveryapplication.MailMessage, int, error) {
	if repository == nil || repository.runtime == nil || deliveryID <= 0 {
		return deliveryapplication.MailMessage{}, 0, sharedrepository.ErrInvalidInput
	}
	var recipient, title, summary, body string
	var attempts int
	err := repository.runtime.SQL.QueryRowContext(ctx, `
SELECT COALESCE(s.recipient, ''), r.title, COALESCE(r.summary, ''),
       COALESCE(string_agg(ri.title_snapshot || E'\\n' || COALESCE(ri.summary_snapshot, '') ||
           CASE WHEN COALESCE(source.original_url, '') <> '' THEN E'\\n原文链接: ' || source.original_url ELSE '' END,
           E'\\n\\n' ORDER BY ri.rank, ri.event_id), ''),
       (SELECT count(*) FROM delivery_attempts WHERE delivery_id = d.id)
FROM report_deliveries d
JOIN report_subscriptions s ON s.id = d.subscription_id
JOIN reports r ON r.id = d.report_id AND r.status = 'published'
LEFT JOIN report_items ri ON ri.report_id = r.id
LEFT JOIN LATERAL (
    SELECT c.canonical_url AS original_url
    FROM event_contents ec
    JOIN contents c ON c.id = ec.content_id AND c.content_status = 'active' AND c.deleted_at IS NULL
    WHERE ec.event_id = ri.event_id AND ec.evidence_role <> 'duplicate'
    ORDER BY ec.is_representative DESC, ec.membership_score DESC, ec.content_id ASC
    LIMIT 1
) source ON true
WHERE d.id = $1
GROUP BY d.id, s.recipient, r.title, r.summary`, deliveryID).Scan(&recipient, &title, &summary, &body, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return deliveryapplication.MailMessage{}, 0, sharedrepository.ErrNotFound
	}
	if err != nil {
		return deliveryapplication.MailMessage{}, 0, sharedrepository.MapError(err)
	}
	if strings.TrimSpace(recipient) == "" {
		return deliveryapplication.MailMessage{}, 0, fmt.Errorf("%w: email recipient is missing", sharedrepository.ErrConstraint)
	}
	if attempts < 1 {
		attempts = 1
	} else {
		attempts++
	}
	text := strings.TrimSpace(summary)
	if body != "" {
		if text != "" {
			text += "\n\n"
		}
		text += body
	}
	paragraphs := strings.ReplaceAll(html.EscapeString(text), "\n", "<br>\n")
	return deliveryapplication.MailMessage{To: recipient, Subject: title, Text: text, HTML: "<html><body>" + paragraphs + "</body></html>"}, attempts, nil
}

type Repository struct{ runtime *database.Runtime }

var _ interface {
	SaveSubscription(context.Context, domain.Subscription) error
	CreateSubscription(context.Context, domain.Subscription) (domain.Subscription, error)
	GetSubscription(context.Context, int64, int64) (domain.Subscription, error)
	ListSubscriptions(context.Context, int64, domain.SubscriptionListQuery) (domain.SubscriptionPage, error)
	UpdateSubscription(context.Context, domain.Subscription, int64) (domain.Subscription, error)
	RotateRSSToken(context.Context, int64, int64, int64, string) (domain.Subscription, error)
	DeleteSubscription(context.Context, int64, int64, int64) (domain.Subscription, error)
	CreateDelivery(context.Context, domain.Delivery) (bool, error)
	GetDelivery(context.Context, int64) (domain.Delivery, error)
	ClaimDelivery(context.Context, int64) (domain.Delivery, error)
	UpdateDelivery(context.Context, domain.Delivery) error
	AppendAttempt(context.Context, int64, int, string, int, string) error
	Message(context.Context, int64) (deliveryapplication.MailMessage, int, error)
} = (*Repository)(nil)

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

func (repository *Repository) GetEnabledSubscription(ctx context.Context, subscriptionID int64) (domain.Subscription, error) {
	if repository == nil || repository.runtime == nil || subscriptionID <= 0 {
		return domain.Subscription{}, sharedrepository.ErrUnavailable
	}
	return scanSubscription(repository.runtime.SQL.QueryRowContext(ctx, `
SELECT `+subscriptionColumns+` FROM report_subscriptions
WHERE id = $1 AND enabled = true AND deleted_at IS NULL`, subscriptionID))
}

func (repository *Repository) ListEnabledSubscriptions(ctx context.Context) ([]domain.Subscription, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT `+subscriptionColumns+` FROM report_subscriptions
WHERE enabled = true AND deleted_at IS NULL ORDER BY id ASC`)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	items := make([]domain.Subscription, 0)
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return items, nil
}

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

func (repository *Repository) ListSubscriptions(ctx context.Context, userID int64, query domain.SubscriptionListQuery) (domain.SubscriptionPage, error) {
	if repository == nil || repository.runtime == nil || userID <= 0 {
		return domain.SubscriptionPage{}, sharedrepository.ErrInvalidInput
	}
	if err := query.Validate(); err != nil {
		return domain.SubscriptionPage{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	cursor, err := pagination.Decode(query.Cursor, "id", true, subscriptionListFingerprint(userID))
	if err != nil {
		return domain.SubscriptionPage{}, fmt.Errorf("%w: subscription cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	rows, err := deliveryQueryerFor(ctx, repository.runtime).QueryContext(ctx, `SELECT `+subscriptionColumns+`
FROM report_subscriptions
WHERE user_id = $1 AND deleted_at IS NULL AND ($2 = 0 OR id < $2)
ORDER BY id DESC
LIMIT $3`, userID, cursor.ID, query.Limit+1)
	if err != nil {
		return domain.SubscriptionPage{}, sharedrepository.MapError(err)
	}
	defer rows.Close()
	page := domain.SubscriptionPage{Items: make([]domain.Subscription, 0, query.Limit)}
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return domain.SubscriptionPage{}, err
		}
		page.Items = append(page.Items, subscription)
	}
	if err := rows.Err(); err != nil {
		return domain.SubscriptionPage{}, sharedrepository.MapError(err)
	}
	if len(page.Items) > query.Limit {
		page.NextCursor, err = pagination.Encode("id", true, subscriptionListFingerprint(userID), page.Items[query.Limit-1].ID)
		if err != nil {
			return domain.SubscriptionPage{}, fmt.Errorf("encode subscription cursor: %w", err)
		}
		page.Items = page.Items[:query.Limit]
	}
	return page, nil
}

func subscriptionListFingerprint(userID int64) string {
	return fmt.Sprintf("user:%d", userID)
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

func (repository *Repository) DeleteSubscription(ctx context.Context, subscriptionID, userID, expectedVersion int64) (domain.Subscription, error) {
	if repository == nil || repository.runtime == nil || subscriptionID <= 0 || userID <= 0 || expectedVersion <= 0 {
		return domain.Subscription{}, sharedrepository.ErrInvalidInput
	}
	next, err := scanSubscription(deliveryQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
UPDATE report_subscriptions
SET deleted_at = now(), enabled = false, version = version + 1, updated_at = now()
WHERE id = $1 AND user_id = $2 AND version = $3 AND enabled = false AND deleted_at IS NULL
RETURNING `+subscriptionColumns, subscriptionID, userID, expectedVersion))
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

func (repository *Repository) GetDeliveryForScope(ctx context.Context, reportID, subscriptionID int64) (domain.Delivery, error) {
	if repository == nil || repository.runtime == nil || reportID <= 0 || subscriptionID <= 0 {
		return domain.Delivery{}, sharedrepository.ErrInvalidInput
	}
	return scanDelivery(repository.runtime.SQL.QueryRowContext(ctx, `
SELECT id, report_id, subscription_id, idempotency_key, status, next_attempt_at, succeeded_at
FROM report_deliveries WHERE report_id = $1 AND subscription_id = $2`, reportID, subscriptionID))
}

type deliveryScanner interface {
	Scan(...any) error
}

func scanDelivery(row deliveryScanner) (domain.Delivery, error) {
	var delivery domain.Delivery
	var next, succeeded sql.NullTime
	if err := row.Scan(&delivery.ID, &delivery.ReportID, &delivery.SubscriptionID, &delivery.IdempotencyKey, &delivery.Status, &next, &succeeded); err != nil {
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

func (repository *Repository) GetDelivery(ctx context.Context, deliveryID int64) (domain.Delivery, error) {
	if repository == nil || repository.runtime == nil || deliveryID <= 0 {
		return domain.Delivery{}, sharedrepository.ErrInvalidInput
	}
	return scanDelivery(repository.runtime.SQL.QueryRowContext(ctx, `SELECT id, report_id, subscription_id, idempotency_key, status, next_attempt_at, succeeded_at FROM report_deliveries WHERE id = $1`, deliveryID))
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
