package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type TextQuoteSelectorRepository struct{ runtime *database.Runtime }

var _ ingestionapplication.TextQuoteSelectorRepository = (*TextQuoteSelectorRepository)(nil)

func NewTextQuoteSelectorRepository(runtime *database.Runtime) *TextQuoteSelectorRepository {
	return &TextQuoteSelectorRepository{runtime: runtime}
}

type textQuoteSelectorExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type textQuoteAnchorRecord struct {
	Ordinal                int    `json:"ordinal"`
	PlaintextUTF8ByteStart int64  `json:"plaintext_utf8_byte_start"`
	PlaintextUTF8ByteEnd   int64  `json:"plaintext_utf8_byte_end"`
	MarkdownAnchor         string `json:"markdown_anchor"`
}

type textQuoteTargetRecord struct {
	sourceConnectionID, documentID, documentVersionID int64
	contentSHA256, documentLifecycleState             string
	plaintextArtifactID                               int64
	plaintextTransformerProfileSHA256                 string
	plaintextMIMEType, plaintextSHA256                string
	plaintextSizeBytes                                int64
	plaintextRetentionUntil                           time.Time
	markdownArtifactID                                int64
	anchorMapSHA256                                   string
	anchorBlocksJSON                                  []byte
	quoteDecisionID, retainDecisionID                 int64
	retentionUntil, decisionAt                        time.Time
}

