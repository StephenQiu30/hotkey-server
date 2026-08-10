package postgres

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
)

// ReadPublishableIntentProfile resolves only the exact current intent draft,
// its successful preview result, and the immutable ready preview profile. An
// existing v2 intent with any missing prerequisite fails closed; it is never
// represented as a legacy-compatible absence.
func (repository *IntentRepository) ReadPublishableIntentProfile(ctx context.Context, query monitorapplication.ReadPublishableIntentProfileQuery) (monitorapplication.PublishableIntentProfileDTO, error) {
	if repository == nil || query.MonitorID <= 0 || query.ConfigVersionID <= 0 {
		return monitorapplication.PublishableIntentProfileDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	executor := repository.intentExecutor(ctx)
	var draftID, resourceVersion int64
	err := executor.QueryRowContext(ctx, `
SELECT id,resource_version
FROM monitor_intent_drafts
WHERE monitor_id=$1 AND config_version_id=$2`, query.MonitorID, query.ConfigVersionID).Scan(&draftID, &resourceVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return monitorapplication.PublishableIntentProfileDTO{Exists: false}, nil
	}
	if err != nil {
		return monitorapplication.PublishableIntentProfileDTO{}, mapIntentDatabaseError(err)
	}

	var candidate monitorapplication.PublishableIntentProfileDTO
	var semanticReason sql.NullString
	err = executor.QueryRowContext(ctx, `
SELECT run.monitor_id,draft.config_version_id,run.id,draft.id,draft.resource_version,revision.id,profile.id,
       profile.compiler_version,profile.matching_algorithm_version,profile.lexical_algorithm_version,
       profile.semantic_algorithm_version,profile.structured_algorithm_version,
       profile.search_normalization_profile_version,profile.semantic_state,
       profile.semantic_unavailable_reason,btrim(profile.profile_hash)
FROM monitor_intent_drafts AS draft
JOIN monitor_intent_draft_revisions AS revision
  ON revision.draft_id=draft.id AND revision.monitor_id=draft.monitor_id
 AND revision.config_version_id=draft.config_version_id AND revision.resource_version=draft.resource_version
JOIN monitor_config_versions AS config
  ON config.id=draft.config_version_id AND config.monitor_id=draft.monitor_id
JOIN LATERAL (
    SELECT candidate.*
    FROM monitor_intent_analysis_runs AS candidate
    JOIN monitor_intent_preview_results AS result ON result.run_id=candidate.id
    WHERE candidate.kind='preview' AND candidate.status='succeeded'
      AND candidate.monitor_id=draft.monitor_id AND candidate.draft_id=draft.id
      AND candidate.draft_resource_version=draft.resource_version
    ORDER BY candidate.completed_at DESC,candidate.id DESC
    LIMIT 1
) AS run ON true
JOIN monitor_compiled_profiles AS profile
  ON profile.purpose='preview' AND profile.status='ready' AND profile.preview_run_id=run.id
 AND profile.monitor_id=draft.monitor_id AND profile.config_version_id=draft.config_version_id
 AND profile.draft_id=draft.id AND profile.draft_resource_version=draft.resource_version
 AND profile.intent_revision_id=revision.id
WHERE draft.monitor_id=$1 AND draft.config_version_id=$2 AND draft.id=$3
  AND draft.resource_version=$4 AND config.state='draft'`, query.MonitorID, query.ConfigVersionID, draftID, resourceVersion).Scan(
		&candidate.MonitorID, &candidate.ConfigVersionID, &candidate.PreviewRunID, &candidate.DraftID,
		&candidate.DraftResourceVersion, &candidate.IntentRevisionID, &candidate.PreviewCompiledProfileID,
		&candidate.CompilerVersion, &candidate.MatchingAlgorithmVersion, &candidate.LexicalAlgorithmVersion,
		&candidate.SemanticAlgorithmVersion, &candidate.StructuredAlgorithmVersion,
		&candidate.SearchNormalizationProfileVersion, &candidate.SemanticState, &semanticReason,
		&candidate.PreviewProfileHash,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return monitorapplication.PublishableIntentProfileDTO{}, monitorapplication.ErrIntentPublicationUnavailable
	}
	if err != nil {
		return monitorapplication.PublishableIntentProfileDTO{}, mapIntentDatabaseError(err)
	}
	candidate.Exists = true
	candidate.SemanticUnavailableReason = semanticReason.String
	candidate.PreviewProfileHash = strings.TrimSpace(candidate.PreviewProfileHash)
	candidate.Clauses, err = readCompiledIntentProfileClauses(ctx, executor, candidate.PreviewCompiledProfileID)
	if err != nil {
		return monitorapplication.PublishableIntentProfileDTO{}, err
	}
	candidate.Entities, err = readCompiledIntentProfileEntities(ctx, executor, candidate.PreviewCompiledProfileID)
	if err != nil {
		return monitorapplication.PublishableIntentProfileDTO{}, err
	}
	return candidate, nil
}

// StagePublishedIntentProfile copies the exact successful preview facts into a
// building published owner. It must run inside the Monitor publish transaction;
// a later validation or publish failure therefore removes the staged row and
// all child facts with the same rollback.
func (repository *IntentRepository) StagePublishedIntentProfile(ctx context.Context, command monitorapplication.StagePublishedIntentProfileDTO) (monitorapplication.StagePublishedIntentProfileReceiptDTO, error) {
	if repository == nil || !validPublishedIntentProfileStage(command) {
		return monitorapplication.StagePublishedIntentProfileReceiptDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	var receipt monitorapplication.StagePublishedIntentProfileReceiptDTO
	err := repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		candidate, readErr := lockPublishableIntentProfile(transactionCtx, executor, command)
		if readErr != nil {
			return readErr
		}
		if !samePublishedIntentSource(candidate, command) {
			return monitorapplication.ErrIntentPublicationUnavailable
		}
		clauses, readErr := readCompiledIntentProfileClauses(transactionCtx, executor, command.SourcePreviewCompiledProfileID)
		if readErr != nil {
			return readErr
		}
		entities, readErr := readCompiledIntentProfileEntities(transactionCtx, executor, command.SourcePreviewCompiledProfileID)
		if readErr != nil {
			return readErr
		}
		if !reflect.DeepEqual(clauses, command.Clauses) || !reflect.DeepEqual(entities, command.Entities) {
			return monitorapplication.ErrIntentPublicationUnavailable
		}

		existingID, existingErr := readBuildingPublishedIntentProfile(transactionCtx, executor, command)
		if existingErr == nil {
			existingClauses, factsErr := readCompiledIntentProfileClauses(transactionCtx, executor, existingID)
			if factsErr != nil {
				return factsErr
			}
			existingEntities, factsErr := readCompiledIntentProfileEntities(transactionCtx, executor, existingID)
			if factsErr != nil {
				return factsErr
			}
			if !reflect.DeepEqual(existingClauses, command.Clauses) || !reflect.DeepEqual(existingEntities, command.Entities) {
				return monitorapplication.ErrCompiledIntentProfileConflict
			}
			receipt = monitorapplication.StagePublishedIntentProfileReceiptDTO{CompiledProfileID: existingID, Reused: true}
			return nil
		}
		if !errors.Is(existingErr, sql.ErrNoRows) {
			return existingErr
		}

		var profileID int64
		insertErr := executor.QueryRowContext(transactionCtx, `
INSERT INTO monitor_compiled_profiles (
  monitor_id,purpose,config_version_id,monitor_version_id,source_preview_compiled_profile_id,intent_revision_id,
  compiler_version,matching_algorithm_version,lexical_algorithm_version,semantic_algorithm_version,
  structured_algorithm_version,search_normalization_profile_version,semantic_state,semantic_unavailable_reason
) VALUES ($1,'published',$2,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
RETURNING id`, command.MonitorID, command.ConfigVersionID, command.SourcePreviewCompiledProfileID,
			command.IntentRevisionID,
			command.CompilerVersion, command.MatchingAlgorithmVersion, command.LexicalAlgorithmVersion,
			command.SemanticAlgorithmVersion, command.StructuredAlgorithmVersion,
			command.SearchNormalizationProfileVersion, command.SemanticState,
			nullableIntentText(command.SemanticUnavailableReason)).Scan(&profileID)
		if insertErr != nil {
			return mapIntentDatabaseError(insertErr)
		}
		if insertErr = insertPublishedIntentProfileFacts(transactionCtx, executor, profileID, command); insertErr != nil {
			return insertErr
		}
		if command.SemanticState == monitorapplication.IntentSemanticStateReady {
			result, copyErr := executor.ExecContext(transactionCtx, `
INSERT INTO monitor_compiled_intent_embeddings (
  compiled_profile_id,config_version_id,model_profile_id,model_profile_version,
  model_version,input_hash,embedding,ai_run_id,created_at
)
SELECT $1,$2,model_profile_id,model_profile_version,model_version,input_hash,embedding,ai_run_id,created_at
FROM monitor_compiled_intent_embeddings WHERE compiled_profile_id=$3`,
				profileID, command.ConfigVersionID, command.SourcePreviewCompiledProfileID)
			if copyErr != nil {
				return mapIntentDatabaseError(copyErr)
			}
			if changed, rowsErr := result.RowsAffected(); rowsErr != nil || changed != 1 {
				return monitorapplication.ErrIntentPublicationUnavailable
			}
		}
		receipt = monitorapplication.StagePublishedIntentProfileReceiptDTO{CompiledProfileID: profileID}
		return nil
	})
	if err != nil {
		return monitorapplication.StagePublishedIntentProfileReceiptDTO{}, err
	}
	return receipt, nil
}

