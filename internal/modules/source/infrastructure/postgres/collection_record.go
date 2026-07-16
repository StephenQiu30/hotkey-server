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

type collectionRunSummaryRecord struct {
	id                                           int64
	status                                       string
	candidateCount, acceptedCount, rejectedCount int64
	errorCode                                    sql.NullString
	startedAt, finishedAt                        sql.NullTime
}

func scanCollectionRunSummary(scanner interface{ Scan(...any) error }) (domain.CollectionRunSummary, error) {
	var record collectionRunSummaryRecord
	if err := scanner.Scan(
		&record.id, &record.status, &record.candidateCount, &record.acceptedCount, &record.rejectedCount,
		&record.errorCode, &record.startedAt, &record.finishedAt,
	); err != nil {
		return domain.CollectionRunSummary{}, err
	}
	summary := domain.CollectionRunSummary{
		ID: record.id, Status: domain.CollectionRunStatus(record.status), CandidateCount: record.candidateCount,
		AcceptedCount: record.acceptedCount, RejectedCount: record.rejectedCount, ErrorCode: record.errorCode.String,
	}
	if record.startedAt.Valid {
		value := record.startedAt.Time.UTC()
		summary.StartedAt = &value
	}
	if record.finishedAt.Valid {
		value := record.finishedAt.Time.UTC()
		summary.FinishedAt = &value
	}
	return summary, nil
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

const collectionRunSummaryColumns = `
id, status, candidate_count, accepted_count, rejected_count, error_code, started_at, finished_at`
