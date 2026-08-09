package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
)

func (repository *IntentRepository) Find(ctx context.Context, query monitorapplication.ReadIntentDraftQuery) (monitorapplication.IntentDraftDTO, error) {
	if repository == nil || repository.runtime == nil || query.MonitorID <= 0 || query.DraftID <= 0 {
		return monitorapplication.IntentDraftDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	draft, err := readIntentDraftAt(ctx, repository.intentExecutor(ctx), query.MonitorID, query.DraftID, 0)
	if errors.Is(err, sql.ErrNoRows) {
		return monitorapplication.IntentDraftDTO{}, monitorapplication.ErrIntentDraftNotFound
	}
	return draft, mapIntentDatabaseError(err)
}

func (repository *IntentRepository) FindIntentDraftRevision(ctx context.Context, query monitorapplication.ReadIntentDraftRevisionQuery) (monitorapplication.IntentDraftDTO, error) {
	if repository == nil || repository.runtime == nil || query.MonitorID <= 0 || query.DraftID <= 0 || query.ResourceVersion <= 0 {
		return monitorapplication.IntentDraftDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	draft, err := readIntentDraftAt(ctx, repository.intentExecutor(ctx), query.MonitorID, query.DraftID, query.ResourceVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return monitorapplication.IntentDraftDTO{}, monitorapplication.ErrIntentDraftNotFound
	}
	return draft, mapIntentDatabaseError(err)
}

func (repository *IntentRepository) FindMutation(ctx context.Context, lookup monitorapplication.IntentDraftMutationLookupDTO) (monitorapplication.IntentDraftMutationReceiptDTO, error) {
	if repository == nil || repository.runtime == nil || lookup.MonitorID <= 0 || lookup.DraftID <= 0 || lookup.IdempotencyKey == "" {
		return monitorapplication.IntentDraftMutationReceiptDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	record, err := readIntentMutationReceipt(ctx, repository.intentExecutor(ctx), lookup, false)
	if errors.Is(err, sql.ErrNoRows) {
		return monitorapplication.IntentDraftMutationReceiptDTO{}, monitorapplication.ErrIntentMutationNotFound
	}
	if err != nil {
		return monitorapplication.IntentDraftMutationReceiptDTO{}, mapIntentDatabaseError(err)
	}
	draft, err := readIntentDraftAt(ctx, repository.intentExecutor(ctx), record.MonitorID, record.DraftID, record.ResultVersion)
	if err != nil {
		return monitorapplication.IntentDraftMutationReceiptDTO{}, mapIntentDatabaseError(err)
	}
	return monitorapplication.IntentDraftMutationReceiptDTO{Draft: draft, CommandFingerprint: record.Fingerprint, Created: false}, nil
}

func (repository *IntentRepository) SaveAndInvalidateRuns(ctx context.Context, mutation monitorapplication.IntentDraftMutationDTO) (monitorapplication.IntentDraftMutationReceiptDTO, error) {
	if !validIntentDraftMutation(mutation) {
		return monitorapplication.IntentDraftMutationReceiptDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	var receipt monitorapplication.IntentDraftMutationReceiptDTO
	err := repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		current, err := lockIntentDraft(transactionCtx, executor, mutation.Next.MonitorID, mutation.ExpectedDraftID)
		if errors.Is(err, sql.ErrNoRows) {
			return monitorapplication.ErrIntentDraftNotFound
		}
		if err != nil {
			return err
		}

		if mutation.Kind == monitorapplication.IntentDraftMutationCandidateReview {
			lookup := monitorapplication.IntentDraftMutationLookupDTO{
				MonitorID: mutation.Next.MonitorID, DraftID: mutation.ExpectedDraftID, IdempotencyKey: mutation.IdempotencyKey,
			}
			prior, receiptErr := readIntentMutationReceipt(transactionCtx, executor, lookup, true)
			if receiptErr == nil {
				if prior.Fingerprint != mutation.CommandFingerprint {
					return monitorapplication.ErrIntentIdempotencyConflict
				}
				priorDraft, readErr := readIntentDraftAt(transactionCtx, executor, prior.MonitorID, prior.DraftID, prior.ResultVersion)
				if readErr != nil {
					return readErr
				}
				receipt = monitorapplication.IntentDraftMutationReceiptDTO{Draft: priorDraft, CommandFingerprint: prior.Fingerprint, Created: false}
				return nil
			}
			if !errors.Is(receiptErr, sql.ErrNoRows) {
				return receiptErr
			}
		}

		if current.ResourceVersion != mutation.ExpectedResourceVersion {
			return monitorapplication.ErrIntentVersionConflict
		}
		currentDraft, err := readIntentDraftAt(transactionCtx, executor, mutation.Next.MonitorID, mutation.ExpectedDraftID, current.ResourceVersion)
		if err != nil {
			return err
		}
		if mutation.Kind == monitorapplication.IntentDraftMutationReplace && len(mutation.Next.Candidates) != 0 {
			return monitorapplication.ErrInvalidIntentContract
		}
		if mutation.Kind == monitorapplication.IntentDraftMutationCandidateReview && !validIntentCandidateReviewMutation(currentDraft, mutation.Next) {
			return monitorapplication.ErrInvalidIntentContract
		}
		if _, err := insertIntentDraftRevision(transactionCtx, executor, current.ConfigVersionID, mutation.Next); err != nil {
			return err
		}
		result, err := executor.ExecContext(transactionCtx, `
UPDATE monitor_intent_drafts
SET resource_version=$3,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND monitor_id=$2 AND resource_version=$4`,
			mutation.ExpectedDraftID, mutation.Next.MonitorID, mutation.Next.ResourceVersion, mutation.ExpectedResourceVersion)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return monitorapplication.ErrIntentVersionConflict
		}
		if err := invalidateSupersededIntentRuns(transactionCtx, executor, mutation.Next.MonitorID, mutation.ExpectedDraftID, mutation.Next.ResourceVersion, mutation.InvalidatedAt, 0); err != nil {
			return err
		}
		if mutation.Kind == monitorapplication.IntentDraftMutationCandidateReview {
			_, err = executor.ExecContext(transactionCtx, `
INSERT INTO monitor_intent_mutation_receipts (
  monitor_id,draft_id,mutation_kind,idempotency_key,command_fingerprint,
  expected_resource_version,result_resource_version
) VALUES ($1,$2,$3,$4,$5,$6,$7)`, mutation.Next.MonitorID, mutation.ExpectedDraftID,
				string(mutation.Kind), mutation.IdempotencyKey, mutation.CommandFingerprint,
				mutation.ExpectedResourceVersion, mutation.Next.ResourceVersion)
			if err != nil {
				return err
			}
		}
		stored, err := readIntentDraftAt(transactionCtx, executor, mutation.Next.MonitorID, mutation.ExpectedDraftID, mutation.Next.ResourceVersion)
		if err != nil {
			return err
		}
		receipt = monitorapplication.IntentDraftMutationReceiptDTO{
			Draft: stored, CommandFingerprint: mutation.CommandFingerprint, Created: true,
		}
		return nil
	})
	if err != nil {
		return monitorapplication.IntentDraftMutationReceiptDTO{}, mapIntentDatabaseError(err)
	}
	return receipt, nil
}

func validIntentDraftMutation(mutation monitorapplication.IntentDraftMutationDTO) bool {
	if mutation.ExpectedDraftID <= 0 || mutation.ExpectedResourceVersion <= 0 || mutation.Next.MonitorID <= 0 ||
		mutation.Next.DraftID != mutation.ExpectedDraftID || mutation.Next.ResourceVersion != mutation.ExpectedResourceVersion+1 ||
		mutation.Next.Objective == "" || mutation.InvalidatedAt.IsZero() {
		return false
	}
	switch mutation.Kind {
	case monitorapplication.IntentDraftMutationReplace:
		return mutation.IdempotencyKey == "" && mutation.CommandFingerprint == ""
	case monitorapplication.IntentDraftMutationCandidateReview:
		return mutation.IdempotencyKey != "" && len([]byte(mutation.IdempotencyKey)) <= 128 && validateIntentRecordHash(mutation.CommandFingerprint) == nil
	default:
		return false
	}
}

func lockIntentDraft(ctx context.Context, executor intentExecutor, monitorID, draftID int64) (intentDraftRecord, error) {
	var record intentDraftRecord
	err := executor.QueryRowContext(ctx, `
SELECT d.id,d.resource_version,d.monitor_id,d.config_version_id,d.created_at,d.updated_at
FROM monitors m
JOIN monitor_config_versions c
  ON c.id=m.draft_config_version_id AND c.monitor_id=m.id AND c.state='draft'
JOIN monitor_intent_drafts d
  ON d.id=$1 AND d.monitor_id=m.id AND d.config_version_id=c.id
WHERE m.id=$2 AND m.deleted_at IS NULL
FOR UPDATE OF m,c,d`, draftID, monitorID).Scan(
		&record.ID, &record.ResourceVersion, &record.MonitorID, &record.ConfigVersionID, &record.CreatedAt, &record.UpdatedAt,
	)
	return record, err
}

func readIntentMutationReceipt(ctx context.Context, executor intentExecutor, lookup monitorapplication.IntentDraftMutationLookupDTO, lock bool) (intentMutationReceiptRecord, error) {
	query := `
SELECT id,monitor_id,draft_id,mutation_kind,idempotency_key,command_fingerprint,
       expected_resource_version,result_resource_version,created_at
FROM monitor_intent_mutation_receipts
WHERE monitor_id=$1 AND draft_id=$2 AND idempotency_key=$3`
	if lock {
		query += ` FOR UPDATE`
	}
	var record intentMutationReceiptRecord
	err := executor.QueryRowContext(ctx, query, lookup.MonitorID, lookup.DraftID, lookup.IdempotencyKey).Scan(
		&record.ID, &record.MonitorID, &record.DraftID, &record.Kind, &record.IdempotencyKey, &record.Fingerprint,
		&record.ExpectedVersion, &record.ResultVersion, &record.CreatedAt,
	)
	return record, err
}

func readIntentDraftAt(ctx context.Context, executor intentExecutor, monitorID, draftID, resourceVersion int64) (monitorapplication.IntentDraftDTO, error) {
	var revision intentDraftRevisionRecord
	query := `
SELECT r.id,r.draft_id,r.monitor_id,r.config_version_id,r.resource_version,r.objective,r.created_at
FROM monitor_intent_draft_revisions r
JOIN monitor_intent_drafts d ON d.id=r.draft_id AND d.monitor_id=r.monitor_id
WHERE r.draft_id=$1 AND r.monitor_id=$2`
	arguments := []any{draftID, monitorID}
	if resourceVersion > 0 {
		query += ` AND r.resource_version=$3`
		arguments = append(arguments, resourceVersion)
	} else {
		query += ` AND r.resource_version=d.resource_version`
	}
	err := executor.QueryRowContext(ctx, query, arguments...).Scan(
		&revision.ID, &revision.DraftID, &revision.MonitorID, &revision.ConfigVersionID,
		&revision.ResourceVersion, &revision.Objective, &revision.CreatedAt,
	)
	if err != nil {
		return monitorapplication.IntentDraftDTO{}, err
	}
	draft := monitorapplication.IntentDraftDTO{
		MonitorID: revision.MonitorID, DraftID: revision.DraftID,
		ResourceVersion: revision.ResourceVersion, Objective: revision.Objective,
	}
	if draft.Clauses, err = readIntentClauses(ctx, executor, revision); err != nil {
		return monitorapplication.IntentDraftDTO{}, err
	}
	if draft.Entities, err = readIntentEntities(ctx, executor, revision); err != nil {
		return monitorapplication.IntentDraftDTO{}, err
	}
	if draft.Examples, err = readIntentExamples(ctx, executor, revision); err != nil {
		return monitorapplication.IntentDraftDTO{}, err
	}
	if draft.Candidates, err = readIntentDraftCandidates(ctx, executor, revision); err != nil {
		return monitorapplication.IntentDraftDTO{}, err
	}
	return draft, nil
}

func readIntentClauses(ctx context.Context, executor intentExecutor, revision intentDraftRevisionRecord) ([]monitorapplication.IntentClauseDTO, error) {
	rows, err := executor.QueryContext(ctx, `
SELECT operator,field,value FROM monitor_intent_clauses
WHERE revision_id=$1 ORDER BY ordinal`, revision.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]monitorapplication.IntentClauseDTO, 0)
	for rows.Next() {
		var item monitorapplication.IntentClauseDTO
		if err := rows.Scan(&item.Operator, &item.Field, &item.Value); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func readIntentEntities(ctx context.Context, executor intentExecutor, revision intentDraftRevisionRecord) ([]monitorapplication.IntentEntityDTO, error) {
	rows, err := executor.QueryContext(ctx, `
SELECT id,canonical_id,display_name,ambiguity_note FROM monitor_intent_entities
WHERE revision_id=$1 ORDER BY ordinal`, revision.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type entityRecord struct {
		ID  int64
		DTO monitorapplication.IntentEntityDTO
	}
	records := make([]entityRecord, 0)
	for rows.Next() {
		var record entityRecord
		if err := rows.Scan(&record.ID, &record.DTO.CanonicalID, &record.DTO.DisplayName, &record.DTO.AmbiguityNote); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]monitorapplication.IntentEntityDTO, 0, len(records))
	for _, record := range records {
		aliasRows, err := executor.QueryContext(ctx, `SELECT alias FROM monitor_intent_entity_aliases WHERE entity_id=$1 ORDER BY ordinal`, record.ID)
		if err != nil {
			return nil, err
		}
		for aliasRows.Next() {
			var alias string
			if err := aliasRows.Scan(&alias); err != nil {
				aliasRows.Close()
				return nil, err
			}
			record.DTO.Aliases = append(record.DTO.Aliases, alias)
		}
		if err := aliasRows.Close(); err != nil {
			return nil, err
		}
		items = append(items, record.DTO)
	}
	return items, nil
}

func readIntentExamples(ctx context.Context, executor intentExecutor, revision intentDraftRevisionRecord) ([]monitorapplication.IntentExampleDTO, error) {
	rows, err := executor.QueryContext(ctx, `
SELECT label,example_text FROM monitor_intent_examples WHERE revision_id=$1 ORDER BY ordinal`, revision.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]monitorapplication.IntentExampleDTO, 0)
	for rows.Next() {
		var item monitorapplication.IntentExampleDTO
		if err := rows.Scan(&item.Label, &item.Text); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func readIntentDraftCandidates(ctx context.Context, executor intentExecutor, revision intentDraftRevisionRecord) ([]monitorapplication.ExpansionCandidateDTO, error) {
	rows, err := executor.QueryContext(ctx, `
SELECT c.id,c.draft_id,c.introduced_resource_version,c.candidate_id,c.candidate_value,c.source,c.reason,
       c.model_version,c.prompt_version,c.input_hash,c.similarity,c.risk,
       dc.approval_status,dc.reviewer_user_id,dc.reviewed_at,dc.review_note
FROM monitor_intent_draft_candidates dc
JOIN monitor_intent_expansion_candidates c ON c.id=dc.candidate_record_id
WHERE dc.revision_id=$1 ORDER BY dc.ordinal`, revision.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]monitorapplication.ExpansionCandidateDTO, 0)
	for rows.Next() {
		var record intentDraftCandidateRecord
		if err := rows.Scan(
			&record.Candidate.ID, &record.Candidate.DraftID, &record.Candidate.IntroducedResourceVersion,
			&record.Candidate.CandidateID, &record.Candidate.Value, &record.Candidate.Source, &record.Candidate.Reason,
			&record.Candidate.ModelVersion, &record.Candidate.PromptVersion, &record.Candidate.InputHash,
			&record.Candidate.Similarity, &record.Candidate.Risk, &record.Status, &record.Reviewer, &record.Reviewed, &record.Note,
		); err != nil {
			return nil, err
		}
		items = append(items, record.applicationDTO())
	}
	return items, rows.Err()
}

func insertIntentDraftRevision(ctx context.Context, executor intentExecutor, configVersionID int64, draft monitorapplication.IntentDraftDTO) (int64, error) {
	var revisionID int64
	err := executor.QueryRowContext(ctx, `
INSERT INTO monitor_intent_draft_revisions (draft_id,monitor_id,config_version_id,resource_version,objective)
VALUES ($1,$2,$3,$4,$5) RETURNING id`, draft.DraftID, draft.MonitorID, configVersionID, draft.ResourceVersion, draft.Objective).Scan(&revisionID)
	if err != nil {
		return 0, err
	}
	for ordinal, clause := range draft.Clauses {
		if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_intent_clauses (revision_id,draft_id,resource_version,ordinal,operator,field,value)
VALUES ($1,$2,$3,$4,$5,$6,$7)`, revisionID, draft.DraftID, draft.ResourceVersion, ordinal, clause.Operator, clause.Field, clause.Value); err != nil {
			return 0, err
		}
	}
	for ordinal, entity := range draft.Entities {
		var entityID int64
		if err := executor.QueryRowContext(ctx, `
INSERT INTO monitor_intent_entities (revision_id,draft_id,resource_version,ordinal,canonical_id,display_name,ambiguity_note)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, revisionID, draft.DraftID, draft.ResourceVersion, ordinal,
			entity.CanonicalID, entity.DisplayName, entity.AmbiguityNote).Scan(&entityID); err != nil {
			return 0, err
		}
		for aliasOrdinal, alias := range entity.Aliases {
			if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_intent_entity_aliases (entity_id,draft_id,resource_version,ordinal,alias)
VALUES ($1,$2,$3,$4,$5)`, entityID, draft.DraftID, draft.ResourceVersion, aliasOrdinal, alias); err != nil {
				return 0, err
			}
		}
	}
	for ordinal, example := range draft.Examples {
		if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_intent_examples (revision_id,draft_id,resource_version,ordinal,label,example_text)
VALUES ($1,$2,$3,$4,$5,$6)`, revisionID, draft.DraftID, draft.ResourceVersion, ordinal, example.Label, example.Text); err != nil {
			return 0, err
		}
	}
	for ordinal, candidate := range draft.Candidates {
		record, err := findIntentCandidateRecord(ctx, executor, draft.DraftID, candidate.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, monitorapplication.ErrExpansionCandidateNotFound
		}
		if err != nil {
			return 0, err
		}
		if !intentCandidateRecordMatches(record, candidate) {
			return 0, monitorapplication.ErrInvalidIntentContract
		}
		var reviewer any
		if candidate.ReviewerUserID != nil {
			reviewer = *candidate.ReviewerUserID
		}
		var reviewed any
		if candidate.ReviewedAt != nil {
			reviewed = candidate.ReviewedAt.UTC()
		}
		if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_intent_draft_candidates (
  revision_id,draft_id,resource_version,candidate_record_id,ordinal,approval_status,reviewer_user_id,reviewed_at,review_note
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, revisionID, draft.DraftID, draft.ResourceVersion, record.ID, ordinal,
			candidate.ApprovalStatus, reviewer, reviewed, candidate.ReviewNote); err != nil {
			return 0, err
		}
	}
	return revisionID, nil
}

func findIntentCandidateRecord(ctx context.Context, executor intentExecutor, draftID int64, candidateID string) (intentExpansionCandidateRecord, error) {
	var record intentExpansionCandidateRecord
	err := executor.QueryRowContext(ctx, `
SELECT id,draft_id,introduced_resource_version,candidate_id,candidate_value,source,reason,
       model_version,prompt_version,input_hash,similarity,risk
FROM monitor_intent_expansion_candidates
WHERE draft_id=$1 AND candidate_id=$2
ORDER BY introduced_resource_version DESC,id DESC LIMIT 1`, draftID, candidateID).Scan(
		&record.ID, &record.DraftID, &record.IntroducedResourceVersion, &record.CandidateID, &record.Value,
		&record.Source, &record.Reason, &record.ModelVersion, &record.PromptVersion, &record.InputHash,
		&record.Similarity, &record.Risk,
	)
	return record, err
}

func intentCandidateRecordMatches(record intentExpansionCandidateRecord, candidate monitorapplication.ExpansionCandidateDTO) bool {
	return record.CandidateID == candidate.ID && record.Value == candidate.Value && record.Source == candidate.Source &&
		record.Reason == candidate.Reason && record.ModelVersion == candidate.ModelVersion && record.PromptVersion == candidate.PromptVersion &&
		record.InputHash == candidate.InputHash && record.Similarity == candidate.Similarity && record.Risk == candidate.Risk
}

func invalidateSupersededIntentRuns(ctx context.Context, executor intentExecutor, monitorID, draftID, nextVersion int64, invalidatedAt time.Time, excludedRunID int64) error {
	_, err := executor.ExecContext(ctx, `
UPDATE monitor_intent_analysis_runs
SET status='invalidated',invalidated_at=$4,updated_at=CURRENT_TIMESTAMP
WHERE monitor_id=$1 AND draft_id=$2 AND draft_resource_version < $3
  AND status <> 'invalidated' AND ($5=0 OR id <> $5)`, monitorID, draftID, nextVersion, invalidatedAt.UTC(), excludedRunID)
	return err
}

func insertGeneratedIntentCandidates(ctx context.Context, executor intentExecutor, runID int64, draft monitorapplication.IntentDraftDTO, generated []monitorapplication.ExpansionCandidateDTO) error {
	for _, candidate := range generated {
		if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_intent_expansion_candidates (
  draft_id,introduced_resource_version,candidate_id,origin_run_id,candidate_value,source,reason,
  model_version,prompt_version,input_hash,similarity,risk
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, draft.DraftID, draft.ResourceVersion, candidate.ID, runID,
			candidate.Value, candidate.Source, candidate.Reason, candidate.ModelVersion, candidate.PromptVersion,
			candidate.InputHash, candidate.Similarity, candidate.Risk); err != nil {
			return err
		}
	}
	return nil
}

func insertIntentDraftRevisionWithoutCandidates(ctx context.Context, executor intentExecutor, configVersionID int64, draft monitorapplication.IntentDraftDTO) (int64, error) {
	copy := draft
	copy.Candidates = nil
	return insertIntentDraftRevision(ctx, executor, configVersionID, copy)
}

func insertIntentDraftCandidateLinks(ctx context.Context, executor intentExecutor, revisionID int64, draft monitorapplication.IntentDraftDTO) error {
	for ordinal, candidate := range draft.Candidates {
		record, err := findIntentCandidateRecord(ctx, executor, draft.DraftID, candidate.ID)
		if err != nil {
			return err
		}
		if !intentCandidateRecordMatches(record, candidate) {
			return fmt.Errorf("%w: expansion candidate facts changed", monitorapplication.ErrInvalidIntentContract)
		}
		var reviewer, reviewed any
		if candidate.ReviewerUserID != nil {
			reviewer = *candidate.ReviewerUserID
		}
		if candidate.ReviewedAt != nil {
			reviewed = candidate.ReviewedAt.UTC()
		}
		if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_intent_draft_candidates (
  revision_id,draft_id,resource_version,candidate_record_id,ordinal,approval_status,reviewer_user_id,reviewed_at,review_note
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, revisionID, draft.DraftID, draft.ResourceVersion, record.ID, ordinal,
			candidate.ApprovalStatus, reviewer, reviewed, candidate.ReviewNote); err != nil {
			return err
		}
	}
	return nil
}
