package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	sharedclock "github.com/StephenQiu30/hotkey-server/backend/internal/shared/clock"
)

var ErrInvalidDecisionQualityProfileContract = errors.New("decision quality profile contract is invalid")

type PersistDecisionQualityEvaluationCommand struct {
	ActorUserID int64
	Activate    bool
	EvaluatedAt time.Time
	Dataset     DecisionQualityDatasetMetadataDTO
	Metrics     []DecisionQualityMetricDTO
	Slices      []DecisionQualitySliceDTO
}

type DecisionQualityDatasetMetadataDTO struct {
	DatasetVersion, DatasetSHA256, AnnotationProtocolVersion, AnnotationGuidelineSHA256 string
	SplitStrategyVersion, FamilyIsolationSHA256, EventIsolationSHA256                   string
	AnnotatorCount                                                                      int
	AgreementMetric                                                                     string
	AgreementScore                                                                      float64
	TimeBoundary                                                                        time.Time
}

type DecisionQualityEvaluationRunDTO struct {
	ID                int64     `json:"id"`
	Version           int64     `json:"version"`
	Module            string    `json:"module"`
	ProfileVersion    string    `json:"profile_version"`
	DatasetVersion    string    `json:"dataset_version"`
	DatasetSHA256     string    `json:"dataset_sha256"`
	SampleCount       int       `json:"sample_count"`
	PositiveCount     int       `json:"positive_count"`
	NegativeCount     int       `json:"negative_count"`
	Passed            bool      `json:"passed"`
	EvaluatedByUserID int64     `json:"evaluated_by_user_id"`
	EvaluatedAt       time.Time `json:"evaluated_at"`
}

type DecisionQualityProfileDTO struct {
	ID                int64      `json:"id"`
	Version           int64      `json:"version"`
	EvaluationRunID   int64      `json:"evaluation_run_id"`
	Module            string     `json:"module"`
	ProfileVersion    string     `json:"profile_version"`
	Status            string     `json:"status"`
	ActivatedByUserID *int64     `json:"activated_by_user_id,omitempty"`
	ActivatedAt       *time.Time `json:"activated_at,omitempty"`
}

type PersistDecisionQualityEvaluationResult struct {
	Runs     []DecisionQualityEvaluationRunDTO `json:"runs"`
	Profiles []DecisionQualityProfileDTO       `json:"profiles"`
	Reused   bool                              `json:"reused"`
}

type DecisionQualityRepository interface {
	PersistDecisionQualityEvaluation(context.Context, PersistDecisionQualityEvaluationCommand) (PersistDecisionQualityEvaluationResult, error)
}

type DecisionQualityService struct {
	repository DecisionQualityRepository
	clock      sharedclock.Clock
}

func NewDecisionQualityService(repository DecisionQualityRepository, clock sharedclock.Clock) (*DecisionQualityService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidDecisionQualityProfileContract)
	}
	if clock == nil {
		clock = sharedclock.System{}
	}
	return &DecisionQualityService{repository: repository, clock: clock}, nil
}

