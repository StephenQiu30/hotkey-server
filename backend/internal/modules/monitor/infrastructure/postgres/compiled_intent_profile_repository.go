package postgres

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
)

func (repository *IntentRepository) PersistPreviewCompiledProfile(ctx context.Context, command monitorapplication.PersistPreviewCompiledProfileDTO) (monitorapplication.PersistPreviewCompiledProfileReceiptDTO, error) {
	if repository == nil || !validCompiledIntentProfileCommand(command) {
		return monitorapplication.PersistPreviewCompiledProfileReceiptDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	var receipt monitorapplication.PersistPreviewCompiledProfileReceiptDTO
	err := repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		owner, ownerErr := lockCompiledIntentPreviewOwner(transactionCtx, executor, command)
		if ownerErr != nil {
			return ownerErr
		}
		existing, existingErr := readCompiledIntentProfileByPreviewRun(transactionCtx, executor, command.Task.Run.RunID)
		if existingErr == nil {
			if !sameCompiledIntentProfile(existing, owner, command) {
				return monitorapplication.ErrCompiledIntentProfileConflict
			}
			clauses, readErr := readCompiledIntentProfileClauses(transactionCtx, executor, existing.ID)
			if readErr != nil {
				return readErr
			}
			entities, readErr := readCompiledIntentProfileEntities(transactionCtx, executor, existing.ID)
			if readErr != nil {
				return readErr
			}
			if !reflect.DeepEqual(clauses, command.Clauses) || !reflect.DeepEqual(entities, command.Entities) {
				return monitorapplication.ErrCompiledIntentProfileConflict
			}
			receipt = monitorapplication.PersistPreviewCompiledProfileReceiptDTO{
				CompiledProfileID: existing.ID, ConfigVersionID: owner.ConfigVersionID,
				IntentRevisionID: owner.IntentRevisionID, Status: existing.Status,
				SemanticState: existing.SemanticState, SemanticUnavailableReason: existing.SemanticUnavailableReason,
				Reused: true,
			}
			return nil
		}
		if !errors.Is(existingErr, sql.ErrNoRows) {
			return existingErr
		}
		profileID, insertErr := insertCompiledIntentProfile(transactionCtx, executor, owner, command)
		if insertErr != nil {
			return insertErr
		}
		if insertErr = insertCompiledIntentProfileFacts(transactionCtx, executor, profileID, command); insertErr != nil {
			return insertErr
		}
		receipt = monitorapplication.PersistPreviewCompiledProfileReceiptDTO{
			CompiledProfileID: profileID, ConfigVersionID: owner.ConfigVersionID,
			IntentRevisionID: owner.IntentRevisionID, Status: "building",
			SemanticState: command.SemanticState, SemanticUnavailableReason: command.SemanticUnavailableReason,
		}
		return nil
	})
	if err != nil {
		return monitorapplication.PersistPreviewCompiledProfileReceiptDTO{}, err
	}
	return receipt, nil
}