func (repository *TextQuoteSelectorRepository) ReadTextQuoteSelectorTarget(ctx context.Context, query ingestionapplication.TextQuoteSelectorTargetQuery) (ingestionapplication.TextQuoteSelectorTargetDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return ingestionapplication.TextQuoteSelectorTargetDTO{}, sharedrepository.ErrUnavailable
	}
	if query.DocumentVersionID <= 0 || !lowerHexTextQuoteDigest(query.PlaintextSHA256) || query.DecisionAt.IsZero() {
		return ingestionapplication.TextQuoteSelectorTargetDTO{}, fmt.Errorf("%w: invalid text quote selector target query", sharedrepository.ErrInvalidInput)
	}
	row := repository.executor(ctx).QueryRowContext(ctx, `
WITH target AS (
  SELECT document.source_connection_id,document.id AS document_id,version.id AS document_version_id,
         btrim(version.content_sha256) AS content_sha256,version.lifecycle_state
  FROM document_versions AS version
  JOIN documents AS document ON document.id=version.document_id
  WHERE version.id=$1 AND version.content_sha256=$2 AND version.lifecycle_state='readable'
), artifacts AS (
  SELECT target.*,
         plaintext.id AS plaintext_artifact_id,btrim(plaintext.transformer_profile_sha256) AS plaintext_transformer_profile_sha256,
         plaintext.mime_type AS plaintext_mime_type,btrim(plaintext.sha256) AS plaintext_sha256,
         plaintext.size_bytes AS plaintext_size_bytes,plaintext.retention_until AS plaintext_retention_until,
         markdown.id AS markdown_artifact_id,btrim(markdown.anchor_map_sha256) AS anchor_map_sha256,
         markdown.retention_until AS markdown_retention_until
  FROM target
  JOIN derived_artifacts AS plaintext ON plaintext.document_version_id=target.document_version_id
    AND plaintext.source_connection_id=target.source_connection_id
    AND plaintext.artifact_type='plaintext' AND plaintext.lifecycle_state='derived_available' AND plaintext.active
    AND plaintext.sha256=target.content_sha256 AND plaintext.retention_until>$3
    AND current_rights_action_allowed(plaintext.store_derived_rights_decision_id,target.source_connection_id,
      'document_version',target.document_version_id::text,target.content_sha256,'store_derived',$3)
    AND current_rights_action_allowed(plaintext.retain_rights_decision_id,target.source_connection_id,
      'document_version',target.document_version_id::text,target.content_sha256,'retain',$3)
  JOIN derived_artifacts AS markdown ON markdown.document_version_id=target.document_version_id
    AND markdown.source_connection_id=target.source_connection_id
    AND markdown.artifact_type='markdown' AND markdown.lifecycle_state='derived_available' AND markdown.active
    AND markdown.anchor_plaintext_sha256=target.content_sha256 AND markdown.anchor_map_sha256 IS NOT NULL
    AND markdown.retention_until>$3
    AND current_rights_action_allowed(markdown.store_derived_rights_decision_id,target.source_connection_id,
      'document_version',target.document_version_id::text,target.content_sha256,'store_derived',$3)
    AND current_rights_action_allowed(markdown.retain_rights_decision_id,target.source_connection_id,
      'document_version',target.document_version_id::text,target.content_sha256,'retain',$3)
), rights_terminal AS (
  SELECT decision.*,artifacts.document_version_id,artifacts.content_sha256
  FROM artifacts
  JOIN source_rights_decisions AS decision ON decision.source_connection_id=artifacts.source_connection_id
    AND (
      (decision.subject_type='document_version'
       AND decision.subject_key=artifacts.document_version_id::text
       AND decision.input_digest=artifacts.content_sha256)
      OR
      (decision.subject_type='source_endpoint'
       AND decision.subject_key=artifacts.source_connection_id::text
       AND EXISTS (
         SELECT 1 FROM source_rights_policies AS policy
         WHERE policy.id=decision.policy_id AND policy.policy_hash=decision.input_digest
       ))
    )
    AND decision.action IN ('quote','retain')
  WHERE decision.effective_from<=$3
    AND (decision.expires_at IS NULL OR $3<decision.expires_at)
    AND NOT EXISTS (
      SELECT 1 FROM source_rights_decisions AS superseding
      WHERE superseding.supersedes_decision_id=decision.id
        AND superseding.effective_from<=$3
    )
), highest_rights_priority AS (
  SELECT action,max(priority_rank) AS priority_rank FROM rights_terminal GROUP BY action
), allowed_rights_action AS (
  SELECT terminal.action
  FROM rights_terminal AS terminal
  JOIN highest_rights_priority AS highest
    ON highest.action=terminal.action AND highest.priority_rank=terminal.priority_rank
  GROUP BY terminal.action
  HAVING bool_and(terminal.decision='allow')
), selected_rights AS (
  SELECT DISTINCT ON (terminal.action) terminal.id,terminal.action
  FROM rights_terminal AS terminal
  JOIN highest_rights_priority AS highest
    ON highest.action=terminal.action AND highest.priority_rank=terminal.priority_rank
  JOIN allowed_rights_action AS allowed ON allowed.action=terminal.action
  WHERE terminal.decision='allow'
  ORDER BY terminal.action,
           CASE WHEN terminal.action='retain' THEN terminal.retention_days END ASC NULLS LAST,
           terminal.effective_from DESC,terminal.id DESC
), selected_quote AS (
  SELECT id FROM selected_rights WHERE action='quote'
), selected_retain AS (
  SELECT id FROM selected_rights WHERE action='retain'
)
SELECT artifacts.source_connection_id,artifacts.document_id,artifacts.document_version_id,
       artifacts.content_sha256,artifacts.lifecycle_state,
       artifacts.plaintext_artifact_id,artifacts.plaintext_transformer_profile_sha256,
       artifacts.plaintext_mime_type,artifacts.plaintext_sha256,artifacts.plaintext_size_bytes,
       artifacts.plaintext_retention_until,artifacts.markdown_artifact_id,artifacts.anchor_map_sha256,
       COALESCE((SELECT jsonb_agg(jsonb_build_object(
         'ordinal',anchor.block_ordinal,'plaintext_utf8_byte_start',anchor.plaintext_utf8_byte_start,
         'plaintext_utf8_byte_end',anchor.plaintext_utf8_byte_end,'markdown_anchor',anchor.markdown_anchor
       ) ORDER BY anchor.block_ordinal)
       FROM document_anchor_blocks AS anchor
       WHERE anchor.derived_artifact_id=artifacts.markdown_artifact_id
         AND anchor.anchor_map_sha256=artifacts.anchor_map_sha256),'[]'::jsonb),
       selected_quote.id,selected_retain.id,
       LEAST(artifacts.plaintext_retention_until,artifacts.markdown_retention_until),$3::timestamptz
FROM artifacts CROSS JOIN selected_quote CROSS JOIN selected_retain`, query.DocumentVersionID, query.PlaintextSHA256, query.DecisionAt.UTC())
	record, err := scanTextQuoteTargetRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.TextQuoteSelectorTargetDTO{}, fmt.Errorf("%w: quote target is not currently authorized", sharedrepository.ErrNotFound)
	}
	if err != nil {
		return ingestionapplication.TextQuoteSelectorTargetDTO{}, databaserepository.MapError(err)
	}
	return textQuoteTargetDTO(record)
}