func (repository *IntentRepository) CompletePublishedIntentProfile(ctx context.Context, command monitorapplication.CompletePublishedIntentProfileDTO) error {
	if repository == nil || command.MonitorID <= 0 || command.ConfigVersionID <= 0 || command.CompiledProfileID <= 0 ||
		command.PreviousConfigVersionID < 0 || command.PreviousConfigVersionID == command.ConfigVersionID ||
		validateIntentRecordHash(command.ProfileHash) != nil || command.PublishedAt.IsZero() {
		return monitorapplication.ErrInvalidIntentContract
	}
	return repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		var status, state string
		err := executor.QueryRowContext(transactionCtx, `
SELECT profile.status,config.state
FROM monitor_compiled_profiles AS profile
JOIN monitor_config_versions AS config
  ON config.id=profile.config_version_id AND config.monitor_id=profile.monitor_id
WHERE profile.id=$1 AND profile.monitor_id=$2 AND profile.config_version_id=$3
  AND profile.monitor_version_id=$3 AND profile.purpose='published'
FOR UPDATE OF profile,config`, command.CompiledProfileID, command.MonitorID, command.ConfigVersionID).Scan(&status, &state)
		if errors.Is(err, sql.ErrNoRows) {
			return monitorapplication.ErrIntentPublicationUnavailable
		}
		if err != nil {
			return mapIntentDatabaseError(err)
		}
		if status != "building" || state != "published" {
			return monitorapplication.ErrIntentPublicationUnavailable
		}
		if command.PreviousConfigVersionID > 0 {
			var previousID int64
			var previousStatus string
			previousErr := executor.QueryRowContext(transactionCtx, `
SELECT id,status FROM monitor_compiled_profiles
WHERE monitor_id=$1 AND purpose='published' AND monitor_version_id=$2
FOR UPDATE`, command.MonitorID, command.PreviousConfigVersionID).Scan(&previousID, &previousStatus)
			if previousErr == nil {
				if previousStatus != "ready" {
					return monitorapplication.ErrCompiledIntentProfileConflict
				}
				result, updateErr := executor.ExecContext(transactionCtx, `
UPDATE monitor_compiled_profiles SET status='retired',retired_at=$2
WHERE id=$1 AND status='ready'`, previousID, command.PublishedAt.UTC())
				if updateErr != nil {
					return mapIntentDatabaseError(updateErr)
				}
				if changed, rowsErr := result.RowsAffected(); rowsErr != nil || changed != 1 {
					return monitorapplication.ErrCompiledIntentProfileConflict
				}
			} else if !errors.Is(previousErr, sql.ErrNoRows) {
				return mapIntentDatabaseError(previousErr)
			}
		}
		result, err := executor.ExecContext(transactionCtx, `
UPDATE monitor_compiled_profiles
SET status='ready',profile_hash=$2,ready_at=$3
WHERE id=$1 AND status='building'`, command.CompiledProfileID, command.ProfileHash, command.PublishedAt.UTC())
		if err != nil {
			return mapIntentDatabaseError(err)
		}
		if changed, rowsErr := result.RowsAffected(); rowsErr != nil || changed != 1 {
			return monitorapplication.ErrCompiledIntentProfileConflict
		}
		return nil
	})
}