func (repository *IntentRepository) CompletePreviewCompiledProfile(ctx context.Context, command monitorapplication.CompletePreviewCompiledProfileDTO) (monitorapplication.CompletePreviewCompiledProfileReceiptDTO, error) {
	if repository == nil || command.CompiledProfileID <= 0 || command.ConfigVersionID <= 0 || command.IntentRevisionID <= 0 ||
		validateIntentRecordHash(command.ProfileHash) != nil || command.ReadyAt.IsZero() ||
		!validCompletedCompiledIntentSemanticState(command.SemanticState, command.SemanticUnavailableReason) {
		return monitorapplication.CompletePreviewCompiledProfileReceiptDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	var receipt monitorapplication.CompletePreviewCompiledProfileReceiptDTO
	err := repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		var status, semanticState string
		var semanticReason, profileHash sql.NullString
		err := executor.QueryRowContext(transactionCtx, `
SELECT status,semantic_state,semantic_unavailable_reason,btrim(profile_hash)
FROM monitor_compiled_profiles
WHERE id=$1 AND config_version_id=$2 AND intent_revision_id=$3 AND purpose='preview'
FOR UPDATE`, command.CompiledProfileID, command.ConfigVersionID, command.IntentRevisionID).Scan(
			&status, &semanticState, &semanticReason, &profileHash,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return monitorapplication.ErrIntentPublicationUnavailable
		}
		if err != nil {
			return mapIntentDatabaseError(err)
		}
		if status == "ready" {
			if strings.TrimSpace(profileHash.String) != command.ProfileHash || semanticState != command.SemanticState ||
				semanticReason.String != command.SemanticUnavailableReason {
				return monitorapplication.ErrCompiledIntentProfileConflict
			}
			receipt = monitorapplication.CompletePreviewCompiledProfileReceiptDTO{
				CompiledProfileID: command.CompiledProfileID, Status: status, SemanticState: semanticState,
				SemanticUnavailableReason: semanticReason.String, Reused: true,
			}
			return nil
		}
		if status != "building" {
			return monitorapplication.ErrCompiledIntentProfileConflict
		}
		result, updateErr := executor.ExecContext(transactionCtx, `
UPDATE monitor_compiled_profiles
SET status='ready',profile_hash=$2,ready_at=$3,semantic_state=$4,semantic_unavailable_reason=$5
WHERE id=$1 AND status='building'`, command.CompiledProfileID, command.ProfileHash, command.ReadyAt.UTC(),
			command.SemanticState, nullableIntentText(command.SemanticUnavailableReason))
		if updateErr != nil {
			return mapIntentDatabaseError(updateErr)
		}
		if changed, rowsErr := result.RowsAffected(); rowsErr != nil || changed != 1 {
			return monitorapplication.ErrCompiledIntentProfileConflict
		}
		receipt = monitorapplication.CompletePreviewCompiledProfileReceiptDTO{
			CompiledProfileID: command.CompiledProfileID, Status: "ready", SemanticState: command.SemanticState,
			SemanticUnavailableReason: command.SemanticUnavailableReason,
		}
		return nil
	})
	return receipt, err
}

func validCompletedCompiledIntentSemanticState(state, reason string) bool {
	return state == monitorapplication.IntentSemanticStateReady && reason == "" ||
		state == monitorapplication.IntentSemanticStateUnavailable &&
			(reason == monitorapplication.IntentSemanticModelUnavailable || reason == monitorapplication.IntentSemanticGenerationUnavailable)
}

func validCompiledIntentProfileCommand(command monitorapplication.PersistPreviewCompiledProfileDTO) bool {
	task := command.Task
	if task.Run.RunID <= 0 || task.Run.Kind != "preview" || task.Run.MonitorID <= 0 || task.Run.DraftID <= 0 ||
		task.Run.DraftResourceVersion <= 0 || task.SampleLimit < 1 || task.SampleLimit > 200 ||
		validateIntentRecordHash(task.Run.InputHash) != nil || validateIntentRecordHash(command.ProfileHash) != nil || command.ReadyAt.IsZero() ||
		command.SemanticState != "unavailable" || command.SemanticUnavailableReason != monitorapplication.IntentSemanticGenerationUnavailable {
		return false
	}
	for _, version := range []string{command.CompilerVersion, command.MatchingAlgorithmVersion, command.LexicalAlgorithmVersion,
		command.SemanticAlgorithmVersion, command.StructuredAlgorithmVersion, command.SearchNormalizationProfileVersion, task.AnalysisProfile} {
		if !intentAnalysisProfilePattern.MatchString(version) {
			return false
		}
	}
	if len(command.Clauses) > 128 || len(command.Entities) > 64 {
		return false
	}
	for _, clause := range command.Clauses {
		if clause.Value == "" || clause.NormalizedValue == "" || clause.Value != strings.TrimSpace(clause.Value) ||
			len([]byte(clause.Value)) > 2048 || len([]byte(clause.NormalizedValue)) > 2048 ||
			!validCompiledIntentClauseEnum(clause.Operator, clause.Field, clause.Origin) {
			return false
		}
	}
	for _, entity := range command.Entities {
		if entity.CanonicalID == "" || len([]byte(entity.CanonicalID)) > 256 || len(entity.Aliases) > 32 || len(entity.NormalizedAliases) != len(entity.Aliases) {
			return false
		}
		for index, alias := range entity.Aliases {
			if alias == "" || len([]byte(alias)) > 640 || entity.NormalizedAliases[index] == "" || len([]byte(entity.NormalizedAliases[index])) > 640 {
				return false
			}
		}
	}
	return true
}

