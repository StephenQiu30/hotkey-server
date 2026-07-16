package postgres

import (
	"database/sql"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

type collectionRunRecord struct {
	id, sourceConnectionID    int64
	querySignature            string
	requestCursor, nextCursor sql.NullString
	etag, lastModified        sql.NullString
	retryAfter                sql.NullTime
	pageCount                 int
	windowStart, windowEnd    time.Time
	status                    string
}

func scanCollectionRun(scanner interface{ Scan(...any) error }) (domain.CollectionRun, error) {
	var record collectionRunRecord
	if err := scanner.Scan(
		&record.id, &record.sourceConnectionID, &record.querySignature,
		&record.requestCursor, &record.nextCursor, &record.etag, &record.lastModified,
		&record.retryAfter, &record.pageCount, &record.windowStart, &record.windowEnd, &record.status,
	); err != nil {
		return domain.CollectionRun{}, err
	}
	run := domain.CollectionRun{
		ID: record.id, SourceConnectionID: record.sourceConnectionID, QuerySignature: record.querySignature,
		RequestCursor: record.requestCursor.String, NextCursor: record.nextCursor.String,
		ETag: record.etag.String, LastModified: record.lastModified.String, PageCount: record.pageCount,
		WindowStart: record.windowStart.UTC(), WindowEnd: record.windowEnd.UTC(), Status: domain.CollectionRunStatus(record.status),
	}
	if record.retryAfter.Valid {
		value := record.retryAfter.Time.UTC()
		run.RetryAfter = &value
	}
	return run, nil
}

const collectionRunColumns = `
id, source_connection_id, query_signature, request_cursor, next_cursor, etag,
last_modified, retry_after, page_count, window_start, window_end, status`