func scanTextQuoteTargetRecord(row *sql.Row) (textQuoteTargetRecord, error) {
	var record textQuoteTargetRecord
	err := row.Scan(&record.sourceConnectionID, &record.documentID, &record.documentVersionID,
		&record.contentSHA256, &record.documentLifecycleState,
		&record.plaintextArtifactID, &record.plaintextTransformerProfileSHA256,
		&record.plaintextMIMEType, &record.plaintextSHA256, &record.plaintextSizeBytes,
		&record.plaintextRetentionUntil, &record.markdownArtifactID, &record.anchorMapSHA256,
		&record.anchorBlocksJSON, &record.quoteDecisionID, &record.retainDecisionID,
		&record.retentionUntil, &record.decisionAt)
	return record, err
}

func textQuoteTargetDTO(record textQuoteTargetRecord) (ingestionapplication.TextQuoteSelectorTargetDTO, error) {
	var blocks []textQuoteAnchorRecord
	if err := json.Unmarshal(record.anchorBlocksJSON, &blocks); err != nil || len(blocks) == 0 {
		return ingestionapplication.TextQuoteSelectorTargetDTO{}, fmt.Errorf("%w: invalid text quote anchor map", sharedrepository.ErrConstraint)
	}
	result := ingestionapplication.TextQuoteSelectorTargetDTO{
		SourceConnectionID: record.sourceConnectionID, DocumentID: record.documentID,
		DocumentVersionID: record.documentVersionID, ContentSHA256: strings.TrimSpace(record.contentSHA256),
		DocumentLifecycleState: record.documentLifecycleState,
		PlaintextArtifact: ingestionapplication.TextQuoteProjectionArtifactDTO{
			ID: record.plaintextArtifactID, ArtifactType: ingestionapplication.DocumentProjectionPlaintext,
			TransformerProfileSHA256: strings.TrimSpace(record.plaintextTransformerProfileSHA256),
			MIMEType:                 record.plaintextMIMEType, SHA256: strings.TrimSpace(record.plaintextSHA256),
			SizeBytes: record.plaintextSizeBytes, RetentionUntil: record.plaintextRetentionUntil.UTC(),
		},
		MarkdownArtifactID: record.markdownArtifactID, AnchorMapSHA256: strings.TrimSpace(record.anchorMapSHA256),
		AnchorBlocks:          make([]ingestionapplication.TextQuoteAnchorBlockDTO, len(blocks)),
		QuoteRightsDecisionID: record.quoteDecisionID, RetainRightsDecisionID: record.retainDecisionID,
		RetentionUntil: record.retentionUntil.UTC(), DecisionAt: record.decisionAt.UTC(),
	}
	for index, block := range blocks {
		result.AnchorBlocks[index] = ingestionapplication.TextQuoteAnchorBlockDTO{
			Ordinal: block.Ordinal, PlaintextUTF8ByteStart: block.PlaintextUTF8ByteStart,
			PlaintextUTF8ByteEnd: block.PlaintextUTF8ByteEnd, MarkdownAnchor: block.MarkdownAnchor,
		}
	}
	return result, nil
}

type textQuoteSelectorRecord struct {
	id, version                                                         int64
	sourceConnectionID, documentVersionID                               int64
	plaintextArtifactID, markdownArtifactID                             int64
	quoteDecisionID, retainDecisionID                                   int64
	exactQuote, prefix, suffix                                          string
	utf8ByteStart, utf8ByteEnd                                          int64
	quoteSHA256, plaintextSHA256, normalizationVersion, selectorVersion string
	anchorMapSHA256                                                     string
	markdownAnchor                                                      sql.NullString
	retentionUntil, createdAt                                           time.Time
}

