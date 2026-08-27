package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// CitationRepository reads one immutable DocumentVersion and evaluates its
// rights in the same PostgreSQL statement. It never selects object keys,
// Vault paths, credentials, or raw payloads.
type CitationRepository struct{ runtime *database.Runtime }

var _ ingestionapplication.CitationReader = (*CitationRepository)(nil)

func NewCitationRepository(runtime *database.Runtime) *CitationRepository {
	return &CitationRepository{runtime: runtime}
}

type citationQueryExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// citationRecord is private to the PostgreSQL adapter. Application DTOs are
// populated explicitly so persistence-only nullable and rights fields cannot
// cross the adapter boundary by accident.
type citationRecord struct {
	documentID                   int64
	documentVersionID            int64
	sourceConnectionID           int64
	documentState                string
	documentLifecycleState       string
	observationState             string
	sourceType                   string
	sourceName                   string
	title                        string
	author                       sql.NullString
	partyFactsJSON               []byte
	sourceRecordURL              sql.NullString
	canonicalURL                 sql.NullString
	discussionURL                sql.NullString
	bodyOrigin                   string
	completeness                 string
	language                     string
	publishedAt                  sql.NullTime
	capturedAt                   time.Time
	contentSHA256                string
	displayPrivateAllowed        bool
	rightsEvaluatedAt            time.Time
	rawEvidenceAvailability      string
	rawEvidencePayloadsJSON      []byte
	rawEvidenceRetentionUntil    sql.NullTime
	rawEvidenceDeletionAudited   bool
	rawEvidenceExceptionApproved bool

	artifactID                 sql.NullInt64
	artifactType               sql.NullString
	transformerProfileSHA256   sql.NullString
	mimeType                   sql.NullString
	artifactSHA256             sql.NullString
	sizeBytes                  sql.NullInt64
	artifactLifecycleState     sql.NullString
	artifactActive             sql.NullBool
	failureCode                sql.NullString
	availableAt                sql.NullTime
	retentionUntil             sql.NullTime
	storeDerivedAllowed        bool
	retainAllowed              bool
	currentRetentionDays       sql.NullInt64
	anchorNormalizationVersion sql.NullString
	anchorMapProfileVersion    sql.NullString
	anchorPlaintextSHA256      sql.NullString
	anchorMarkdownSHA256       sql.NullString
	anchorMapSHA256            sql.NullString
	anchorBlocksJSON           []byte
}

type citationAnchorBlockRecord struct {
	Ordinal                int    `json:"ordinal"`
	PlaintextUTF8ByteStart int64  `json:"plaintext_utf8_byte_start"`
	PlaintextUTF8ByteEnd   int64  `json:"plaintext_utf8_byte_end"`
	MarkdownUTF8ByteStart  int64  `json:"markdown_utf8_byte_start"`
	MarkdownUTF8ByteEnd    int64  `json:"markdown_utf8_byte_end"`
	MarkdownAnchor         string `json:"markdown_anchor"`
}

type citationPartyRecord struct {
	Role              string  `json:"role"`
	Kind              string  `json:"kind"`
	IdentityNamespace string  `json:"identity_namespace"`
	ExternalID        string  `json:"external_id"`
	DisplayName       string  `json:"display_name"`
	HomepageURL       *string `json:"homepage_url"`
}

