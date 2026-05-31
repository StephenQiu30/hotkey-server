package channelrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListChannels(ctx context.Context, activeOnly bool) ([]servicechannel.Channel, error) {
	query := `
SELECT id, name, slug, description, status, created_at, updated_at
FROM channels`
	args := []any{}
	if activeOnly {
		query += ` WHERE status = $1`
		args = append(args, servicechannel.ChannelStatusActive)
	}
	query += ` ORDER BY created_at ASC, id ASC`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var channels []servicechannel.Channel
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (r *Repository) ChannelByID(ctx context.Context, channelID string) (servicechannel.Channel, error) {
	const query = `
SELECT id, name, slug, description, status, created_at, updated_at
FROM channels
WHERE id = $1`
	return scanChannel(r.db.QueryRowContext(ctx, query, channelID))
}

func (r *Repository) CreateChannel(ctx context.Context, channel servicechannel.Channel) (servicechannel.Channel, error) {
	const query = `
INSERT INTO channels (id, name, slug, description, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, name, slug, description, status, created_at, updated_at`
	created, err := scanChannel(r.db.QueryRowContext(ctx, query,
		channel.ID, channel.Name, channel.Slug, channel.Description, channel.Status, channel.CreatedAt, channel.UpdatedAt,
	))
	if err != nil && isUniqueViolation(err) {
		return servicechannel.Channel{}, servicechannel.ErrAlreadyExists
	}
	return created, err
}

func (r *Repository) UpdateChannel(ctx context.Context, channel servicechannel.Channel) (servicechannel.Channel, error) {
	const query = `
UPDATE channels
SET name = $2, slug = $3, description = $4, status = $5, updated_at = $6
WHERE id = $1
RETURNING id, name, slug, description, status, created_at, updated_at`
	updated, err := scanChannel(r.db.QueryRowContext(ctx, query,
		channel.ID, channel.Name, channel.Slug, channel.Description, channel.Status, channel.UpdatedAt,
	))
	if err != nil && isUniqueViolation(err) {
		return servicechannel.Channel{}, servicechannel.ErrAlreadyExists
	}
	return updated, err
}

func (r *Repository) DeleteChannel(ctx context.Context, channelID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM channels WHERE id = $1`, channelID)
	return requireRows(result, err)
}

func (r *Repository) UpsertSubscription(ctx context.Context, userID string, channelID string, createdAt time.Time) (servicechannel.Subscription, error) {
	const query = `
INSERT INTO user_channel_subscriptions (user_id, channel_id, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, channel_id) DO UPDATE SET channel_id = EXCLUDED.channel_id
RETURNING user_id, channel_id, created_at`
	var subscription servicechannel.Subscription
	if err := r.db.QueryRowContext(ctx, query, userID, channelID, createdAt).Scan(
		&subscription.UserID,
		&subscription.Channel.ID,
		&subscription.CreatedAt,
	); err != nil {
		return servicechannel.Subscription{}, err
	}
	channel, err := r.ChannelByID(ctx, channelID)
	if err != nil {
		return servicechannel.Subscription{}, err
	}
	subscription.Channel = channel
	return subscription, nil
}

func (r *Repository) DeleteSubscription(ctx context.Context, userID string, channelID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM user_channel_subscriptions WHERE user_id = $1 AND channel_id = $2`, userID, channelID)
	return requireRows(result, err)
}

