package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

var _ operationsapplication.DecisionQualityRepository = (*DecisionQualityRepository)(nil)

type DecisionQualityRepository struct{ runtime *database.Runtime }

func NewDecisionQualityRepository(runtime *database.Runtime) *DecisionQualityRepository {
	return &DecisionQualityRepository{runtime: runtime}
}

func (repository *DecisionQualityRepository) IsDecisionQualityProfileActive(ctx context.Context, module, profileVersion string) (bool, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || strings.TrimSpace(module) == "" || strings.TrimSpace(profileVersion) == "" {
		return false, fmt.Errorf("%w: invalid decision quality profile query", sharedrepository.ErrInvalidInput)
	}
	var active bool
	err := repository.runtime.SQL.QueryRowContext(ctx, `
SELECT EXISTS(
 SELECT 1 FROM decision_quality_profiles AS profile
 JOIN decision_quality_evaluation_runs AS evaluation ON evaluation.id=profile.evaluation_run_id
 WHERE profile.module=$1 AND profile.profile_version=$2 AND profile.status='active' AND evaluation.passed
)`, strings.TrimSpace(module), strings.TrimSpace(profileVersion)).Scan(&active)
	if err != nil {
		return false, databaserepository.MapError(err)
	}
	return active, nil
}

type decisionQualityRunRecord struct {
	id, version, evaluatedBy                    int64
	module, profile, datasetVersion, datasetSHA string
	sampleCount, positiveCount, negativeCount   int
	passed                                      bool
	evaluatedAt                                 time.Time
}

type decisionQualityProfileRecord struct {
	id, version, evaluationRunID int64
	module, profile, status      string
	activatedBy                  sql.NullInt64
	activatedAt                  sql.NullTime
}

func (repository *DecisionQualityRepository) PersistDecisionQualityEvaluation(ctx context.Context, command operationsapplication.PersistDecisionQualityEvaluationCommand) (operationsapplication.PersistDecisionQualityEvaluationResult, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || !validDecisionQualityPersistenceCommand(command) {
		return operationsapplication.PersistDecisionQualityEvaluationResult{}, fmt.Errorf("%w: invalid decision quality persistence", sharedrepository.ErrInvalidInput)
	}
	transaction, err := repository.runtime.SQL.BeginTx(ctx, nil)
	if err != nil {
		return operationsapplication.PersistDecisionQualityEvaluationResult{}, databaserepository.MapError(err)
	}
	defer func() { _ = transaction.Rollback() }()
	if _, err := transaction.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('decision-quality-profile-activation'))`); err != nil {
		return operationsapplication.PersistDecisionQualityEvaluationResult{}, databaserepository.MapError(err)
	}
	var activeAdministrator bool
	if err := transaction.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND role='admin' AND status='active' AND deleted_at IS NULL)`, command.ActorUserID).Scan(&activeAdministrator); err != nil || !activeAdministrator {
		if err != nil {
			return operationsapplication.PersistDecisionQualityEvaluationResult{}, databaserepository.MapError(err)
		}
		return operationsapplication.PersistDecisionQualityEvaluationResult{}, fmt.Errorf("%w: decision quality evaluator must be an active administrator", sharedrepository.ErrConstraint)
	}
	metrics := append([]operationsapplication.DecisionQualityMetricDTO(nil), command.Metrics...)
	sort.Slice(metrics, func(left, right int) bool { return metrics[left].Module < metrics[right].Module })
	result := operationsapplication.PersistDecisionQualityEvaluationResult{Runs: make([]operationsapplication.DecisionQualityEvaluationRunDTO, 0, len(metrics)), Profiles: make([]operationsapplication.DecisionQualityProfileDTO, 0, len(metrics))}
	for _, metric := range metrics {
		run, created, err := persistDecisionQualityRun(ctx, transaction, command, metric)
		if err != nil {
			return operationsapplication.PersistDecisionQualityEvaluationResult{}, err
		}
		if !created {
			result.Reused = true
		}
		if err := persistDecisionQualitySlices(ctx, transaction, run.id, metric.Module, command.Slices); err != nil {
			return operationsapplication.PersistDecisionQualityEvaluationResult{}, err
		}
		profile, err := persistDecisionQualityProfile(ctx, transaction, command, metric, run.id)
		if err != nil {
			return operationsapplication.PersistDecisionQualityEvaluationResult{}, err
		}
		result.Runs = append(result.Runs, decisionQualityRunDTO(run))
		result.Profiles = append(result.Profiles, decisionQualityProfileDTO(profile))
	}
	if err := transaction.Commit(); err != nil {
		return operationsapplication.PersistDecisionQualityEvaluationResult{}, databaserepository.MapError(err)
	}
	return result, nil
}

