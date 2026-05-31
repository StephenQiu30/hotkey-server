package hotspotrepo

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) SaveEmbedding(ctx context.Context, embedding hotspot.Embedding) (hotspot.Embedding, error) {
	const query = `
INSERT INTO item_embeddings (
	item_id, model, embedding, text_hash, status, last_error, created_at, updated_at
) VALUES (
	$1, $2, $3::vector, $4, $5, NULLIF($6, ''), $7, $8
)
ON CONFLICT (item_id) DO UPDATE SET
	model = EXCLUDED.model,
	embedding = EXCLUDED.embedding,
	text_hash = EXCLUDED.text_hash,
	status = EXCLUDED.status,
	last_error = EXCLUDED.last_error,
	updated_at = EXCLUDED.updated_at
RETURNING item_id, model, embedding::text, text_hash, status, COALESCE(last_error, ''), created_at, updated_at`
	var vectorText string
	var status string
	err := r.db.QueryRowContext(ctx, query,
		embedding.ItemID,
		embedding.Model,
		vectorLiteral(embedding.Vector),
		embedding.TextHash,
		string(embedding.Status),
		embedding.LastError,
		embedding.CreatedAt,
		embedding.UpdatedAt,
	).Scan(&embedding.ItemID, &embedding.Model, &vectorText, &embedding.TextHash, &status, &embedding.LastError, &embedding.CreatedAt, &embedding.UpdatedAt)
	if err != nil {
		return hotspot.Embedding{}, err
	}
	embedding.Status = hotspot.EmbeddingStatus(status)
	vector, err := parseVectorLiteral(vectorText)
	if err != nil {
		return hotspot.Embedding{}, err
	}
	embedding.Vector = vector
	return embedding, nil
}

func (r *Repository) FindEmbedding(ctx context.Context, itemID string) (hotspot.Embedding, error) {
	const query = `
SELECT item_id, model, embedding::text, text_hash, status, COALESCE(last_error, ''), created_at, updated_at
FROM item_embeddings
WHERE item_id = $1
ORDER BY updated_at DESC
LIMIT 1`
	var embedding hotspot.Embedding
	var vectorText string
	var status string
	err := r.db.QueryRowContext(ctx, query, itemID).Scan(&embedding.ItemID, &embedding.Model, &vectorText, &embedding.TextHash, &status, &embedding.LastError, &embedding.CreatedAt, &embedding.UpdatedAt)
	if err == sql.ErrNoRows {
		return hotspot.Embedding{}, hotspot.ErrNotFound
	}
	if err != nil {
		return hotspot.Embedding{}, err
	}
	embedding.Status = hotspot.EmbeddingStatus(status)
	vector, err := parseVectorLiteral(vectorText)
	if err != nil {
		return hotspot.Embedding{}, err
	}
	embedding.Vector = vector
	return embedding, nil
}

func (r *Repository) ListCandidates(ctx context.Context, start time.Time, end time.Time) ([]hotspot.Candidate, error) {
	const query = `
SELECT
	i.id, i.source_id, i.title, i.snippet, i.raw_url, i.canonical_url, i.published_at, i.content_hash, i.language, i.status, COALESCE(i.duplicate_of_item_id, ''), i.created_at, i.updated_at,
	e.item_id, e.model, e.embedding::text, e.text_hash, e.status, COALESCE(e.last_error, ''), e.created_at, e.updated_at
FROM source_items i
JOIN item_embeddings e ON e.item_id = i.id
WHERE e.status = 'succeeded'
  AND COALESCE(i.published_at, i.created_at) >= $1
  AND COALESCE(i.published_at, i.created_at) < $2
ORDER BY COALESCE(i.published_at, i.created_at), i.id`
	rows, err := r.db.QueryContext(ctx, query, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candidates []hotspot.Candidate
	for rows.Next() {
		var candidate hotspot.Candidate
		var duplicateOf string
		var embeddingStatus string
		var vectorText string
		if err := rows.Scan(
			&candidate.Item.ID, &candidate.Item.SourceID, &candidate.Item.Title, &candidate.Item.Snippet, &candidate.Item.RawURL, &candidate.Item.CanonicalURL, &candidate.Item.PublishedAt, &candidate.Item.ContentHash, &candidate.Item.Language, &candidate.Item.Status, &duplicateOf, &candidate.Item.CreatedAt, &candidate.Item.UpdatedAt,
			&candidate.Embedding.ItemID, &candidate.Embedding.Model, &vectorText, &candidate.Embedding.TextHash, &embeddingStatus, &candidate.Embedding.LastError, &candidate.Embedding.CreatedAt, &candidate.Embedding.UpdatedAt,
		); err != nil {
			return nil, err
		}
		candidate.Item.DuplicateOfItemID = duplicateOf
		candidate.Embedding.Status = hotspot.EmbeddingStatus(embeddingStatus)
		vector, err := parseVectorLiteral(vectorText)
		if err != nil {
			return nil, err
		}
		candidate.Embedding.Vector = vector
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (r *Repository) ReplaceClusters(ctx context.Context, clusters []hotspot.Cluster, itemsByCluster map[string][]hotspot.ClusterItem) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM hotspot_items`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM hotspot_clusters`); err != nil {
		return err
	}
	for _, cluster := range clusters {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO hotspot_clusters (id, title, keywords, centroid, window_start, window_end, created_at, updated_at)
VALUES ($1, $2, $3, $4::vector, $5, $6, $7, $8)
`, cluster.ID, cluster.Title, arrayLiteral(cluster.Keywords), vectorLiteral(cluster.Centroid), cluster.WindowStart, cluster.WindowEnd, cluster.CreatedAt, cluster.UpdatedAt); err != nil {
			return err
		}
		for _, item := range itemsByCluster[cluster.ID] {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO hotspot_items (cluster_id, item_id, similarity, created_at)
VALUES ($1, $2, $3, $4)
`, cluster.ID, item.ItemID, item.Similarity, item.CreatedAt); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func vectorLiteral(vector []float64) string {
	parts := make([]string, len(vector))
	for i, value := range vector {
		parts[i] = strconv.FormatFloat(value, 'f', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func parseVectorLiteral(value string) ([]float64, error) {
	value = strings.Trim(value, "[] ")
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	vector := make([]float64, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, fmt.Errorf("parse vector literal %q: %w", value, err)
		}
		vector = append(vector, parsed)
	}
	return vector, nil
}

func arrayLiteral(values []string) string {
	escaped := make([]string, len(values))
	for i, value := range values {
		value = strings.ReplaceAll(value, `\`, `\\`)
		value = strings.ReplaceAll(value, `"`, `\"`)
		escaped[i] = `"` + value + `"`
	}
	return "{" + strings.Join(escaped, ",") + "}"
}