func (r *Repository) ListSubscriptions(ctx context.Context, userID string) ([]servicechannel.Subscription, error) {
	const query = `
SELECT s.user_id, s.created_at, c.id, c.name, c.slug, c.description, c.status, c.created_at, c.updated_at
FROM user_channel_subscriptions s
JOIN channels c ON c.id = s.channel_id
WHERE s.user_id = $1
ORDER BY s.created_at ASC, c.id ASC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var subscriptions []servicechannel.Subscription
	for rows.Next() {
		var subscription servicechannel.Subscription
		if err := rows.Scan(
			&subscription.UserID,
			&subscription.CreatedAt,
			&subscription.Channel.ID,
			&subscription.Channel.Name,
			&subscription.Channel.Slug,
			&subscription.Channel.Description,
			&subscription.Channel.Status,
			&subscription.Channel.CreatedAt,
			&subscription.Channel.UpdatedAt,
		); err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions, rows.Err()
}

func (r *Repository) CreateKeyword(ctx context.Context, keyword servicechannel.Keyword) (servicechannel.Keyword, error) {
	const query = `
INSERT INTO user_keywords (id, user_id, keyword, enabled, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, keyword, enabled, created_at, updated_at`
	return scanKeyword(r.db.QueryRowContext(ctx, query,
		keyword.ID, keyword.UserID, keyword.Keyword, keyword.Enabled, keyword.CreatedAt, keyword.UpdatedAt,
	))
}

func (r *Repository) UpdateKeyword(ctx context.Context, keyword servicechannel.Keyword) (servicechannel.Keyword, error) {
	const query = `
UPDATE user_keywords
SET keyword = $3, enabled = $4, updated_at = $5
WHERE user_id = $1 AND id = $2
RETURNING id, user_id, keyword, enabled, created_at, updated_at`
	return scanKeyword(r.db.QueryRowContext(ctx, query, keyword.UserID, keyword.ID, keyword.Keyword, keyword.Enabled, keyword.UpdatedAt))
}

func (r *Repository) KeywordByID(ctx context.Context, userID string, keywordID string) (servicechannel.Keyword, error) {
	const query = `
SELECT id, user_id, keyword, enabled, created_at, updated_at
FROM user_keywords
WHERE user_id = $1 AND id = $2`
	return scanKeyword(r.db.QueryRowContext(ctx, query, userID, keywordID))
}

func (r *Repository) DeleteKeyword(ctx context.Context, userID string, keywordID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM user_keywords WHERE user_id = $1 AND id = $2`, userID, keywordID)
	return requireRows(result, err)
}

func (r *Repository) ListKeywords(ctx context.Context, userID string) ([]servicechannel.Keyword, error) {
	const query = `
SELECT id, user_id, keyword, enabled, created_at, updated_at
FROM user_keywords
WHERE user_id = $1
ORDER BY created_at ASC, id ASC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var keywords []servicechannel.Keyword
	for rows.Next() {
		keyword, err := scanKeyword(rows)
		if err != nil {
			return nil, err
		}
		keywords = append(keywords, keyword)
	}
	return keywords, rows.Err()
}

func (r *Repository) Setting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key = $1`, key).Scan(&value)
	return value, err
}

func (r *Repository) UpsertSetting(ctx context.Context, key string, value string, updatedAt time.Time) error {
	const query = `
INSERT INTO system_settings (key, value, updated_at)
VALUES ($1, $2, $3)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`
	_, err := r.db.ExecContext(ctx, query, key, value, updatedAt)
	return err
}

func (r *Repository) UserDailySendAt(ctx context.Context, userID string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `SELECT daily_send_at FROM users WHERE id = $1`, userID).Scan(&value)
	return value, err
}

func (r *Repository) SetUserDailySendAt(ctx context.Context, userID string, dailySendAt string, updatedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE users SET daily_send_at = $2, updated_at = $3 WHERE id = $1`, userID, dailySendAt, updatedAt)
	return requireRows(result, err)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanChannel(row scanner) (servicechannel.Channel, error) {
	var channel servicechannel.Channel
	err := row.Scan(&channel.ID, &channel.Name, &channel.Slug, &channel.Description, &channel.Status, &channel.CreatedAt, &channel.UpdatedAt)
	return channel, err
}

func scanKeyword(row scanner) (servicechannel.Keyword, error) {
	var keyword servicechannel.Keyword
	err := row.Scan(&keyword.ID, &keyword.UserID, &keyword.Keyword, &keyword.Enabled, &keyword.CreatedAt, &keyword.UpdatedAt)
	return keyword, err
}

func requireRows(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}