func persistDecisionQualityRun(ctx context.Context, transaction *sql.Tx, command operationsapplication.PersistDecisionQualityEvaluationCommand, metric operationsapplication.DecisionQualityMetricDTO) (decisionQualityRunRecord, bool, error) {
	arguments := decisionQualityRunArguments(command, metric)
	row := transaction.QueryRowContext(ctx, `
INSERT INTO decision_quality_evaluation_runs (
 module,profile_version,dataset_version,dataset_sha256,annotation_protocol_version,annotation_guideline_sha256,
 split_strategy_version,family_isolation_sha256,event_isolation_sha256,annotator_count,agreement_metric,agreement_score,
 time_boundary,sample_count,positive_count,negative_count,precision_score,recall_score,precision_wilson_lower,
 false_merge_rate,pairwise_precision,b_cubed_f1,ceaf_e,cluster_count_ratio,locator_accuracy,provenance_completeness,
 evidence_relation_macro_f1,hotspot_precision,median_discovery_delay_seconds,passed,reason_codes,evaluated_by_user_id,evaluated_at
) VALUES (`+qualityPlaceholders(33)+`)
ON CONFLICT (module,profile_version,dataset_sha256) DO NOTHING
RETURNING id,version,module,profile_version,dataset_version,dataset_sha256,sample_count,positive_count,negative_count,passed,evaluated_by_user_id,evaluated_at`, arguments...)
	record, err := scanDecisionQualityRun(row)
	if err == nil {
		return record, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return decisionQualityRunRecord{}, false, databaserepository.MapError(err)
	}
	record, err = scanDecisionQualityRun(transaction.QueryRowContext(ctx, `
SELECT id,version,module,profile_version,dataset_version,dataset_sha256,sample_count,positive_count,negative_count,passed,evaluated_by_user_id,evaluated_at
FROM decision_quality_evaluation_runs WHERE module=$1 AND profile_version=$2 AND dataset_sha256=$3 FOR KEY SHARE`, metric.Module, metric.ProfileVersion, command.Dataset.DatasetSHA256))
	if err != nil {
		return decisionQualityRunRecord{}, false, databaserepository.MapError(err)
	}
	if record.datasetVersion != command.Dataset.DatasetVersion || record.sampleCount != metric.SampleCount || record.positiveCount != metric.PositiveCount || record.negativeCount != metric.NegativeCount || record.passed != metric.Passed || record.evaluatedBy != command.ActorUserID {
		return decisionQualityRunRecord{}, false, fmt.Errorf("%w: decision quality evaluation replay changed", sharedrepository.ErrConflict)
	}
	return record, false, nil
}

func decisionQualityRunArguments(command operationsapplication.PersistDecisionQualityEvaluationCommand, metric operationsapplication.DecisionQualityMetricDTO) []any {
	return []any{metric.Module, metric.ProfileVersion, command.Dataset.DatasetVersion, command.Dataset.DatasetSHA256,
		command.Dataset.AnnotationProtocolVersion, command.Dataset.AnnotationGuidelineSHA256, command.Dataset.SplitStrategyVersion,
		command.Dataset.FamilyIsolationSHA256, command.Dataset.EventIsolationSHA256, command.Dataset.AnnotatorCount,
		command.Dataset.AgreementMetric, roundQualityMetric(command.Dataset.AgreementScore), command.Dataset.TimeBoundary.UTC(),
		metric.SampleCount, metric.PositiveCount, metric.NegativeCount, roundQualityMetric(metric.Precision), roundQualityMetric(metric.Recall),
		roundQualityMetric(metric.PrecisionWilsonLower), roundQualityMetric(metric.FalseMergeRate), roundQualityMetric(metric.PairwisePrecision),
		roundQualityMetric(metric.BCubedF1), roundQualityMetric(metric.CEAFE), roundQualityMetric(metric.ClusterCountRatio),
		roundQualityMetric(metric.LocatorAccuracy), roundQualityMetric(metric.ProvenanceCompleteness), roundQualityMetric(metric.EvidenceRelationMacroF1),
		roundQualityMetric(metric.HotspotPrecision), math.Round(metric.MedianDiscoveryDelaySeconds*1000) / 1000, metric.Passed,
		append([]string(nil), metric.ReasonCodes...), command.ActorUserID, command.EvaluatedAt.UTC()}
}