func (repository *CitationRepository) ReadCitation(ctx context.Context, documentVersionID int64) (ingestionapplication.CitationReadDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return ingestionapplication.CitationReadDTO{}, sharedrepository.ErrUnavailable
	}
	if documentVersionID <= 0 {
		return ingestionapplication.CitationReadDTO{}, fmt.Errorf("%w: invalid document version id", sharedrepository.ErrInvalidInput)
	}
	record, err := scanCitationRecord(repository.executor(ctx).QueryRowContext(ctx, `
SELECT
  document.id,
  document_version.id,
  document.source_connection_id,
  document.document_state,
  document_version.lifecycle_state,
  observation.observation_state,
  source.source_type,
  source.name,
  observation.title,
  observation.author_snapshot,
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'role',relation.role,
      'kind',party.party_kind,
      'identity_namespace',party.identity_namespace,
      'external_id',party.external_id,
      'display_name',relation.display_name_snapshot,
      'homepage_url',relation.homepage_url_snapshot
    ) ORDER BY
      CASE relation.role WHEN 'publisher' THEN 0 WHEN 'content_origin' THEN 1 WHEN 'distributor' THEN 2 ELSE 3 END,
      party.identity_namespace,party.external_id)
    FROM source_observation_parties AS relation
    JOIN source_parties AS party
      ON party.id=relation.source_party_id AND party.source_connection_id=relation.source_connection_id
    WHERE relation.source_observation_id=observation.id
      AND relation.source_connection_id=observation.source_connection_id
  ),'[]'::jsonb) AS party_facts,
  observation.source_record_url,
  observation.canonical_url,
  observation.discussion_url,
  document_version.body_origin,
  document_version.completeness,
  document_version.language,
  observation.published_at,
  document_version.captured_at,
  btrim(document_version.content_sha256),
  current_rights_action_allowed(
    document_version.display_private_rights_decision_id,
    document.source_connection_id,
    'document_version', document_version.id::text, document_version.content_sha256,
    'display_private', CURRENT_TIMESTAMP
  ) AS display_private_allowed,
  CURRENT_TIMESTAMP AS rights_evaluated_at,
  raw_evidence.availability,
  raw_evidence.payload_sha256s,
  raw_evidence.retention_until,
  raw_evidence.deletion_audited,
  raw_evidence.exception_approved,
  artifact.id,
  artifact.artifact_type,
  btrim(artifact.transformer_profile_sha256),
  artifact.mime_type,
  btrim(artifact.sha256),
  artifact.size_bytes,
  artifact.anchor_normalization_version,
  artifact.anchor_map_profile_version,
  btrim(artifact.anchor_plaintext_sha256),
  btrim(artifact.anchor_markdown_sha256),
  btrim(artifact.anchor_map_sha256),
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'ordinal',anchor.block_ordinal,
      'plaintext_utf8_byte_start',anchor.plaintext_utf8_byte_start,
      'plaintext_utf8_byte_end',anchor.plaintext_utf8_byte_end,
      'markdown_utf8_byte_start',anchor.markdown_utf8_byte_start,
      'markdown_utf8_byte_end',anchor.markdown_utf8_byte_end,
      'markdown_anchor',anchor.markdown_anchor
    ) ORDER BY anchor.block_ordinal)
    FROM document_anchor_blocks AS anchor
    WHERE anchor.derived_artifact_id=artifact.id
      AND anchor.anchor_map_sha256=artifact.anchor_map_sha256
  ),'[]'::jsonb),
  artifact.lifecycle_state,
  artifact.active,
  artifact.failure_code,
  artifact.available_at,
  artifact.retention_until,
  CASE WHEN artifact.id IS NULL THEN false ELSE current_rights_action_allowed(
    artifact.store_derived_rights_decision_id,
    document.source_connection_id,
    'document_version', document_version.id::text, document_version.content_sha256,
    'store_derived', CURRENT_TIMESTAMP
  ) END AS store_derived_allowed,
  CASE WHEN artifact.id IS NULL THEN false ELSE current_rights_action_allowed(
    artifact.retain_rights_decision_id,
    document.source_connection_id,
    'document_version', document_version.id::text, document_version.content_sha256,
    'retain', CURRENT_TIMESTAMP
  ) END AS retain_allowed,
  CASE WHEN artifact.id IS NULL THEN NULL ELSE current_rights_retention_days(
    document.source_connection_id,
    'document_version', document_version.id::text, document_version.content_sha256,
    CURRENT_TIMESTAMP
  ) END AS current_retention_days
FROM document_versions AS document_version
JOIN documents AS document ON document.id = document_version.document_id
JOIN source_observations AS observation ON observation.id = document_version.source_observation_id
JOIN source_connections AS source ON source.id = document.source_connection_id
LEFT JOIN LATERAL (
  SELECT
    CASE
      WHEN count(snapshot.id)=0 THEN 'unavailable'
      WHEN bool_or(snapshot.lifecycle_state='raw_available' AND EXISTS (
        SELECT 1 FROM evidence_retention_exceptions AS exception
        WHERE exception.evidence_snapshot_id=snapshot.id AND exception.revoked_at IS NULL
          AND exception.approved_at <= CURRENT_TIMESTAMP
          AND (exception.expires_at IS NULL OR exception.expires_at > CURRENT_TIMESTAMP)
      )) THEN 'exception_retained'
      WHEN bool_or(snapshot.lifecycle_state='raw_available' AND snapshot.retention_until > CURRENT_TIMESTAMP) THEN 'available'
      WHEN bool_and(snapshot.lifecycle_state IN ('retention_blocked','tombstoned') OR snapshot.retention_until <= CURRENT_TIMESTAMP) THEN 'expired'
      ELSE 'unavailable'
    END AS availability,
    COALESCE(jsonb_agg(DISTINCT btrim(snapshot.payload_sha256) ORDER BY btrim(snapshot.payload_sha256)) FILTER (WHERE snapshot.id IS NOT NULL),'[]'::jsonb) AS payload_sha256s,
    max(snapshot.retention_until) AS retention_until,
    COALESCE(bool_or(EXISTS (
      SELECT 1 FROM evidence_deletion_audits AS deletion
      WHERE deletion.evidence_snapshot_id=snapshot.id AND deletion.event_type='delete_succeeded'
    )),false) AS deletion_audited,
    COALESCE(bool_or(snapshot.lifecycle_state='raw_available' AND EXISTS (
      SELECT 1 FROM evidence_retention_exceptions AS exception
      WHERE exception.evidence_snapshot_id=snapshot.id AND exception.revoked_at IS NULL
        AND exception.approved_at <= CURRENT_TIMESTAMP
        AND (exception.expires_at IS NULL OR exception.expires_at > CURRENT_TIMESTAMP)
    )),false) AS exception_approved
  FROM source_observation_evidences AS reference
  JOIN evidence_snapshots AS snapshot ON snapshot.id=reference.evidence_snapshot_id
  WHERE reference.source_observation_id=observation.id
    AND reference.source_connection_id=observation.source_connection_id
) AS raw_evidence ON true
LEFT JOIN LATERAL (
  SELECT candidate.*
  FROM derived_artifacts AS candidate
  WHERE candidate.document_version_id = document_version.id
    AND candidate.source_connection_id = document.source_connection_id
    AND candidate.artifact_type = 'markdown'
  ORDER BY candidate.active DESC,
           (candidate.lifecycle_state = 'derived_available') DESC,
           candidate.created_at DESC,
           candidate.id DESC
  LIMIT 1
) AS artifact ON true
WHERE document_version.id = $1`, documentVersionID))
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.CitationReadDTO{}, fmt.Errorf("%w: document version %d", sharedrepository.ErrNotFound, documentVersionID)
	}
	if err != nil {
		return ingestionapplication.CitationReadDTO{}, databaserepository.MapError(err)
	}
	result, err := citationReadDTO(record)
	if err != nil {
		return ingestionapplication.CitationReadDTO{}, fmt.Errorf("%w: invalid persisted citation anchor map", sharedrepository.ErrConflict)
	}
	return result, nil
}

