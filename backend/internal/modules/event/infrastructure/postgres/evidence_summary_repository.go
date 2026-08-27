package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type EvidenceSummaryPostgresRepository struct{ runtime *database.Runtime }

var _ eventapplication.EvidenceSummaryRepository = (*EvidenceSummaryPostgresRepository)(nil)

func NewEvidenceSummaryPostgresRepository(runtime *database.Runtime) (*EvidenceSummaryPostgresRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("evidence summary database runtime is required")
	}
	return &EvidenceSummaryPostgresRepository{runtime: runtime}, nil
}

type evidenceSummaryRecord struct {
	id, version, eventID, eventVersion int64
	profile, commandFingerprint        string
	createdAt                          time.Time
}

func (repository *EvidenceSummaryPostgresRepository) CommitEvidenceSummary(ctx context.Context, command eventapplication.CommitEvidenceSummaryCommand) (eventapplication.EvidenceSummaryDTO, error) {
	if repository == nil || repository.runtime == nil || command.MicroEventID <= 0 || command.EventVersion <= 0 ||
		strings.TrimSpace(command.SummaryProfileVersion) == "" || strings.TrimSpace(command.IdempotencyKey) == "" ||
		len(command.CommandFingerprint) != 64 || len(command.Sentences) == 0 || command.CreatedAt.IsZero() {
		return eventapplication.EvidenceSummaryDTO{}, eventapplication.ErrInvalidEvidenceSummaryContract
	}
	var result eventapplication.EvidenceSummaryDTO
	err := repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		record, found, err := readEvidenceSummaryRecord(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if record.commandFingerprint != command.CommandFingerprint {
				return sharedrepository.ErrConflict
			}
			result, err = readEvidenceSummaryDTO(transactionCtx, transaction.SQL, record, false)
			return err
		}
		var currentVersion int64
		var status string
		if err := transaction.SQL.QueryRowContext(transactionCtx, `SELECT version,status FROM micro_events WHERE id=$1 FOR KEY SHARE`, command.MicroEventID).Scan(&currentVersion, &status); err != nil {
			return databaserepository.MapError(err)
		}
		if currentVersion != command.EventVersion || status != "active" && status != "review_pending" {
			return sharedrepository.ErrConflict
		}
		for _, sentence := range command.Sentences {
			if sentence.EditorialNote {
				continue
			}
			var count int
			if err := transaction.SQL.QueryRowContext(transactionCtx, `SELECT count(DISTINCT evidence.id)
FROM claim_evidence_versions AS evidence
JOIN claims AS claim ON claim.id=evidence.claim_id AND claim.micro_event_id=$1
JOIN document_text_quote_selectors AS selector ON selector.id=evidence.text_quote_selector_id
JOIN content_families AS family ON family.id=evidence.content_family_id AND family.status IN ('active','review_pending')
JOIN micro_event_members AS member ON member.content_family_id=family.id AND member.micro_event_id=$1 AND member.active
WHERE evidence.id=ANY($2) AND selector.retention_until>$3
  AND NOT EXISTS (
      SELECT 1 FROM claim_evidence_feedbacks AS feedback
      WHERE feedback.original_claim_evidence_version_id=evidence.id
  )
  AND current_rights_action_allowed(selector.quote_rights_decision_id,selector.source_connection_id,
      'document_version',evidence.document_version_id::text,evidence.plaintext_sha256,'quote',$3)
  AND current_rights_action_allowed(selector.retain_rights_decision_id,selector.source_connection_id,
      'document_version',evidence.document_version_id::text,evidence.plaintext_sha256,'retain',$3)`,
				command.MicroEventID, sentence.ClaimEvidenceVersionIDs, command.CreatedAt.UTC()).Scan(&count); err != nil {
				return databaserepository.MapError(err)
			}
			if count != len(sentence.ClaimEvidenceVersionIDs) {
				return sharedrepository.ErrConstraint
			}
		}
		record = evidenceSummaryRecord{}
		if err := transaction.SQL.QueryRowContext(transactionCtx, `INSERT INTO micro_event_summaries
(micro_event_id,micro_event_version,summary_profile_version,idempotency_key,command_fingerprint,created_at)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING id,version,micro_event_id,micro_event_version,summary_profile_version,command_fingerprint,created_at`,
			command.MicroEventID, command.EventVersion, command.SummaryProfileVersion, command.IdempotencyKey,
			command.CommandFingerprint, command.CreatedAt.UTC()).Scan(&record.id, &record.version, &record.eventID,
			&record.eventVersion, &record.profile, &record.commandFingerprint, &record.createdAt); err != nil {
			return databaserepository.MapError(err)
		}
		for ordinal, sentence := range command.Sentences {
			var sentenceID int64
			if err := transaction.SQL.QueryRowContext(transactionCtx, `INSERT INTO micro_event_summary_sentences
(micro_event_summary_id,ordinal,sentence,editorial_note,decision_origin,model_run_id,actor_user_id,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, record.id, ordinal, sentence.Text, sentence.EditorialNote,
				sentence.DecisionOrigin, sentence.ModelRunID, sentence.ActorUserID, command.CreatedAt.UTC()).Scan(&sentenceID); err != nil {
				return databaserepository.MapError(err)
			}
			for evidenceOrdinal, evidenceID := range sentence.ClaimEvidenceVersionIDs {
				if _, err := transaction.SQL.ExecContext(transactionCtx, `INSERT INTO micro_event_summary_sentence_evidences
(summary_sentence_id,claim_evidence_version_id,ordinal,created_at) VALUES ($1,$2,$3,$4)`, sentenceID, evidenceID,
					evidenceOrdinal, command.CreatedAt.UTC()); err != nil {
					return databaserepository.MapError(err)
				}
			}
		}
		result, err = readEvidenceSummaryDTO(transactionCtx, transaction.SQL, record, true)
		return err
	})
	if err != nil {
		return eventapplication.EvidenceSummaryDTO{}, databaserepository.MapError(err)
	}
	return result, nil
}

func readEvidenceSummaryRecord(ctx context.Context, tx *sql.Tx, idempotencyKey string) (evidenceSummaryRecord, bool, error) {
	var record evidenceSummaryRecord
	err := tx.QueryRowContext(ctx, `SELECT id,version,micro_event_id,micro_event_version,summary_profile_version,command_fingerprint,created_at
FROM micro_event_summaries WHERE idempotency_key=$1 FOR KEY SHARE`, idempotencyKey).Scan(&record.id, &record.version,
		&record.eventID, &record.eventVersion, &record.profile, &record.commandFingerprint, &record.createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return evidenceSummaryRecord{}, false, nil
	}
	return record, err == nil, databaserepository.MapError(err)
}

func readEvidenceSummaryDTO(ctx context.Context, tx *sql.Tx, record evidenceSummaryRecord, created bool) (eventapplication.EvidenceSummaryDTO, error) {
	rows, err := tx.QueryContext(ctx, `SELECT sentence.id,sentence.version,sentence.ordinal,sentence.sentence,sentence.editorial_note,
sentence.decision_origin,sentence.model_run_id,sentence.actor_user_id,sentence.created_at,
COALESCE(json_agg(citation.claim_evidence_version_id ORDER BY citation.ordinal)
    FILTER (WHERE citation.claim_evidence_version_id IS NOT NULL),'[]'::json)
FROM micro_event_summary_sentences AS sentence
LEFT JOIN micro_event_summary_sentence_evidences AS citation ON citation.summary_sentence_id=sentence.id
WHERE sentence.micro_event_summary_id=$1
GROUP BY sentence.id ORDER BY sentence.ordinal`, record.id)
	if err != nil {
		return eventapplication.EvidenceSummaryDTO{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	sentences := []eventapplication.EvidenceSummarySentenceDTO{}
	for rows.Next() {
		var sentence eventapplication.EvidenceSummarySentenceDTO
		var modelRunID, actorUserID sql.NullInt64
		var evidenceJSON []byte
		if err := rows.Scan(&sentence.ID, &sentence.Version, &sentence.Ordinal, &sentence.Text, &sentence.EditorialNote,
			&sentence.DecisionOrigin, &modelRunID, &actorUserID, &sentence.CreatedAt, &evidenceJSON); err != nil {
			return eventapplication.EvidenceSummaryDTO{}, databaserepository.MapError(err)
		}
		if err := json.Unmarshal(evidenceJSON, &sentence.ClaimEvidenceVersionIDs); err != nil {
			return eventapplication.EvidenceSummaryDTO{}, fmt.Errorf("invalid summary citation projection: %w", err)
		}
		sentence.SummaryID, sentence.ModelRunID, sentence.ActorUserID = record.id, nullableClaimEvidenceInt64(modelRunID), nullableClaimEvidenceInt64(actorUserID)
		sentence.CreatedAt = sentence.CreatedAt.UTC()
		sentences = append(sentences, sentence)
	}
	if err := rows.Err(); err != nil {
		return eventapplication.EvidenceSummaryDTO{}, databaserepository.MapError(err)
	}
	return eventapplication.EvidenceSummaryDTO{ID: record.id, Version: record.version, MicroEventID: record.eventID,
		EventVersion: record.eventVersion, SummaryProfileVersion: record.profile, Sentences: sentences,
		CreatedAt: record.createdAt.UTC(), Created: created}, nil
}