func persistDecisionQualitySlices(ctx context.Context, transaction *sql.Tx, runID int64, module string, slices []operationsapplication.DecisionQualitySliceDTO) error {
	for _, slice := range slices {
		if slice.Module != module {
			continue
		}
		result, err := transaction.ExecContext(ctx, `
INSERT INTO decision_quality_evaluation_slices (evaluation_run_id,module,dimension,value,sample_count,precision_score,recall_score,passed)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (evaluation_run_id,dimension,value) DO NOTHING`,
			runID, module, slice.Dimension, slice.Value, slice.SampleCount, roundQualityMetric(slice.Precision), roundQualityMetric(slice.Recall), slice.Passed)
		if err != nil {
			return databaserepository.MapError(err)
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			var count int
			if err := transaction.QueryRowContext(ctx, `SELECT count(*) FROM decision_quality_evaluation_slices WHERE evaluation_run_id=$1 AND module=$2 AND dimension=$3 AND value=$4 AND sample_count=$5 AND precision_score=$6 AND recall_score=$7 AND passed=$8`, runID, module, slice.Dimension, slice.Value, slice.SampleCount, roundQualityMetric(slice.Precision), roundQualityMetric(slice.Recall), slice.Passed).Scan(&count); err != nil || count != 1 {
				if err != nil {
					return databaserepository.MapError(err)
				}
				return fmt.Errorf("%w: decision quality slice replay changed", sharedrepository.ErrConflict)
			}
		}
	}
	return nil
}

func persistDecisionQualityProfile(ctx context.Context, transaction *sql.Tx, command operationsapplication.PersistDecisionQualityEvaluationCommand, metric operationsapplication.DecisionQualityMetricDTO, runID int64) (decisionQualityProfileRecord, error) {
	wanted := "shadow"
	if command.Activate && metric.Passed {
		wanted = "active"
	}
	if wanted == "active" {
		if _, err := transaction.ExecContext(ctx, `
UPDATE decision_quality_profiles SET version=version+1,status='rolled_back',rolled_back_by_user_id=$2,rolled_back_at=$3,updated_at=$3
WHERE module=$1 AND status='active' AND profile_version<>$4`, metric.Module, command.ActorUserID, command.EvaluatedAt.UTC(), metric.ProfileVersion); err != nil {
			return decisionQualityProfileRecord{}, databaserepository.MapError(err)
		}
	}
	var row *sql.Row
	if wanted == "active" {
		row = transaction.QueryRowContext(ctx, `
INSERT INTO decision_quality_profiles (module,profile_version,status,evaluation_run_id,created_by_user_id,activated_by_user_id,activated_at)
VALUES ($1,$2,'active',$3,$4,$4,$5)
ON CONFLICT (module,profile_version) DO UPDATE SET
 version=CASE WHEN decision_quality_profiles.status='shadow' THEN decision_quality_profiles.version+1 ELSE decision_quality_profiles.version END,
 status='active',evaluation_run_id=EXCLUDED.evaluation_run_id,
 activated_by_user_id=COALESCE(decision_quality_profiles.activated_by_user_id,EXCLUDED.activated_by_user_id),
 activated_at=COALESCE(decision_quality_profiles.activated_at,EXCLUDED.activated_at),updated_at=EXCLUDED.activated_at
WHERE decision_quality_profiles.status IN ('shadow','active') AND decision_quality_profiles.evaluation_run_id=EXCLUDED.evaluation_run_id
RETURNING id,version,module,profile_version,status,evaluation_run_id,activated_by_user_id,activated_at`,
			metric.Module, metric.ProfileVersion, runID, command.ActorUserID, command.EvaluatedAt.UTC())
	} else {
		row = transaction.QueryRowContext(ctx, `
INSERT INTO decision_quality_profiles (module,profile_version,status,evaluation_run_id,created_by_user_id)
VALUES ($1,$2,'shadow',$3,$4) ON CONFLICT (module,profile_version) DO NOTHING
RETURNING id,version,module,profile_version,status,evaluation_run_id,activated_by_user_id,activated_at`, metric.Module, metric.ProfileVersion, runID, command.ActorUserID)
	}
	record, err := scanDecisionQualityProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		record, err = scanDecisionQualityProfile(transaction.QueryRowContext(ctx, `SELECT id,version,module,profile_version,status,evaluation_run_id,activated_by_user_id,activated_at FROM decision_quality_profiles WHERE module=$1 AND profile_version=$2 FOR KEY SHARE`, metric.Module, metric.ProfileVersion))
	}
	if err != nil {
		return decisionQualityProfileRecord{}, databaserepository.MapError(err)
	}
	if record.evaluationRunID != runID || wanted == "active" && record.status != "active" || wanted == "shadow" && record.status != "shadow" && record.status != "active" {
		return decisionQualityProfileRecord{}, fmt.Errorf("%w: decision quality profile replay changed", sharedrepository.ErrConflict)
	}
	return record, nil
}

