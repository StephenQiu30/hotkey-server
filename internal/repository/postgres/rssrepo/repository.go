package rssrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	servicerss "github.com/StephenQiu30/hotkey-server/internal/service/rss"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindByUserID(ctx context.Context, userID string) (servicerss.Feed, error) {
	const query = `
		select user_id, token_hash, enabled, last_accessed_at, created_at, updated_at
		from rss_feeds
		where user_id = $1
	`
	return scanFeed(r.db.QueryRowContext(ctx, query, userID))
}

func (r *Repository) FindByTokenHash(ctx context.Context, tokenHash string) (servicerss.Feed, error) {
	const query = `
		select user_id, token_hash, enabled, last_accessed_at, created_at, updated_at
		from rss_feeds
		where token_hash = $1
	`
	return scanFeed(r.db.QueryRowContext(ctx, query, tokenHash))
}

func (r *Repository) Save(ctx context.Context, feed servicerss.Feed) (servicerss.Feed, error) {
	const query = `
		insert into rss_feeds (user_id, token_hash, enabled, last_accessed_at, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (user_id) do update set
			token_hash = excluded.token_hash,
			enabled = excluded.enabled,
			last_accessed_at = excluded.last_accessed_at,
			updated_at = excluded.updated_at
		returning user_id, token_hash, enabled, last_accessed_at, created_at, updated_at
	`
	return scanFeed(r.db.QueryRowContext(ctx, query, feed.UserID, feed.TokenHash, feed.Enabled, feed.LastAccessedAt, feed.CreatedAt, feed.UpdatedAt))
}

func (r *Repository) Disable(ctx context.Context, userID string, now time.Time) error {
	const query = `
		update rss_feeds
		set enabled = false, updated_at = $2
		where user_id = $1
	`
	result, err := r.db.ExecContext(ctx, query, userID, now)
	if err != nil {
		return err
	}
	return rowsAffectedOrNotFound(result)
}

func (r *Repository) Touch(ctx context.Context, userID string, now time.Time) error {
	const query = `
		update rss_feeds
		set last_accessed_at = $2, updated_at = $2
		where user_id = $1
	`
	result, err := r.db.ExecContext(ctx, query, userID, now)
	if err != nil {
		return err
	}
	return rowsAffectedOrNotFound(result)
}

type rowScanner interface {
	Scan(...any) error
}

func scanFeed(row rowScanner) (servicerss.Feed, error) {
	var feed servicerss.Feed
	var lastAccessed sql.NullTime
	if err := row.Scan(&feed.UserID, &feed.TokenHash, &feed.Enabled, &lastAccessed, &feed.CreatedAt, &feed.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return servicerss.Feed{}, servicerss.ErrFeedNotFound
		}
		return servicerss.Feed{}, err
	}
	if lastAccessed.Valid {
		feed.LastAccessedAt = &lastAccessed.Time
	}
	return feed, nil
}

func rowsAffectedOrNotFound(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return servicerss.ErrFeedNotFound
	}
	return nil
}
