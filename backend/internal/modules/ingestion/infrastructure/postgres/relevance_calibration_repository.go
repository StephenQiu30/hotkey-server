package postgres

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"reflect"
	"strings"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

var _ ingestionapplication.RelevanceCalibrationRepository = (*DocumentMatchRepository)(nil)

type relevanceEvaluationRunRecord struct {
	ID                                                               int64
	DatasetVersion, DatasetHash, FamilyIsolationHash                 string
	AnnotationProtocolVersion, AnnotationGuidelineSHA256             string
	SplitStrategyVersion, AgreementMetric                            string
	AnnotatorCount                                                   int
	AgreementScore                                                   float64
	TimeBoundary                                                     time.Time
	SampleWindowStart, SampleWindowEnd                               time.Time
	MatchingAlgorithmVersion, RerankerVersion, CalibrationVersion    string
	CalibrationSlope, CalibrationIntercept                           float64
	RejectThreshold, AcceptThreshold                                 float64
	SampleCount, PositiveCount, NegativeCount                        int
	RecallAt100, Precision, Recall, ECE, Brier, PrecisionWilsonLower float64
	HardNegativeCount                                                int
	HardNegativePassed                                               bool
	Status                                                           string
	EvaluatedByUserID                                                int64
	EvaluatedAt                                                      time.Time
}

type relevanceEvaluationSliceRecord struct {
	Dimension, Value                          string
	SampleCount, PositiveCount, NegativeCount int
	Precision, Recall                         float64
	Passed                                    bool
}

type relevanceCalibrationProfileRecord struct {
	ID, Version, EvaluationRunID                                  int64
	MatchingAlgorithmVersion, RerankerVersion, CalibrationVersion string
	Status                                                        string
	RejectThreshold, AcceptThreshold                              float64
	CalibrationSlope, CalibrationIntercept                        float64
	EvaluationSampleCount                                         int
}