func (repository *CitationRepository) executor(ctx context.Context) citationQueryExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}

func scanCitationRecord(row *sql.Row) (citationRecord, error) {
	var record citationRecord
	err := row.Scan(
		&record.documentID, &record.documentVersionID, &record.sourceConnectionID,
		&record.documentState, &record.documentLifecycleState, &record.observationState,
		&record.sourceType, &record.sourceName, &record.title, &record.author, &record.partyFactsJSON,
		&record.sourceRecordURL, &record.canonicalURL, &record.discussionURL,
		&record.bodyOrigin, &record.completeness, &record.language, &record.publishedAt,
		&record.capturedAt, &record.contentSHA256, &record.displayPrivateAllowed, &record.rightsEvaluatedAt,
		&record.rawEvidenceAvailability, &record.rawEvidencePayloadsJSON, &record.rawEvidenceRetentionUntil,
		&record.rawEvidenceDeletionAudited, &record.rawEvidenceExceptionApproved,
		&record.artifactID, &record.artifactType, &record.transformerProfileSHA256, &record.mimeType,
		&record.artifactSHA256, &record.sizeBytes,
		&record.anchorNormalizationVersion, &record.anchorMapProfileVersion, &record.anchorPlaintextSHA256,
		&record.anchorMarkdownSHA256, &record.anchorMapSHA256, &record.anchorBlocksJSON,
		&record.artifactLifecycleState, &record.artifactActive,
		&record.failureCode, &record.availableAt, &record.retentionUntil,
		&record.storeDerivedAllowed, &record.retainAllowed, &record.currentRetentionDays,
	)
	return record, err
}