const textQuoteSelectorColumns = `id,version,source_connection_id,document_version_id,
plaintext_artifact_id,markdown_artifact_id,quote_rights_decision_id,retain_rights_decision_id,
exact_quote,prefix,suffix,utf8_byte_start,utf8_byte_end,quote_sha256,plaintext_sha256,
normalization_version,selector_version,anchor_map_sha256,markdown_anchor,retention_until,created_at`

func (repository *TextQuoteSelectorRepository) PersistTextQuoteSelector(ctx context.Context, command ingestionapplication.PersistTextQuoteSelectorCommand) (ingestionapplication.TextQuoteSelectorDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return ingestionapplication.TextQuoteSelectorDTO{}, sharedrepository.ErrUnavailable
	}
	if err := validatePersistTextQuoteSelectorCommand(command); err != nil {
		return ingestionapplication.TextQuoteSelectorDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	var stored textQuoteSelectorRecord
	err := repository.runtime.WithinTransaction(ctx, func(transactionContext context.Context, transaction database.Transaction) error {
		row := transaction.SQL.QueryRowContext(transactionContext, `
INSERT INTO document_text_quote_selectors (
  source_connection_id,document_version_id,plaintext_artifact_id,markdown_artifact_id,
  quote_rights_decision_id,retain_rights_decision_id,exact_quote,prefix,suffix,
  utf8_byte_start,utf8_byte_end,quote_sha256,plaintext_sha256,normalization_version,
  selector_version,anchor_map_sha256,markdown_anchor,retention_until
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
ON CONFLICT (document_version_id,plaintext_sha256,utf8_byte_start,utf8_byte_end,quote_sha256) DO NOTHING
RETURNING `+textQuoteSelectorColumns,
			command.SourceConnectionID, command.DocumentVersionID, command.PlaintextArtifactID, command.MarkdownArtifactID,
			command.QuoteRightsDecisionID, command.RetainRightsDecisionID, command.ExactQuote, command.Prefix, command.Suffix,
			command.UTF8ByteStart, command.UTF8ByteEnd, command.QuoteSHA256, command.PlaintextSHA256,
			command.NormalizationVersion, command.SelectorVersion, command.AnchorMapSHA256, command.MarkdownAnchor,
			command.RetentionUntil.UTC())
		var err error
		stored, err = scanTextQuoteSelectorRecord(row)
		if errors.Is(err, sql.ErrNoRows) {
			stored, err = scanTextQuoteSelectorRecord(transaction.SQL.QueryRowContext(transactionContext, `
SELECT `+textQuoteSelectorColumns+` FROM document_text_quote_selectors
WHERE document_version_id=$1 AND plaintext_sha256=$2 AND utf8_byte_start=$3 AND utf8_byte_end=$4 AND quote_sha256=$5
FOR SHARE`, command.DocumentVersionID, command.PlaintextSHA256, command.UTF8ByteStart, command.UTF8ByteEnd, command.QuoteSHA256))
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		if !textQuoteSelectorRecordMatches(stored, command) {
			return fmt.Errorf("%w: immutable text quote selector identity has different facts", sharedrepository.ErrConflict)
		}
		return nil
	})
	if err != nil {
		return ingestionapplication.TextQuoteSelectorDTO{}, err
	}
	return textQuoteSelectorDTO(stored), nil
}

func scanTextQuoteSelectorRecord(row *sql.Row) (textQuoteSelectorRecord, error) {
	var record textQuoteSelectorRecord
	err := row.Scan(&record.id, &record.version, &record.sourceConnectionID, &record.documentVersionID,
		&record.plaintextArtifactID, &record.markdownArtifactID, &record.quoteDecisionID, &record.retainDecisionID,
		&record.exactQuote, &record.prefix, &record.suffix, &record.utf8ByteStart, &record.utf8ByteEnd,
		&record.quoteSHA256, &record.plaintextSHA256, &record.normalizationVersion, &record.selectorVersion,
		&record.anchorMapSHA256, &record.markdownAnchor, &record.retentionUntil, &record.createdAt)
	return record, err
}

func validatePersistTextQuoteSelectorCommand(command ingestionapplication.PersistTextQuoteSelectorCommand) error {
	selector := ingestiondomain.TextQuoteSelector{
		ExactQuote: command.ExactQuote, Prefix: command.Prefix, Suffix: command.Suffix,
		UTF8ByteStart: command.UTF8ByteStart, UTF8ByteEnd: command.UTF8ByteEnd,
		QuoteSHA256: command.QuoteSHA256, PlaintextSHA256: command.PlaintextSHA256,
		NormalizationVersion: command.NormalizationVersion, SelectorVersion: command.SelectorVersion,
	}
	if command.SourceConnectionID <= 0 || command.DocumentVersionID <= 0 || command.PlaintextArtifactID <= 0 ||
		command.MarkdownArtifactID <= 0 || command.QuoteRightsDecisionID <= 0 || command.RetainRightsDecisionID <= 0 ||
		!lowerHexTextQuoteDigest(command.AnchorMapSHA256) || !command.RetentionUntil.After(command.DecisionAt) || command.DecisionAt.IsZero() {
		return errors.New("text quote selector persistence identity is invalid")
	}
	return ingestiondomain.ValidateTextQuoteSelector(selector)
}

func textQuoteSelectorRecordMatches(record textQuoteSelectorRecord, command ingestionapplication.PersistTextQuoteSelectorCommand) bool {
	return record.id > 0 && record.version > 0 && record.sourceConnectionID == command.SourceConnectionID &&
		record.documentVersionID == command.DocumentVersionID && record.plaintextArtifactID == command.PlaintextArtifactID &&
		record.markdownArtifactID == command.MarkdownArtifactID && record.quoteDecisionID == command.QuoteRightsDecisionID &&
		record.retainDecisionID == command.RetainRightsDecisionID && record.exactQuote == command.ExactQuote &&
		record.prefix == command.Prefix && record.suffix == command.Suffix && record.utf8ByteStart == command.UTF8ByteStart &&
		record.utf8ByteEnd == command.UTF8ByteEnd && strings.TrimSpace(record.quoteSHA256) == command.QuoteSHA256 &&
		strings.TrimSpace(record.plaintextSHA256) == command.PlaintextSHA256 && record.normalizationVersion == command.NormalizationVersion &&
		record.selectorVersion == command.SelectorVersion && strings.TrimSpace(record.anchorMapSHA256) == command.AnchorMapSHA256 &&
		optionalTextQuoteStringMatches(record.markdownAnchor, command.MarkdownAnchor) && record.retentionUntil.Equal(command.RetentionUntil)
}

func textQuoteSelectorDTO(record textQuoteSelectorRecord) ingestionapplication.TextQuoteSelectorDTO {
	return ingestionapplication.TextQuoteSelectorDTO{
		ID: record.id, Version: record.version, SourceConnectionID: record.sourceConnectionID, DocumentVersionID: record.documentVersionID,
		PlaintextArtifactID: record.plaintextArtifactID, MarkdownArtifactID: record.markdownArtifactID,
		QuoteRightsDecisionID: record.quoteDecisionID, RetainRightsDecisionID: record.retainDecisionID,
		ExactQuote: record.exactQuote, Prefix: record.prefix, Suffix: record.suffix,
		UTF8ByteStart: record.utf8ByteStart, UTF8ByteEnd: record.utf8ByteEnd,
		QuoteSHA256: strings.TrimSpace(record.quoteSHA256), PlaintextSHA256: strings.TrimSpace(record.plaintextSHA256),
		NormalizationVersion: record.normalizationVersion, SelectorVersion: record.selectorVersion,
		AnchorMapSHA256: strings.TrimSpace(record.anchorMapSHA256), MarkdownAnchor: nullableTextQuoteString(record.markdownAnchor),
		RetentionUntil: record.retentionUntil.UTC(), CreatedAt: record.createdAt.UTC(),
	}
}

func optionalTextQuoteStringMatches(record sql.NullString, value *string) bool {
	if value == nil {
		return !record.Valid
	}
	return record.Valid && record.String == *value
}

func nullableTextQuoteString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func lowerHexTextQuoteDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for index := range value {
		if (value[index] < '0' || value[index] > '9') && (value[index] < 'a' || value[index] > 'f') {
			return false
		}
	}
	return true
}

func (repository *TextQuoteSelectorRepository) executor(ctx context.Context) textQuoteSelectorExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}