func (repository *DocumentMatchRepository) PersistRelevanceCalibration(ctx context.Context, command ingestionapplication.PersistRelevanceCalibrationCommand) (ingestionapplication.RelevanceCalibrationProfileDTO, error) {
	if repository == nil || repository.runtime == nil || !validRelevanceCalibrationCommand(command) {
		return ingestionapplication.RelevanceCalibrationProfileDTO{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	var result ingestionapplication.RelevanceCalibrationProfileDTO
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		var authorized bool
		if err := transaction.SQL.QueryRowContext(transactionCtx, `
SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND role='admin' AND status='active' AND deleted_at IS NULL)`, command.ActorUserID).Scan(&authorized); err != nil {
			return databaserepository.MapError(err)
		}
		if !authorized {
			return ingestionapplication.ErrDocumentMatchAuthorizationDenied
		}
		run, created, err := persistRelevanceEvaluationRun(transactionCtx, transaction.SQL, command)
		if err != nil {
			return err
		}
		if created {
			for _, slice := range command.Slices {
				if _, err := transaction.SQL.ExecContext(transactionCtx, `
INSERT INTO relevance_evaluation_slices (
 evaluation_run_id,dimension,value,sample_count,positive_count,negative_count,precision_score,recall_score,passed
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, run.ID, slice.Dimension, slice.Value, slice.SampleCount,
					slice.PositiveCount, slice.NegativeCount, slice.Precision, slice.Recall, slice.Passed); err != nil {
					return databaserepository.MapError(err)
				}
			}
		}
		storedSlices, err := readRelevanceEvaluationSlices(transactionCtx, transaction.SQL, run.ID)
		if err != nil {
			return err
		}
		if !sameRelevanceEvaluationRun(run, command) || !reflect.DeepEqual(storedSlices, relevanceEvaluationSliceRecords(command.Slices)) {
			return sharedrepository.ErrConflict
		}
		profile, err := persistRelevanceDecisionProfile(transactionCtx, transaction.SQL, run.ID, command)
		if err != nil {
			return err
		}
		result = relevanceCalibrationProfileDTO(profile)
		return nil
	})
	if err != nil {
		return ingestionapplication.RelevanceCalibrationProfileDTO{}, err
	}
	return result, nil
}

func persistRelevanceEvaluationRun(ctx context.Context, transaction *sql.Tx, command ingestionapplication.PersistRelevanceCalibrationCommand) (relevanceEvaluationRunRecord, bool, error) {
	var id int64
	err := transaction.QueryRowContext(ctx, `
INSERT INTO relevance_evaluation_runs (
 dataset_version,dataset_hash,family_isolation_hash,annotation_protocol_version,annotation_guideline_sha256,
 split_strategy_version,annotator_count,agreement_metric,agreement_score,time_boundary,sample_window_start,sample_window_end,
 matching_algorithm_version,reranker_version,calibration_version,calibration_slope,calibration_intercept,reject_threshold,accept_threshold,
 sample_count,positive_count,negative_count,recall_at_100,precision_score,recall_score,
 expected_calibration_error,brier_score,precision_wilson_lower,hard_negative_count,hard_negative_passed,
 status,evaluated_by_user_id,evaluated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33)
ON CONFLICT (dataset_hash,matching_algorithm_version,reranker_version,calibration_version,reject_threshold,accept_threshold)
DO NOTHING RETURNING id`, command.DatasetVersion, command.DatasetHash, command.FamilyIsolationHash,
		command.AnnotationProtocolVersion, command.AnnotationGuidelineSHA256, command.SplitStrategyVersion,
		command.AnnotatorCount, command.AgreementMetric, command.AgreementScore, command.TimeBoundary,
		command.SampleWindowStart, command.SampleWindowEnd,
		command.MatchingAlgorithmVersion, command.RerankerVersion, command.CalibrationVersion,
		command.CalibrationSlope, command.CalibrationIntercept, command.RejectThreshold, command.AcceptThreshold,
		command.Metrics.SampleCount, command.Metrics.PositiveCount,
		command.Metrics.NegativeCount, command.Metrics.RecallAt100, command.Metrics.Precision, command.Metrics.Recall,
		command.Metrics.ECE, command.Metrics.Brier, command.Metrics.PrecisionWilsonLower, command.Metrics.HardNegativeCount,
		command.Metrics.HardNegativePassed, relevanceEvaluationStatus(command.Metrics.Passed), command.ActorUserID, command.EvaluatedAt).Scan(&id)
	created := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return relevanceEvaluationRunRecord{}, false, databaserepository.MapError(err)
	}
	run, err := readRelevanceEvaluationRun(ctx, transaction, command.DatasetHash, command.MatchingAlgorithmVersion,
		command.RerankerVersion, command.CalibrationVersion, command.RejectThreshold, command.AcceptThreshold)
	return run, created, err
}

func readRelevanceEvaluationRun(ctx context.Context, transaction *sql.Tx, datasetHash, matchingVersion, rerankerVersion, calibrationVersion string, rejectThreshold, acceptThreshold float64) (relevanceEvaluationRunRecord, error) {
	var record relevanceEvaluationRunRecord
	err := transaction.QueryRowContext(ctx, `
SELECT id,dataset_version,btrim(dataset_hash),btrim(family_isolation_hash),annotation_protocol_version,
       btrim(annotation_guideline_sha256),split_strategy_version,annotator_count,agreement_metric,agreement_score::float8,
       time_boundary,sample_window_start,sample_window_end,
       matching_algorithm_version,reranker_version,calibration_version,calibration_slope::float8,calibration_intercept::float8,
       reject_threshold::float8,accept_threshold::float8,
       sample_count,positive_count,negative_count,recall_at_100::float8,precision_score::float8,recall_score::float8,
       expected_calibration_error::float8,brier_score::float8,precision_wilson_lower::float8,
       hard_negative_count,hard_negative_passed,status,evaluated_by_user_id,evaluated_at
FROM relevance_evaluation_runs
WHERE dataset_hash=$1 AND matching_algorithm_version=$2 AND reranker_version=$3 AND calibration_version=$4
	  AND reject_threshold=$5 AND accept_threshold=$6
FOR UPDATE`, datasetHash, matchingVersion, rerankerVersion, calibrationVersion, rejectThreshold, acceptThreshold).Scan(
		&record.ID, &record.DatasetVersion, &record.DatasetHash, &record.FamilyIsolationHash,
		&record.AnnotationProtocolVersion, &record.AnnotationGuidelineSHA256, &record.SplitStrategyVersion,
		&record.AnnotatorCount, &record.AgreementMetric, &record.AgreementScore,
		&record.TimeBoundary,
		&record.SampleWindowStart, &record.SampleWindowEnd,
		&record.MatchingAlgorithmVersion, &record.RerankerVersion, &record.CalibrationVersion,
		&record.CalibrationSlope, &record.CalibrationIntercept, &record.RejectThreshold, &record.AcceptThreshold,
		&record.SampleCount, &record.PositiveCount, &record.NegativeCount,
		&record.RecallAt100, &record.Precision, &record.Recall, &record.ECE, &record.Brier, &record.PrecisionWilsonLower,
		&record.HardNegativeCount, &record.HardNegativePassed, &record.Status, &record.EvaluatedByUserID, &record.EvaluatedAt,
	)
	if err != nil {
		return relevanceEvaluationRunRecord{}, databaserepository.MapError(err)
	}
	return record, nil
}

func readRelevanceEvaluationSlices(ctx context.Context, transaction *sql.Tx, runID int64) ([]relevanceEvaluationSliceRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
SELECT dimension,value,sample_count,positive_count,negative_count,precision_score::float8,recall_score::float8,passed
FROM relevance_evaluation_slices WHERE evaluation_run_id=$1 ORDER BY dimension,value`, runID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	result := []relevanceEvaluationSliceRecord{}
	for rows.Next() {
		var record relevanceEvaluationSliceRecord
		if err := rows.Scan(&record.Dimension, &record.Value, &record.SampleCount, &record.PositiveCount, &record.NegativeCount,
			&record.Precision, &record.Recall, &record.Passed); err != nil {
			return nil, databaserepository.MapError(err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return result, nil
}

func persistRelevanceDecisionProfile(ctx context.Context, transaction *sql.Tx, evaluationRunID int64, command ingestionapplication.PersistRelevanceCalibrationCommand) (relevanceCalibrationProfileRecord, error) {
	var id int64
	var activatedBy any
	var activatedAt any
	if command.Status == "active" {
		activatedBy, activatedAt = command.ActorUserID, command.EvaluatedAt
	}
	err := transaction.QueryRowContext(ctx, `
INSERT INTO relevance_decision_profiles (
 profile_name,matching_algorithm_version,reranker_version,calibration_version,status,
 reject_threshold,accept_threshold,calibration_slope,calibration_intercept,evaluation_sample_count,evaluation_run_id,created_by_user_id,
 activated_by_user_id,activated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (matching_algorithm_version,reranker_version,calibration_version) DO NOTHING
RETURNING id`, command.ProfileName, command.MatchingAlgorithmVersion, command.RerankerVersion,
		command.CalibrationVersion, command.Status, command.RejectThreshold, command.AcceptThreshold,
		command.CalibrationSlope, command.CalibrationIntercept, command.Metrics.SampleCount, evaluationRunID,
		command.ActorUserID, activatedBy, activatedAt).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return relevanceCalibrationProfileRecord{}, databaserepository.MapError(err)
	}
	var record relevanceCalibrationProfileRecord
	err = transaction.QueryRowContext(ctx, `
SELECT id,version,evaluation_run_id,matching_algorithm_version,reranker_version,calibration_version,
       status,reject_threshold::float8,accept_threshold::float8,calibration_slope::float8,calibration_intercept::float8,evaluation_sample_count
FROM relevance_decision_profiles
WHERE matching_algorithm_version=$1 AND reranker_version=$2 AND calibration_version=$3
FOR UPDATE`, command.MatchingAlgorithmVersion, command.RerankerVersion, command.CalibrationVersion).Scan(
		&record.ID, &record.Version, &record.EvaluationRunID, &record.MatchingAlgorithmVersion,
		&record.RerankerVersion, &record.CalibrationVersion, &record.Status,
		&record.RejectThreshold, &record.AcceptThreshold, &record.CalibrationSlope, &record.CalibrationIntercept,
		&record.EvaluationSampleCount,
	)
	if err != nil {
		return relevanceCalibrationProfileRecord{}, databaserepository.MapError(err)
	}
	if record.EvaluationRunID != evaluationRunID || record.Status != command.Status || record.RejectThreshold != command.RejectThreshold ||
		record.AcceptThreshold != command.AcceptThreshold || record.CalibrationSlope != command.CalibrationSlope ||
		record.CalibrationIntercept != command.CalibrationIntercept || record.EvaluationSampleCount != command.Metrics.SampleCount {
		return relevanceCalibrationProfileRecord{}, sharedrepository.ErrConflict
	}
	return record, nil
}

func validRelevanceCalibrationCommand(command ingestionapplication.PersistRelevanceCalibrationCommand) bool {
	if command.ActorUserID <= 0 || strings.TrimSpace(command.ProfileName) != command.ProfileName || command.ProfileName == "" ||
		len(command.ProfileName) > 120 || !validRelevanceCalibrationVersion(command.DatasetVersion) ||
		!validDocumentMatchHashRecord(command.DatasetHash) || !validDocumentMatchHashRecord(command.FamilyIsolationHash) ||
		!validRelevanceCalibrationVersion(command.AnnotationProtocolVersion) || !validDocumentMatchHashRecord(command.AnnotationGuidelineSHA256) ||
		!validRelevanceCalibrationVersion(command.SplitStrategyVersion) || command.AnnotatorCount < 2 || command.AnnotatorCount > 20 ||
		(command.AgreementMetric != "cohen_kappa" && command.AgreementMetric != "krippendorff_alpha") ||
		!validCalibrationNumber(command.AgreementScore) ||
		command.TimeBoundary.IsZero() || command.SampleWindowStart.IsZero() || command.SampleWindowEnd.IsZero() ||
		!command.TimeBoundary.Before(command.SampleWindowStart) || command.SampleWindowEnd.Before(command.SampleWindowStart) ||
		command.SampleWindowEnd.After(command.EvaluatedAt) || !validRelevanceCalibrationVersion(command.MatchingAlgorithmVersion) ||
		!validRelevanceCalibrationVersion(command.RerankerVersion) || !validRelevanceCalibrationVersion(command.CalibrationVersion) ||
		!validCalibrationNumber(command.RejectThreshold) || !validCalibrationNumber(command.AcceptThreshold) ||
		command.RejectThreshold >= command.AcceptThreshold || (command.Status != "active" && command.Status != "shadow") ||
		math.IsNaN(command.CalibrationSlope) || math.IsInf(command.CalibrationSlope, 0) ||
		math.IsNaN(command.CalibrationIntercept) || math.IsInf(command.CalibrationIntercept, 0) ||
		command.CalibrationSlope <= 0 || command.CalibrationSlope > 100 || math.Abs(command.CalibrationIntercept) > 100 ||
		command.Metrics.SampleCount <= 0 || command.Metrics.SampleCount != command.Metrics.PositiveCount+command.Metrics.NegativeCount ||
		command.EvaluatedAt.IsZero() || len(command.Slices) == 0 || len(command.Slices) > 64 ||
		(command.Status == "active" && (!command.Metrics.Passed || command.AgreementScore < .80)) {
		return false
	}
	for _, value := range []float64{command.Metrics.RecallAt100, command.Metrics.Precision, command.Metrics.Recall, command.Metrics.ECE,
		command.Metrics.Brier, command.Metrics.PrecisionWilsonLower} {
		if !validCalibrationNumber(value) {
			return false
		}
	}
	seen := map[string]struct{}{}
	for _, slice := range command.Slices {
		key := slice.Dimension + ":" + slice.Value
		if (slice.Dimension != "language" && slice.Dimension != "source") || strings.TrimSpace(slice.Value) != slice.Value || slice.Value == "" ||
			slice.SampleCount != slice.PositiveCount+slice.NegativeCount || !validCalibrationNumber(slice.Precision) || !validCalibrationNumber(slice.Recall) {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}

func validCalibrationNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func validRelevanceCalibrationVersion(value string) bool {
	if strings.TrimSpace(value) != value || value == "" || len(value) > 64 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			index > 0 && (character == '.' || character == '_' || character == ':' || character == '-') {
			continue
		}
		return false
	}
	return true
}

func relevanceEvaluationStatus(passed bool) string {
	if passed {
		return "passed"
	}
	return "failed"
}

func sameRelevanceEvaluationRun(record relevanceEvaluationRunRecord, command ingestionapplication.PersistRelevanceCalibrationCommand) bool {
	return record.ID > 0 && record.DatasetVersion == command.DatasetVersion && record.DatasetHash == command.DatasetHash &&
		record.FamilyIsolationHash == command.FamilyIsolationHash &&
		record.AnnotationProtocolVersion == command.AnnotationProtocolVersion &&
		record.AnnotationGuidelineSHA256 == command.AnnotationGuidelineSHA256 &&
		record.SplitStrategyVersion == command.SplitStrategyVersion && record.AnnotatorCount == command.AnnotatorCount &&
		record.AgreementMetric == command.AgreementMetric && record.AgreementScore == roundCalibrationMetric(command.AgreementScore) &&
		record.TimeBoundary.Equal(command.TimeBoundary) &&
		record.SampleWindowStart.Equal(command.SampleWindowStart) && record.SampleWindowEnd.Equal(command.SampleWindowEnd) &&
		record.MatchingAlgorithmVersion == command.MatchingAlgorithmVersion && record.RerankerVersion == command.RerankerVersion &&
		record.CalibrationVersion == command.CalibrationVersion && record.RejectThreshold == command.RejectThreshold &&
		record.AcceptThreshold == command.AcceptThreshold && record.CalibrationSlope == command.CalibrationSlope &&
		record.CalibrationIntercept == command.CalibrationIntercept && record.SampleCount == command.Metrics.SampleCount &&
		record.PositiveCount == command.Metrics.PositiveCount && record.NegativeCount == command.Metrics.NegativeCount &&
		record.RecallAt100 == roundCalibrationMetric(command.Metrics.RecallAt100) && record.Precision == roundCalibrationMetric(command.Metrics.Precision) &&
		record.Recall == roundCalibrationMetric(command.Metrics.Recall) && record.ECE == roundCalibrationMetric(command.Metrics.ECE) &&
		record.Brier == roundCalibrationMetric(command.Metrics.Brier) && record.PrecisionWilsonLower == roundCalibrationMetric(command.Metrics.PrecisionWilsonLower) &&
		record.HardNegativeCount == command.Metrics.HardNegativeCount && record.HardNegativePassed == command.Metrics.HardNegativePassed &&
		record.Status == relevanceEvaluationStatus(command.Metrics.Passed) && record.EvaluatedByUserID == command.ActorUserID &&
		record.EvaluatedAt.Equal(command.EvaluatedAt)
}

func relevanceEvaluationSliceRecords(values []ingestionapplication.RelevanceEvaluationSliceResultDTO) []relevanceEvaluationSliceRecord {
	result := make([]relevanceEvaluationSliceRecord, len(values))
	for index, value := range values {
		result[index] = relevanceEvaluationSliceRecord{Dimension: value.Dimension, Value: value.Value, SampleCount: value.SampleCount,
			PositiveCount: value.PositiveCount, NegativeCount: value.NegativeCount,
			Precision: roundCalibrationMetric(value.Precision), Recall: roundCalibrationMetric(value.Recall), Passed: value.Passed}
	}
	return result
}

func roundCalibrationMetric(value float64) float64 { return math.Round(value*1e7) / 1e7 }

func relevanceCalibrationProfileDTO(record relevanceCalibrationProfileRecord) ingestionapplication.RelevanceCalibrationProfileDTO {
	return ingestionapplication.RelevanceCalibrationProfileDTO{
		ID: record.ID, Version: record.Version, EvaluationRunID: record.EvaluationRunID,
		MatchingAlgorithmVersion: record.MatchingAlgorithmVersion, RerankerVersion: record.RerankerVersion,
		CalibrationVersion: record.CalibrationVersion, Status: record.Status,
		RejectThreshold: record.RejectThreshold, AcceptThreshold: record.AcceptThreshold,
		CalibrationSlope: record.CalibrationSlope, CalibrationIntercept: record.CalibrationIntercept,
		EvaluationSampleCount: record.EvaluationSampleCount,
	}
}