func validCompiledIntentClauseEnum(operator, field, origin string) bool {
	validOperator := operator == "must" || operator == "should" || operator == "must_not"
	validField := field == "term" || field == "phrase" || field == "action" || field == "location" || field == "language" || field == "region" || field == "source" || field == "time_window"
	validOrigin := origin == "intent_clause" || origin == "objective_derived" || origin == "approved_candidate"
	return validOperator && validField && validOrigin
}

func lockCompiledIntentPreviewOwner(ctx context.Context, executor intentExecutor, command monitorapplication.PersistPreviewCompiledProfileDTO) (compiledIntentProfileRecord, error) {
	var owner compiledIntentProfileRecord
	var kind, status, inputHash, profileVersion, configState string
	var sampleLimit int
	err := executor.QueryRowContext(ctx, `
SELECT run.monitor_id,draft.config_version_id,run.id,run.draft_id,run.draft_resource_version,revision.id,
       run.kind,run.status,btrim(run.input_hash),run.profile_version,run.sample_limit,config.state
FROM monitor_intent_analysis_runs AS run
JOIN monitor_intent_drafts AS draft
  ON draft.id=run.draft_id AND draft.monitor_id=run.monitor_id
JOIN monitor_intent_draft_revisions AS revision
  ON revision.draft_id=run.draft_id AND revision.resource_version=run.draft_resource_version
 AND revision.monitor_id=run.monitor_id AND revision.config_version_id=draft.config_version_id
JOIN monitor_config_versions AS config
  ON config.id=draft.config_version_id AND config.monitor_id=run.monitor_id
WHERE run.id=$1 AND run.monitor_id=$2 AND run.draft_id=$3 AND run.draft_resource_version=$4
FOR UPDATE OF run,draft,revision,config`, command.Task.Run.RunID, command.Task.Run.MonitorID,
		command.Task.Run.DraftID, command.Task.Run.DraftResourceVersion).Scan(
		&owner.MonitorID, &owner.ConfigVersionID, &owner.PreviewRunID, &owner.DraftID,
		&owner.DraftResourceVersion, &owner.IntentRevisionID, &kind, &status, &inputHash,
		&profileVersion, &sampleLimit, &configState,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return compiledIntentProfileRecord{}, monitorapplication.ErrIntentRunNotFound
	}
	if err != nil {
		return compiledIntentProfileRecord{}, mapIntentDatabaseError(err)
	}
	if kind != "preview" || status != "running" || configState != "draft" || inputHash != command.Task.Run.InputHash ||
		profileVersion != command.Task.AnalysisProfile || sampleLimit != command.Task.SampleLimit {
		return compiledIntentProfileRecord{}, monitorapplication.ErrIntentRunStateConflict
	}
	return owner, nil
}

func readCompiledIntentProfileByPreviewRun(ctx context.Context, executor intentExecutor, previewRunID int64) (compiledIntentProfileRecord, error) {
	var record compiledIntentProfileRecord
	var reason sql.NullString
	var hash sql.NullString
	err := executor.QueryRowContext(ctx, `
SELECT id,monitor_id,config_version_id,preview_run_id,draft_id,draft_resource_version,intent_revision_id,
       compiler_version,matching_algorithm_version,lexical_algorithm_version,semantic_algorithm_version,
       structured_algorithm_version,search_normalization_profile_version,semantic_state,
       semantic_unavailable_reason,status,btrim(profile_hash)
FROM monitor_compiled_profiles WHERE preview_run_id=$1 FOR UPDATE`, previewRunID).Scan(
		&record.ID, &record.MonitorID, &record.ConfigVersionID, &record.PreviewRunID, &record.DraftID,
		&record.DraftResourceVersion, &record.IntentRevisionID, &record.CompilerVersion,
		&record.MatchingAlgorithmVersion, &record.LexicalAlgorithmVersion, &record.SemanticAlgorithmVersion,
		&record.StructuredAlgorithmVersion, &record.SearchNormalizationProfileVersion, &record.SemanticState,
		&reason, &record.Status, &hash,
	)
	record.SemanticUnavailableReason, record.ProfileHash = reason.String, hash.String
	return record, err
}

