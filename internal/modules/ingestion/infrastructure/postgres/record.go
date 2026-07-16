// Package postgres contains ingestion-owned Content persistence adapters.
// It does not read Source-owned collection tables; ingestion receives durable
// captures through Source's application boundary.
package postgres

import (
	"database/sql"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

type contentRecord struct {
	id, version, sourceConnectionID                int64
	externalID, contentType, title, excerpt        string
	canonicalURL, language, contentHash, status    string
	publishedAt, fetchedAt                         time.Time
	authorExternalID, authorDisplayName            sql.NullString
	duplicateOfID                                  sql.NullInt64
	dedupeReason, dedupeVersion                    sql.NullString
	viewCount, likeCount, commentCount, shareCount sql.NullInt64
	deletedAt                                      sql.NullTime
}

const contentColumns = `
c.id, c.version, c.source_connection_id, c.external_id,
c.content_type, c.title, c.excerpt, c.canonical_url, c.language,
c.published_at, c.fetched_at, c.dedupe_key, c.content_status,
c.duplicate_of_id, c.dedupe_reason, c.dedupe_version,
c.view_count, c.like_count, c.comment_count, c.share_count, c.deleted_at,
author.external_id, author.display_name`

func scanContent(scanner interface{ Scan(...any) error }) (ingestiondomain.Content, error) {
	var record contentRecord
	if err := scanner.Scan(
		&record.id, &record.version, &record.sourceConnectionID, &record.externalID,
		&record.contentType, &record.title, &record.excerpt, &record.canonicalURL, &record.language,
		&record.publishedAt, &record.fetchedAt, &record.contentHash, &record.status,
		&record.duplicateOfID, &record.dedupeReason, &record.dedupeVersion,
		&record.viewCount, &record.likeCount, &record.commentCount, &record.shareCount, &record.deletedAt,
		&record.authorExternalID, &record.authorDisplayName,
	); err != nil {
		return ingestiondomain.Content{}, err
	}
	content := ingestiondomain.Content{
		ID:                 record.id,
		Version:            record.version,
		SourceConnectionID: record.sourceConnectionID,
		ExternalID:         record.externalID,
		ContentType:        record.contentType,
		Title:              record.title,
		Excerpt:            record.excerpt,
		CanonicalURL:       record.canonicalURL,
		Language:           record.language,
		PublishedAt:        record.publishedAt.UTC(),
		FetchedAt:          record.fetchedAt.UTC(),
		ContentHash:        record.contentHash,
		Metrics: sourcedomain.SourceMetrics{
			ViewCount:    nullableMetric(record.viewCount),
			LikeCount:    nullableMetric(record.likeCount),
			CommentCount: nullableMetric(record.commentCount),
			ShareCount:   nullableMetric(record.shareCount),
		},
		Status:        ingestiondomain.ContentStatus(record.status),
		DedupeReason:  record.dedupeReason.String,
		DedupeVersion: record.dedupeVersion.String,
	}
	if record.authorExternalID.Valid {
		content.Author.ExternalID = record.authorExternalID.String
		content.Author.DisplayName = record.authorDisplayName.String
	}
	if record.duplicateOfID.Valid {
		value := record.duplicateOfID.Int64
		content.DuplicateOfID = &value
	}
	if record.deletedAt.Valid {
		value := record.deletedAt.Time.UTC()
		content.DeletedAt = &value
	}
	return content, nil
}

func nullableMetric(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	metric := value.Int64
	return &metric
}

type assetStatusRecord struct {
	version int64
	status  string
}