func citationReadDTO(record citationRecord) (ingestionapplication.CitationReadDTO, error) {
	parties, err := citationPartyReadDTOs(record.partyFactsJSON)
	if err != nil {
		return ingestionapplication.CitationReadDTO{}, err
	}
	result := ingestionapplication.CitationReadDTO{
		DocumentID: record.documentID, DocumentVersionID: record.documentVersionID,
		SourceConnectionID: record.sourceConnectionID, DocumentState: record.documentState,
		DocumentLifecycleState: record.documentLifecycleState,
		ObservationState:       record.observationState,
		SourceType:             record.sourceType, SourceName: record.sourceName, Title: record.title,
		Author: citationOptionalString(record.author), SourceRecordURL: citationOptionalString(record.sourceRecordURL),
		CanonicalURL: citationOptionalString(record.canonicalURL), DiscussionURL: citationOptionalString(record.discussionURL),
		BodyOrigin: record.bodyOrigin, Completeness: record.completeness,
		Language: record.language, PublishedAt: citationOptionalTime(record.publishedAt), CapturedAt: record.capturedAt.UTC(),
		ContentSHA256: record.contentSHA256, DisplayPrivateAllowed: record.displayPrivateAllowed,
		RightsEvaluatedAt: record.rightsEvaluatedAt.UTC(),
	}
	var rawEvidencePayloadSHA256s []string
	if err := json.Unmarshal(record.rawEvidencePayloadsJSON, &rawEvidencePayloadSHA256s); err != nil || rawEvidencePayloadSHA256s == nil {
		return ingestionapplication.CitationReadDTO{}, fmt.Errorf("decode raw evidence payload hashes")
	}
	result.RawEvidence = ingestionapplication.CitationRawEvidenceReadDTO{
		Availability:   ingestionapplication.CitationRawEvidenceAvailability(record.rawEvidenceAvailability),
		PayloadSHA256s: rawEvidencePayloadSHA256s, RetentionUntil: citationTime(record.rawEvidenceRetentionUntil),
		DeletionAudited: record.rawEvidenceDeletionAudited, ExceptionApproved: record.rawEvidenceExceptionApproved,
	}
	for index := range parties {
		party := parties[index]
		switch party.Role {
		case "publisher":
			if result.Publisher != nil {
				return ingestionapplication.CitationReadDTO{}, fmt.Errorf("multiple publisher party facts")
			}
			result.Publisher = &party
		case "content_origin":
			if result.ContentOrigin != nil {
				return ingestionapplication.CitationReadDTO{}, fmt.Errorf("multiple content origin party facts")
			}
			result.ContentOrigin = &party
		case "distributor":
			result.Distributors = append(result.Distributors, party)
		}
	}
	if record.artifactID.Valid {
		result.Artifact = &ingestionapplication.CitationArtifactReadDTO{
			ArtifactType: record.artifactType.String, TransformerProfileSHA256: record.transformerProfileSHA256.String,
			MIMEType: record.mimeType.String, SHA256: record.artifactSHA256.String, SizeBytes: record.sizeBytes.Int64,
			LifecycleState: record.artifactLifecycleState.String, Active: record.artifactActive.Bool,
			FailureCode: citationOptionalString(record.failureCode), AvailableAt: citationOptionalTime(record.availableAt),
			RetentionUntil: citationTime(record.retentionUntil), StoreDerivedAllowed: record.storeDerivedAllowed,
			RetainAllowed: record.retainAllowed, CurrentRetentionDays: citationOptionalInt(record.currentRetentionDays),
		}
		if record.anchorMapSHA256.Valid {
			var persisted []citationAnchorBlockRecord
			if err := json.Unmarshal(record.anchorBlocksJSON, &persisted); err != nil {
				return ingestionapplication.CitationReadDTO{}, err
			}
			result.Artifact.AnchorMap = &ingestionapplication.CitationArtifactAnchorMapReadDTO{
				NormalizationVersion:    record.anchorNormalizationVersion.String,
				AnchorMapProfileVersion: record.anchorMapProfileVersion.String,
				PlaintextSHA256:         record.anchorPlaintextSHA256.String, MarkdownSHA256: record.anchorMarkdownSHA256.String,
				AnchorMapSHA256: record.anchorMapSHA256.String,
				Blocks:          make([]ingestionapplication.CitationAnchorBlockReadDTO, len(persisted)),
			}
			for index, block := range persisted {
				result.Artifact.AnchorMap.Blocks[index] = ingestionapplication.CitationAnchorBlockReadDTO{
					Ordinal: block.Ordinal, PlaintextUTF8ByteStart: block.PlaintextUTF8ByteStart, PlaintextUTF8ByteEnd: block.PlaintextUTF8ByteEnd,
					MarkdownUTF8ByteStart: block.MarkdownUTF8ByteStart, MarkdownUTF8ByteEnd: block.MarkdownUTF8ByteEnd,
					MarkdownAnchor: block.MarkdownAnchor,
				}
			}
		}
	}
	return result, nil
}

func citationPartyReadDTOs(encoded []byte) ([]ingestionapplication.CitationPartyReadDTO, error) {
	var persisted []citationPartyRecord
	if err := json.Unmarshal(encoded, &persisted); err != nil || persisted == nil {
		return nil, fmt.Errorf("decode citation party facts")
	}
	result := make([]ingestionapplication.CitationPartyReadDTO, len(persisted))
	for index, party := range persisted {
		result[index] = ingestionapplication.CitationPartyReadDTO{
			Role: party.Role, Kind: party.Kind, IdentityNamespace: party.IdentityNamespace,
			ExternalID: party.ExternalID, DisplayName: party.DisplayName, HomepageURL: party.HomepageURL,
		}
	}
	return result, nil
}

func citationOptionalString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func citationOptionalTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func citationTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func citationOptionalInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}