func sameCompiledIntentProfile(record, owner compiledIntentProfileRecord, command monitorapplication.PersistPreviewCompiledProfileDTO) bool {
	validStatus := record.Status == "building" || record.Status == "ready" && record.ProfileHash == command.ProfileHash
	validSemantic := record.SemanticState == monitorapplication.IntentSemanticStateReady && record.SemanticUnavailableReason == "" ||
		record.SemanticState == monitorapplication.IntentSemanticStateUnavailable &&
			(record.SemanticUnavailableReason == monitorapplication.IntentSemanticGenerationUnavailable ||
				record.SemanticUnavailableReason == monitorapplication.IntentSemanticModelUnavailable)
	return record.ID > 0 && validStatus && validSemantic && record.MonitorID == owner.MonitorID &&
		record.ConfigVersionID == owner.ConfigVersionID && record.PreviewRunID == owner.PreviewRunID &&
		record.DraftID == owner.DraftID && record.DraftResourceVersion == owner.DraftResourceVersion &&
		record.IntentRevisionID == owner.IntentRevisionID && record.CompilerVersion == command.CompilerVersion &&
		record.MatchingAlgorithmVersion == command.MatchingAlgorithmVersion && record.LexicalAlgorithmVersion == command.LexicalAlgorithmVersion &&
		record.SemanticAlgorithmVersion == command.SemanticAlgorithmVersion && record.StructuredAlgorithmVersion == command.StructuredAlgorithmVersion &&
		record.SearchNormalizationProfileVersion == command.SearchNormalizationProfileVersion
}

func insertCompiledIntentProfile(ctx context.Context, executor intentExecutor, owner compiledIntentProfileRecord, command monitorapplication.PersistPreviewCompiledProfileDTO) (int64, error) {
	var profileID int64
	err := executor.QueryRowContext(ctx, `
INSERT INTO monitor_compiled_profiles (
  monitor_id,purpose,config_version_id,preview_run_id,draft_id,draft_resource_version,intent_revision_id,
  compiler_version,matching_algorithm_version,lexical_algorithm_version,semantic_algorithm_version,
  structured_algorithm_version,search_normalization_profile_version,semantic_state,semantic_unavailable_reason
) VALUES ($1,'preview',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
RETURNING id`, owner.MonitorID, owner.ConfigVersionID, owner.PreviewRunID, owner.DraftID,
		owner.DraftResourceVersion, owner.IntentRevisionID, command.CompilerVersion, command.MatchingAlgorithmVersion,
		command.LexicalAlgorithmVersion, command.SemanticAlgorithmVersion, command.StructuredAlgorithmVersion,
		command.SearchNormalizationProfileVersion, command.SemanticState, command.SemanticUnavailableReason).Scan(&profileID)
	if err != nil {
		return 0, mapIntentDatabaseError(err)
	}
	return profileID, nil
}

