package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// EvidenceSelectionManifestReader owns the Source persistence projection for
// one immutable observation-to-snapshot reference. The query deliberately
// keeps MinIO identity inside Source Application and evaluates both current
// rights actions in the same PostgreSQL statement snapshot.
type EvidenceSelectionManifestReader struct {
	runtime *database.Runtime
}

var _ sourceapplication.EvidenceSelectionManifestReader = (*EvidenceSelectionManifestReader)(nil)

func NewEvidenceSelectionManifestReader(runtime *database.Runtime) *EvidenceSelectionManifestReader {
	return &EvidenceSelectionManifestReader{runtime: runtime}
}

type evidenceSelectionManifestQueryExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// evidenceSelectionManifestRecord is private to the PostgreSQL adapter. SQL
// nullability and serialized JSON never cross the Infrastructure boundary.
type evidenceSelectionManifestRecord struct {
	evidenceReferenceID int64
	sourceObservationID int64
	evidenceSnapshotID  int64
	sourceConnectionID  int64

	externalID       string
	upstreamIdentity string
	sourceCode       string
	contentType      string
	title            string
	language         string
	author           sql.NullString
	sourceRecordURL  sql.NullString
	canonicalURL     sql.NullString
	discussionURL    sql.NullString
	bodyOrigin       string
	completeness     string
	publishedAt      sql.NullTime
	discoveredAt     time.Time
	observationState string

	lifecycleState          string
	evidenceKey             string
	objectKey               string
	payloadSHA256           string
	collectorProfileVersion string
	mimeType                string
	sizeBytes               int64
	responseStatus          int
	requestedURL            string
	finalURL                string
	redirectChainJSON       []byte
	responseHeadersJSON     []byte
	capturedAt              time.Time
	retentionUntil          time.Time
	storeRawAllowed         bool
	retainAllowed           bool
	currentRetentionDays    sql.NullInt64
	rightsEvaluatedAt       time.Time

	locatorType           string
	locatorValue          string
	byteStart             sql.NullInt64
	byteEnd               sql.NullInt64
	selectedPayloadSHA256 string
	selectorVersion       string
}

type evidenceSelectionManifestRow interface {
	Scan(...any) error
}

func (reader *EvidenceSelectionManifestReader) ReadEvidenceSelectionManifest(ctx context.Context, evidenceReferenceID int64) (sourceapplication.EvidenceSelectionManifestDTO, error) {
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil {
		return sourceapplication.EvidenceSelectionManifestDTO{}, sharedrepository.ErrUnavailable
	}
	if evidenceReferenceID <= 0 {
		return sourceapplication.EvidenceSelectionManifestDTO{}, fmt.Errorf("%w: invalid evidence reference id", sharedrepository.ErrInvalidInput)
	}
	record, err := scanEvidenceSelectionManifestRecord(reader.executor(ctx).QueryRowContext(ctx, `
SELECT
  reference.id,
  observation.id,
  snapshot.id,
  reference.source_connection_id,
  observation.external_id,
  btrim(observation.upstream_identity),
  observation.source_code,
  observation.content_type,
  observation.title,
  observation.language,
  observation.author_snapshot,
  observation.source_record_url,
  observation.canonical_url,
  observation.discussion_url,
  observation.body_origin,
  observation.completeness,
  observation.published_at,
  observation.discovered_at,
  observation.observation_state,
  snapshot.lifecycle_state,
  btrim(snapshot.snapshot_key),
  snapshot.object_key,
  btrim(snapshot.payload_sha256),
  snapshot.collector_profile_version,
  snapshot.mime_type,
  snapshot.size_bytes,
  snapshot.response_status,
  snapshot.requested_url,
  snapshot.final_url,
  snapshot.redirect_chain,
  snapshot.response_headers,
  snapshot.captured_at,
  snapshot.retention_until,
  current_rights_action_allowed(
    snapshot.store_raw_rights_decision_id,
    snapshot.source_connection_id,
    'raw_response', btrim(snapshot.snapshot_key), snapshot.payload_sha256,
    'store_raw', CURRENT_TIMESTAMP
  ) AS store_raw_allowed,
  current_rights_action_allowed(
    snapshot.retain_rights_decision_id,
    snapshot.source_connection_id,
    'raw_response', btrim(snapshot.snapshot_key), snapshot.payload_sha256,
    'retain', CURRENT_TIMESTAMP
  ) AS retain_allowed,
  current_rights_retention_days(
    snapshot.source_connection_id,
    'raw_response', btrim(snapshot.snapshot_key), snapshot.payload_sha256,
    CURRENT_TIMESTAMP
  ) AS current_retention_days,
  CURRENT_TIMESTAMP AS rights_evaluated_at,
  reference.locator_type,
  reference.locator_value,
  reference.byte_start,
  reference.byte_end,
  btrim(reference.selected_payload_sha256),
  reference.selector_version
FROM source_observation_evidences AS reference
JOIN source_observations AS observation
  ON observation.id=reference.source_observation_id
 AND observation.source_connection_id=reference.source_connection_id
JOIN evidence_snapshots AS snapshot
  ON snapshot.id=reference.evidence_snapshot_id
 AND snapshot.source_connection_id=reference.source_connection_id
WHERE reference.id=$1`, evidenceReferenceID))
	if errors.Is(err, sql.ErrNoRows) {
		return sourceapplication.EvidenceSelectionManifestDTO{}, fmt.Errorf("%w: evidence reference %d", sharedrepository.ErrNotFound, evidenceReferenceID)
	}
	if err != nil {
		return sourceapplication.EvidenceSelectionManifestDTO{}, databaserepository.MapError(err)
	}
	manifest, err := record.applicationDTO()
	if err != nil {
		return sourceapplication.EvidenceSelectionManifestDTO{}, fmt.Errorf("%w: persisted evidence selection manifest is invalid", sharedrepository.ErrConstraint)
	}
	return manifest, nil
}