func (service *DecisionQualityService) Evaluate(ctx context.Context, dataset DecisionQualityDatasetDTO, actorUserID int64, activate bool) (DecisionQualityEvaluationResult, PersistDecisionQualityEvaluationResult, error) {
	if service == nil || service.repository == nil || service.clock == nil || actorUserID <= 0 {
		return DecisionQualityEvaluationResult{}, PersistDecisionQualityEvaluationResult{}, ErrInvalidDecisionQualityProfileContract
	}
	evaluation, err := EvaluateDecisionQuality(dataset)
	if err != nil {
		return DecisionQualityEvaluationResult{}, PersistDecisionQualityEvaluationResult{}, err
	}
	evaluatedAt := service.clock.Now().UTC()
	if evaluatedAt.IsZero() {
		return DecisionQualityEvaluationResult{}, PersistDecisionQualityEvaluationResult{}, ErrInvalidDecisionQualityProfileContract
	}
	command := PersistDecisionQualityEvaluationCommand{ActorUserID: actorUserID, Activate: activate,
		EvaluatedAt: evaluatedAt, Metrics: cloneDecisionQualityMetrics(evaluation.Metrics), Slices: cloneDecisionQualitySlices(evaluation.Slices),
		Dataset: DecisionQualityDatasetMetadataDTO{DatasetVersion: dataset.DatasetVersion, DatasetSHA256: dataset.DatasetSHA256,
			AnnotationProtocolVersion: dataset.AnnotationProtocolVersion, AnnotationGuidelineSHA256: dataset.AnnotationGuidelineSHA256,
			SplitStrategyVersion: dataset.SplitStrategyVersion, FamilyIsolationSHA256: dataset.FamilyIsolationSHA256,
			EventIsolationSHA256: dataset.EventIsolationSHA256, AnnotatorCount: dataset.AnnotatorCount,
			AgreementMetric: dataset.AgreementMetric, AgreementScore: dataset.AgreementScore, TimeBoundary: dataset.TimeBoundary.UTC()}}
	if activate && !evaluation.AllRequiredGatesPassed {
		return DecisionQualityEvaluationResult{}, PersistDecisionQualityEvaluationResult{}, fmt.Errorf("%w: failed quality gates cannot be activated", ErrInvalidDecisionQualityProfileContract)
	}
	persisted, err := service.repository.PersistDecisionQualityEvaluation(ctx, command)
	if err != nil {
		return DecisionQualityEvaluationResult{}, PersistDecisionQualityEvaluationResult{}, err
	}
	if !decisionQualityPersistenceReceiptMatches(persisted, command) {
		return DecisionQualityEvaluationResult{}, PersistDecisionQualityEvaluationResult{}, fmt.Errorf("%w: persistence receipt changed", ErrInvalidDecisionQualityProfileContract)
	}
	return evaluation, persisted, nil
}

func decisionQualityPersistenceReceiptMatches(value PersistDecisionQualityEvaluationResult, command PersistDecisionQualityEvaluationCommand) bool {
	if len(value.Runs) != len(command.Metrics) || len(value.Profiles) != len(command.Metrics) {
		return false
	}
	metrics := make(map[string]DecisionQualityMetricDTO, len(command.Metrics))
	for _, metric := range command.Metrics {
		metrics[metric.Module+"\x00"+metric.ProfileVersion] = metric
	}
	for _, run := range value.Runs {
		metric, found := metrics[run.Module+"\x00"+run.ProfileVersion]
		if !found || run.ID <= 0 || run.Version <= 0 || run.DatasetVersion != command.Dataset.DatasetVersion ||
			run.DatasetSHA256 != command.Dataset.DatasetSHA256 || run.SampleCount != metric.SampleCount ||
			run.PositiveCount != metric.PositiveCount || run.NegativeCount != metric.NegativeCount || run.Passed != metric.Passed ||
			run.EvaluatedByUserID != command.ActorUserID || run.EvaluatedAt.After(command.EvaluatedAt) ||
			(!value.Reused && !run.EvaluatedAt.Equal(command.EvaluatedAt)) {
			return false
		}
	}
	for _, profile := range value.Profiles {
		metric, found := metrics[profile.Module+"\x00"+profile.ProfileVersion]
		if !found || profile.ID <= 0 || profile.Version <= 0 || profile.EvaluationRunID <= 0 {
			return false
		}
		if profile.Status != "shadow" && profile.Status != "active" || command.Activate && metric.Passed && profile.Status != "active" {
			return false
		}
		if profile.Status == "active" && (profile.ActivatedByUserID == nil || *profile.ActivatedByUserID != command.ActorUserID ||
			profile.ActivatedAt == nil || profile.ActivatedAt.After(command.EvaluatedAt) ||
			(!value.Reused && !profile.ActivatedAt.Equal(command.EvaluatedAt))) {
			return false
		}
	}
	return true
}

func cloneDecisionQualityMetrics(values []DecisionQualityMetricDTO) []DecisionQualityMetricDTO {
	result := append([]DecisionQualityMetricDTO(nil), values...)
	for index := range result {
		result[index].ReasonCodes = append([]string(nil), values[index].ReasonCodes...)
	}
	return result
}

func cloneDecisionQualitySlices(values []DecisionQualitySliceDTO) []DecisionQualitySliceDTO {
	return append([]DecisionQualitySliceDTO(nil), values...)
}