func scanDecisionQualityRun(row *sql.Row) (decisionQualityRunRecord, error) {
	var value decisionQualityRunRecord
	err := row.Scan(&value.id, &value.version, &value.module, &value.profile, &value.datasetVersion, &value.datasetSHA,
		&value.sampleCount, &value.positiveCount, &value.negativeCount, &value.passed, &value.evaluatedBy, &value.evaluatedAt)
	return value, err
}

func scanDecisionQualityProfile(row *sql.Row) (decisionQualityProfileRecord, error) {
	var value decisionQualityProfileRecord
	err := row.Scan(&value.id, &value.version, &value.module, &value.profile, &value.status, &value.evaluationRunID, &value.activatedBy, &value.activatedAt)
	return value, err
}

func decisionQualityRunDTO(value decisionQualityRunRecord) operationsapplication.DecisionQualityEvaluationRunDTO {
	return operationsapplication.DecisionQualityEvaluationRunDTO{ID: value.id, Version: value.version, Module: value.module,
		ProfileVersion: value.profile, DatasetVersion: value.datasetVersion, DatasetSHA256: value.datasetSHA,
		SampleCount: value.sampleCount, PositiveCount: value.positiveCount, NegativeCount: value.negativeCount,
		Passed: value.passed, EvaluatedByUserID: value.evaluatedBy, EvaluatedAt: value.evaluatedAt.UTC()}
}

func decisionQualityProfileDTO(value decisionQualityProfileRecord) operationsapplication.DecisionQualityProfileDTO {
	result := operationsapplication.DecisionQualityProfileDTO{ID: value.id, Version: value.version,
		EvaluationRunID: value.evaluationRunID, Module: value.module, ProfileVersion: value.profile, Status: value.status}
	if value.activatedBy.Valid {
		actor := value.activatedBy.Int64
		result.ActivatedByUserID = &actor
	}
	if value.activatedAt.Valid {
		activated := value.activatedAt.Time.UTC()
		result.ActivatedAt = &activated
	}
	return result
}

func validDecisionQualityPersistenceCommand(value operationsapplication.PersistDecisionQualityEvaluationCommand) bool {
	if value.ActorUserID <= 0 || value.EvaluatedAt.IsZero() || value.Dataset.DatasetVersion == "" || value.Dataset.DatasetSHA256 == "" || len(value.Metrics) != 5 {
		return false
	}
	modules := map[string]bool{}
	for _, metric := range value.Metrics {
		if metric.Module == "" || metric.ProfileVersion == "" || metric.SampleCount < 0 || len(metric.ReasonCodes) == 0 || modules[metric.Module] {
			return false
		}
		modules[metric.Module] = true
	}
	return true
}

func roundQualityMetric(value float64) float64 { return math.Round(value*1e7) / 1e7 }

func qualityPlaceholders(count int) string {
	value := ""
	for index := 1; index <= count; index++ {
		if index > 1 {
			value += ","
		}
		value += fmt.Sprintf("$%d", index)
	}
	return value
}