func (reader *EvidenceSelectionManifestReader) executor(ctx context.Context) evidenceSelectionManifestQueryExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return reader.runtime.SQL
}

func scanEvidenceSelectionManifestRecord(row evidenceSelectionManifestRow) (evidenceSelectionManifestRecord, error) {
	var record evidenceSelectionManifestRecord
	err := row.Scan(
		&record.evidenceReferenceID, &record.sourceObservationID, &record.evidenceSnapshotID, &record.sourceConnectionID,
		&record.externalID, &record.upstreamIdentity, &record.sourceCode, &record.contentType, &record.title, &record.language,
		&record.author, &record.sourceRecordURL, &record.canonicalURL, &record.discussionURL,
		&record.bodyOrigin, &record.completeness, &record.publishedAt, &record.discoveredAt, &record.observationState,
		&record.lifecycleState, &record.evidenceKey, &record.objectKey, &record.payloadSHA256,
		&record.collectorProfileVersion, &record.mimeType, &record.sizeBytes, &record.responseStatus,
		&record.requestedURL, &record.finalURL, &record.redirectChainJSON, &record.responseHeadersJSON,
		&record.capturedAt, &record.retentionUntil, &record.storeRawAllowed, &record.retainAllowed,
		&record.currentRetentionDays, &record.rightsEvaluatedAt,
		&record.locatorType, &record.locatorValue, &record.byteStart, &record.byteEnd,
		&record.selectedPayloadSHA256, &record.selectorVersion,
	)
	return record, err
}

func (record evidenceSelectionManifestRecord) applicationDTO() (sourceapplication.EvidenceSelectionManifestDTO, error) {
	redirectChain, err := decodeRedirectChain(record.redirectChainJSON)
	if err != nil {
		return sourceapplication.EvidenceSelectionManifestDTO{}, err
	}
	responseHeaders, err := decodeResponseHeaders(record.responseHeadersJSON)
	if err != nil {
		return sourceapplication.EvidenceSelectionManifestDTO{}, err
	}
	return sourceapplication.EvidenceSelectionManifestDTO{
		EvidenceReferenceID: record.evidenceReferenceID, SourceObservationID: record.sourceObservationID,
		EvidenceSnapshotID: record.evidenceSnapshotID, SourceConnectionID: record.sourceConnectionID,
		ExternalID: record.externalID, UpstreamIdentity: strings.TrimSpace(record.upstreamIdentity),
		SourceCode: record.sourceCode, ContentType: record.contentType, Title: record.title, Language: record.language,
		Author: evidenceSelectionString(record.author), SourceRecordURL: evidenceSelectionString(record.sourceRecordURL),
		CanonicalURL: evidenceSelectionString(record.canonicalURL), DiscussionURL: evidenceSelectionString(record.discussionURL),
		BodyOrigin: record.bodyOrigin, Completeness: record.completeness,
		PublishedAt: evidenceSelectionTime(record.publishedAt), DiscoveredAt: record.discoveredAt.UTC(), ObservationState: record.observationState,
		LifecycleState: record.lifecycleState, EvidenceKey: strings.TrimSpace(record.evidenceKey),
		ObjectKey: record.objectKey, PayloadSHA256: strings.TrimSpace(record.payloadSHA256),
		CollectorProfileVersion: record.collectorProfileVersion, MIMEType: record.mimeType,
		SizeBytes: record.sizeBytes, ResponseStatus: record.responseStatus,
		RequestedURL: record.requestedURL, FinalURL: record.finalURL, RedirectChain: redirectChain,
		ResponseHeaders: responseHeaders, CapturedAt: record.capturedAt.UTC(), RetentionUntil: record.retentionUntil.UTC(),
		StoreRawAllowed: record.storeRawAllowed, RetainAllowed: record.retainAllowed,
		CurrentRetentionDays: evidenceSelectionInt(record.currentRetentionDays), RightsEvaluatedAt: record.rightsEvaluatedAt.UTC(),
		EvidenceReference: sourceapplication.RawEvidenceReferenceDTO{
			EvidenceKey: strings.TrimSpace(record.evidenceKey), LocatorType: record.locatorType,
			LocatorValue: record.locatorValue, ByteStart: evidenceSelectionInt64(record.byteStart), ByteEnd: evidenceSelectionInt64(record.byteEnd),
			SelectedPayloadSHA256: strings.TrimSpace(record.selectedPayloadSHA256), SelectorVersion: record.selectorVersion,
		},
	}, nil
}

func evidenceSelectionString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func evidenceSelectionTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func evidenceSelectionInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}

func evidenceSelectionInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