func lockPublishableIntentProfile(ctx context.Context, executor intentExecutor, command monitorapplication.StagePublishedIntentProfileDTO) (compiledIntentProfileRecord, error) {
	var record compiledIntentProfileRecord
	var reason sql.NullString
	var hash sql.NullString
	var runStatus, configState string
	err := executor.QueryRowContext(ctx, `
SELECT profile.id,profile.monitor_id,profile.config_version_id,profile.preview_run_id,
       profile.draft_id,profile.draft_resource_version,profile.intent_revision_id,
       profile.compiler_version,profile.matching_algorithm_version,profile.lexical_algorithm_version,
       profile.semantic_algorithm_version,profile.structured_algorithm_version,
       profile.search_normalization_profile_version,profile.semantic_state,
       profile.semantic_unavailable_reason,profile.status,btrim(profile.profile_hash),run.status,config.state
FROM monitor_compiled_profiles AS profile
JOIN monitor_intent_analysis_runs AS run ON run.id=profile.preview_run_id
JOIN monitor_intent_preview_results AS result ON result.run_id=run.id
JOIN monitor_config_versions AS config
  ON config.id=profile.config_version_id AND config.monitor_id=profile.monitor_id
WHERE profile.id=$1 AND profile.purpose='preview' AND profile.preview_run_id=$2
  AND profile.monitor_id=$3 AND profile.config_version_id=$4 AND profile.intent_revision_id=$5
FOR UPDATE OF profile,run,config`, command.SourcePreviewCompiledProfileID, command.SourcePreviewRunID,
		command.MonitorID, command.ConfigVersionID, command.IntentRevisionID).Scan(
		&record.ID, &record.MonitorID, &record.ConfigVersionID, &record.PreviewRunID,
		&record.DraftID, &record.DraftResourceVersion, &record.IntentRevisionID,
		&record.CompilerVersion, &record.MatchingAlgorithmVersion, &record.LexicalAlgorithmVersion,
		&record.SemanticAlgorithmVersion, &record.StructuredAlgorithmVersion,
		&record.SearchNormalizationProfileVersion, &record.SemanticState, &reason, &record.Status,
		&hash, &runStatus, &configState,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return compiledIntentProfileRecord{}, monitorapplication.ErrIntentPublicationUnavailable
	}
	if err != nil {
		return compiledIntentProfileRecord{}, mapIntentDatabaseError(err)
	}
	record.SemanticUnavailableReason = reason.String
	record.ProfileHash = strings.TrimSpace(hash.String)
	if record.Status != "ready" || runStatus != "succeeded" || configState != "draft" {
		return compiledIntentProfileRecord{}, monitorapplication.ErrIntentPublicationUnavailable
	}
	return record, nil
}