func insertCompiledIntentProfileFacts(ctx context.Context, executor intentExecutor, profileID int64, command monitorapplication.PersistPreviewCompiledProfileDTO) error {
	for ordinal, clause := range command.Clauses {
		if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_compiled_clauses (compiled_profile_id,ordinal,operator,field,value,normalized_value,origin)
VALUES ($1,$2,$3,$4,$5,$6,$7)`, profileID, ordinal, clause.Operator, clause.Field, clause.Value, clause.NormalizedValue, clause.Origin); err != nil {
			return mapIntentDatabaseError(err)
		}
	}
	for ordinal, entity := range command.Entities {
		var entityID int64
		if err := executor.QueryRowContext(ctx, `
INSERT INTO monitor_compiled_entities (compiled_profile_id,ordinal,canonical_id)
VALUES ($1,$2,$3) RETURNING id`, profileID, ordinal, entity.CanonicalID).Scan(&entityID); err != nil {
			return mapIntentDatabaseError(err)
		}
		for aliasOrdinal, alias := range entity.Aliases {
			if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_compiled_entity_aliases (compiled_entity_id,compiled_profile_id,ordinal,alias,normalized_alias)
VALUES ($1,$2,$3,$4,$5)`, entityID, profileID, aliasOrdinal, alias, entity.NormalizedAliases[aliasOrdinal]); err != nil {
				return mapIntentDatabaseError(err)
			}
		}
	}
	return nil
}

func readCompiledIntentProfileClauses(ctx context.Context, executor intentExecutor, profileID int64) ([]monitorapplication.CompiledIntentClauseDTO, error) {
	rows, err := executor.QueryContext(ctx, `SELECT operator,field,value,normalized_value,origin FROM monitor_compiled_clauses WHERE compiled_profile_id=$1 ORDER BY ordinal`, profileID)
	if err != nil {
		return nil, mapIntentDatabaseError(err)
	}
	defer func() { _ = rows.Close() }()
	result := []monitorapplication.CompiledIntentClauseDTO{}
	for rows.Next() {
		var record compiledIntentClauseRecord
		if err := rows.Scan(&record.Operator, &record.Field, &record.Value, &record.NormalizedValue, &record.Origin); err != nil {
			return nil, mapIntentDatabaseError(err)
		}
		result = append(result, compiledIntentClauseDTO(record))
	}
	if err := rows.Err(); err != nil {
		return nil, mapIntentDatabaseError(err)
	}
	return result, nil
}

func readCompiledIntentProfileEntities(ctx context.Context, executor intentExecutor, profileID int64) ([]monitorapplication.CompiledIntentEntityDTO, error) {
	rows, err := executor.QueryContext(ctx, `SELECT id,canonical_id FROM monitor_compiled_entities WHERE compiled_profile_id=$1 ORDER BY ordinal`, profileID)
	if err != nil {
		return nil, mapIntentDatabaseError(err)
	}
	records := []compiledIntentEntityRecord{}
	for rows.Next() {
		var record compiledIntentEntityRecord
		if err := rows.Scan(&record.ID, &record.CanonicalID); err != nil {
			_ = rows.Close()
			return nil, mapIntentDatabaseError(err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, mapIntentDatabaseError(err)
	}
	if err := rows.Close(); err != nil {
		return nil, mapIntentDatabaseError(err)
	}
	result := make([]monitorapplication.CompiledIntentEntityDTO, 0, len(records))
	for _, record := range records {
		aliasRows, aliasErr := executor.QueryContext(ctx, `
SELECT alias,normalized_alias FROM monitor_compiled_entity_aliases
WHERE compiled_profile_id=$1 AND compiled_entity_id=$2 ORDER BY ordinal`, profileID, record.ID)
		if aliasErr != nil {
			return nil, mapIntentDatabaseError(aliasErr)
		}
		for aliasRows.Next() {
			var alias, normalizedAlias string
			if aliasErr = aliasRows.Scan(&alias, &normalizedAlias); aliasErr != nil {
				_ = aliasRows.Close()
				return nil, mapIntentDatabaseError(aliasErr)
			}
			record.Aliases = append(record.Aliases, alias)
			record.NormalizedAliases = append(record.NormalizedAliases, normalizedAlias)
		}
		if aliasErr = aliasRows.Err(); aliasErr != nil {
			_ = aliasRows.Close()
			return nil, mapIntentDatabaseError(aliasErr)
		}
		if aliasErr = aliasRows.Close(); aliasErr != nil {
			return nil, mapIntentDatabaseError(aliasErr)
		}
		result = append(result, compiledIntentEntityDTO(record))
	}
	return result, nil
}
