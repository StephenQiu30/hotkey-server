package sourcerepo

import (
	"context"
	"database/sql"
	"errors"

	servicesource "github.com/StephenQiu30/hotkey-server/internal/service/source"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListSources(ctx context.Context) ([]servicesource.Source, error) {
	const query = `
SELECT id, name, type, url, status, compliance_note, fetch_interval_min, rate_limit_per_hour, last_error, last_collected_at, created_at, updated_at
FROM sources
ORDER BY created_at ASC, id ASC`
	return r.listSources(ctx, query)
}

func (r *Repository) ListCollectableSources(ctx context.Context) ([]servicesource.Source, error) {
	const query = `
SELECT id, name, type, url, status, compliance_note, fetch_interval_min, rate_limit_per_hour, last_error, last_collected_at, created_at, updated_at
FROM sources
WHERE status = 'enabled'
ORDER BY created_at ASC, id ASC`
	return r.listSources(ctx, query)
}

func (r *Repository) SourceByID(ctx context.Context, sourceID string) (servicesource.Source, error) {
	const query = `
SELECT id, name, type, url, status, compliance_note, fetch_interval_min, rate_limit_per_hour, last_error, last_collected_at, created_at, updated_at
FROM sources
WHERE id = $1`
	source, err := scanSource(r.db.QueryRowContext(ctx, query, sourceID))
	if err != nil {
		return servicesource.Source{}, normalizeDBError(err)
	}
	source.ChannelIDs, err = r.channelIDs(ctx, source.ID)
	return source, err
}

func (r *Repository) CreateSource(ctx context.Context, source servicesource.Source) (servicesource.Source, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return servicesource.Source{}, err
	}
	defer func() { _ = tx.Rollback() }()
	const query = `
INSERT INTO sources (id, name, type, url, status, compliance_note, fetch_interval_min, rate_limit_per_hour, last_error, last_collected_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, name, type, url, status, compliance_note, fetch_interval_min, rate_limit_per_hour, last_error, last_collected_at, created_at, updated_at`
	created, err := scanSource(tx.QueryRowContext(ctx, query,
		source.ID, source.Name, source.Type, source.URL, source.Status, source.ComplianceNote, source.FetchIntervalMin, source.RateLimitPerHour,
		source.LastError, source.LastCollectedAt, source.CreatedAt, source.UpdatedAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return servicesource.Source{}, servicesource.ErrAlreadyExists
		}
		return servicesource.Source{}, normalizeDBError(err)
	}
	if err := replaceChannelLinks(ctx, tx, created.ID, source.ChannelIDs); err != nil {
		return servicesource.Source{}, err
	}
	if err := tx.Commit(); err != nil {
		return servicesource.Source{}, err
	}
	created.ChannelIDs = append([]string(nil), source.ChannelIDs...)
	return created, nil
}

func (r *Repository) UpdateSource(ctx context.Context, source servicesource.Source) (servicesource.Source, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return servicesource.Source{}, err
	}
	defer func() { _ = tx.Rollback() }()
	const query = `
UPDATE sources
SET name = $2, type = $3, url = $4, status = $5, compliance_note = $6, fetch_interval_min = $7, rate_limit_per_hour = $8, last_error = $9, last_collected_at = $10, updated_at = $11
WHERE id = $1
RETURNING id, name, type, url, status, compliance_note, fetch_interval_min, rate_limit_per_hour, last_error, last_collected_at, created_at, updated_at`
	updated, err := scanSource(tx.QueryRowContext(ctx, query,
		source.ID, source.Name, source.Type, source.URL, source.Status, source.ComplianceNote, source.FetchIntervalMin, source.RateLimitPerHour,
		source.LastError, source.LastCollectedAt, source.UpdatedAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return servicesource.Source{}, servicesource.ErrAlreadyExists
		}
		return servicesource.Source{}, normalizeDBError(err)
	}
	if err := replaceChannelLinks(ctx, tx, updated.ID, source.ChannelIDs); err != nil {
		return servicesource.Source{}, err
	}
	if err := tx.Commit(); err != nil {
		return servicesource.Source{}, err
	}
	updated.ChannelIDs = append([]string(nil), source.ChannelIDs...)
	return updated, nil
}

func (r *Repository) CreateCollectionRun(ctx context.Context, run servicesource.CollectionRun) (servicesource.CollectionRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return servicesource.CollectionRun{}, normalizeDBError(err)
	}
	defer func() { _ = tx.Rollback() }()
	const query = `
INSERT INTO collection_runs (id, source_id, status, items_fetched, error, started_at, finished_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, source_id, status, items_fetched, error, started_at, finished_at, created_at`
	created, err := scanCollectionRun(tx.QueryRowContext(ctx, query,
		run.ID, run.SourceID, run.Status, run.ItemsFetched, run.Error, run.StartedAt, run.FinishedAt, run.CreatedAt,
	))
	if err != nil {
		return servicesource.CollectionRun{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sources SET last_error = $2, last_collected_at = $3, updated_at = $3 WHERE id = $1`, run.SourceID, run.Error, run.FinishedAt); err != nil {
		return servicesource.CollectionRun{}, err
	}
	return created, tx.Commit()
}

func (r *Repository) ListCollectionRuns(ctx context.Context, sourceID string) ([]servicesource.CollectionRun, error) {
	const query = `
SELECT id, source_id, status, items_fetched, error, started_at, finished_at, created_at
FROM collection_runs
WHERE source_id = $1
ORDER BY started_at ASC, id ASC`
	rows, err := r.db.QueryContext(ctx, query, sourceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var runs []servicesource.CollectionRun
	for rows.Next() {
		run, err := scanCollectionRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *Repository) listSources(ctx context.Context, query string) ([]servicesource.Source, error) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var sources []servicesource.Source
	for rows.Next() {
		source, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		source.ChannelIDs, err = r.channelIDs(ctx, source.ID)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (r *Repository) channelIDs(ctx context.Context, sourceID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT channel_id FROM source_channel_links WHERE source_id = $1 ORDER BY channel_id ASC`, sourceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func replaceChannelLinks(ctx context.Context, tx execer, sourceID string, channelIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM source_channel_links WHERE source_id = $1`, sourceID); err != nil {
		return err
	}
	for _, channelID := range channelIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO source_channel_links (source_id, channel_id) VALUES ($1, $2)`, sourceID, channelID); err != nil {
			return err
		}
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSource(row scanner) (servicesource.Source, error) {
	var source servicesource.Source
	err := row.Scan(
		&source.ID,
		&source.Name,
		&source.Type,
		&source.URL,
		&source.Status,
		&source.ComplianceNote,
		&source.FetchIntervalMin,
		&source.RateLimitPerHour,
		&source.LastError,
		&source.LastCollectedAt,
		&source.CreatedAt,
		&source.UpdatedAt,
	)
	return source, err
}

func scanCollectionRun(row scanner) (servicesource.CollectionRun, error) {
	var run servicesource.CollectionRun
	err := row.Scan(&run.ID, &run.SourceID, &run.Status, &run.ItemsFetched, &run.Error, &run.StartedAt, &run.FinishedAt, &run.CreatedAt)
	return run, err
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}

func normalizeDBError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return servicesource.ErrNotFound
	}
	return err
}