func samePublishedIntentSource(record compiledIntentProfileRecord, command monitorapplication.StagePublishedIntentProfileDTO) bool {
	return record.ID == command.SourcePreviewCompiledProfileID && record.MonitorID == command.MonitorID &&
		record.ConfigVersionID == command.ConfigVersionID && record.PreviewRunID == command.SourcePreviewRunID &&
		record.IntentRevisionID == command.IntentRevisionID && record.CompilerVersion == command.CompilerVersion &&
		record.MatchingAlgorithmVersion == command.MatchingAlgorithmVersion &&
		record.LexicalAlgorithmVersion == command.LexicalAlgorithmVersion &&
		record.SemanticAlgorithmVersion == command.SemanticAlgorithmVersion &&
		record.StructuredAlgorithmVersion == command.StructuredAlgorithmVersion &&
		record.SearchNormalizationProfileVersion == command.SearchNormalizationProfileVersion &&
		record.SemanticState == command.SemanticState &&
		record.SemanticUnavailableReason == command.SemanticUnavailableReason
}

func readBuildingPublishedIntentProfile(ctx context.Context, executor intentExecutor, command monitorapplication.StagePublishedIntentProfileDTO) (int64, error) {
	var id, sourcePreviewID int64
	var reason sql.NullString
	var status, compiler, matching, lexical, semantic, structured, normalization, semanticState string
	err := executor.QueryRowContext(ctx, `
SELECT id,source_preview_compiled_profile_id,status,compiler_version,matching_algorithm_version,lexical_algorithm_version,
       semantic_algorithm_version,structured_algorithm_version,search_normalization_profile_version,
       semantic_state,semantic_unavailable_reason
FROM monitor_compiled_profiles
WHERE monitor_id=$1 AND purpose='published' AND monitor_version_id=$2
FOR UPDATE`, command.MonitorID, command.ConfigVersionID).Scan(&id, &sourcePreviewID, &status, &compiler, &matching, &lexical,
		&semantic, &structured, &normalization, &semanticState, &reason)
	if err != nil {
		return 0, err
	}
	if sourcePreviewID != command.SourcePreviewCompiledProfileID || status != "building" || compiler != command.CompilerVersion || matching != command.MatchingAlgorithmVersion ||
		lexical != command.LexicalAlgorithmVersion || semantic != command.SemanticAlgorithmVersion ||
		structured != command.StructuredAlgorithmVersion || normalization != command.SearchNormalizationProfileVersion ||
		semanticState != command.SemanticState || reason.String != command.SemanticUnavailableReason {
		return 0, monitorapplication.ErrCompiledIntentProfileConflict
	}
	return id, nil
}

