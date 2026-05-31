package contentrepo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindByID(ctx context.Context, id string) (content.SourceItem, error) {
	const query = `
SELECT id, source_id, title, snippet, raw_url, canonical_url, published_at, content_hash, language, status, duplicate_of_item_id, created_at, updated_at
FROM source_items
WHERE id = $1`
	item, err := scanItem(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		return content.SourceItem{}, normalizeDBError(err)
	}
	return item, nil
}

func (r *Repository) FindByCanonicalURL(ctx context.Context, canonicalURL string) (content.SourceItem, error) {
	const query = `
SELECT id, source_id, title, snippet, raw_url, canonical_url, published_at, content_hash, language, status, duplicate_of_item_id, created_at, updated_at
FROM source_items
WHERE canonical_url = $1`
	item, err := scanItem(r.db.QueryRowContext(ctx, query, canonicalURL))
	if err != nil {
		return content.SourceItem{}, normalizeDBError(err)
	}
	return item, nil
}

func (r *Repository) FindByContentHash(ctx context.Context, contentHash string) (content.SourceItem, error) {
	const query = `
SELECT id, source_id, title, snippet, raw_url, canonical_url, published_at, content_hash, language, status, duplicate_of_item_id, created_at, updated_at
FROM source_items
WHERE content_hash = $1 AND status = 'primary'
ORDER BY created_at ASC, id ASC
LIMIT 1`
	item, err := scanItem(r.db.QueryRowContext(ctx, query, contentHash))
	if err != nil {
		return content.SourceItem{}, normalizeDBError(err)
	}
	return item, nil
}

func (r *Repository) Create(ctx context.Context, item content.SourceItem) (content.SourceItem, error) {
	const query = `
INSERT INTO source_items (
	id, source_id, title, snippet, raw_url, canonical_url, published_at, content_hash, language, status, duplicate_of_item_id, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), $12, $13
)
RETURNING id, source_id, title, snippet, raw_url, canonical_url, published_at, content_hash, language, status, duplicate_of_item_id, created_at, updated_at`
	created, err := scanItem(r.db.QueryRowContext(ctx, query,
		item.ID,
		item.SourceID,
		item.Title,
		item.Snippet,
		item.RawURL,
		item.CanonicalURL,
		item.PublishedAt,
		item.ContentHash,
		item.Language,
		item.Status,
		item.DuplicateOfItemID,
		item.CreatedAt,
		item.UpdatedAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return content.SourceItem{}, content.ErrAlreadyExists
		}
		return content.SourceItem{}, normalizeDBError(err)
	}
	return created, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(row scanner) (content.SourceItem, error) {
	var item content.SourceItem
	var duplicateOf sql.NullString
	err := row.Scan(
		&item.ID,
		&item.SourceID,
		&item.Title,
		&item.Snippet,
		&item.RawURL,
		&item.CanonicalURL,
		&item.PublishedAt,
		&item.ContentHash,
		&item.Language,
		&item.Status,
		&duplicateOf,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if duplicateOf.Valid {
		item.DuplicateOfItemID = duplicateOf.String
	}
	return item, err
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}

func normalizeDBError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return content.ErrNotFound
	}
	return err
}