func validPublishedIntentProfileStage(command monitorapplication.StagePublishedIntentProfileDTO) bool {
	if command.MonitorID <= 0 || command.ConfigVersionID <= 0 || command.IntentRevisionID <= 0 ||
		command.SourcePreviewRunID <= 0 || command.SourcePreviewCompiledProfileID <= 0 ||
		validateIntentRecordHash(command.ProfileHash) != nil ||
		!validCompletedCompiledIntentSemanticState(command.SemanticState, command.SemanticUnavailableReason) ||
		len(command.Clauses) > 128 || len(command.Entities) > 64 {
		return false
	}
	for _, version := range []string{command.CompilerVersion, command.MatchingAlgorithmVersion, command.LexicalAlgorithmVersion,
		command.SemanticAlgorithmVersion, command.StructuredAlgorithmVersion, command.SearchNormalizationProfileVersion} {
		if !intentAnalysisProfilePattern.MatchString(version) {
			return false
		}
	}
	return true
}

func insertPublishedIntentProfileFacts(ctx context.Context, executor intentExecutor, profileID int64, command monitorapplication.StagePublishedIntentProfileDTO) error {
	previewCommand := monitorapplication.PersistPreviewCompiledProfileDTO{Clauses: command.Clauses, Entities: command.Entities}
	return insertCompiledIntentProfileFacts(ctx, executor, profileID, previewCommand)
}

func nullableIntentText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
