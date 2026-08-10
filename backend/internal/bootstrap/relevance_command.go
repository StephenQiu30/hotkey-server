package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedclock "github.com/StephenQiu30/hotkey-server/backend/internal/shared/clock"
)

const maximumRelevanceDatasetBytes = 64 << 20

type relevanceEvaluationDatasetRequest struct {
	ProfileName               string                             `json:"profile_name"`
	DatasetVersion            string                             `json:"dataset_version"`
	FamilyIsolationHash       string                             `json:"family_isolation_hash"`
	AnnotationProtocolVersion string                             `json:"annotation_protocol_version"`
	AnnotationGuidelineSHA256 string                             `json:"annotation_guideline_sha256"`
	SplitStrategyVersion      string                             `json:"split_strategy_version"`
	AnnotatorCount            int                                `json:"annotator_count"`
	AgreementMetric           string                             `json:"agreement_metric"`
	AgreementScore            float64                            `json:"agreement_score"`
	TimeBoundary              time.Time                          `json:"time_boundary"`
	RequiredSlices            []relevanceEvaluationSliceRequest  `json:"required_slices"`
	RejectThreshold           float64                            `json:"reject_threshold"`
	AcceptThreshold           float64                            `json:"accept_threshold"`
	Samples                   []relevanceEvaluationSampleRequest `json:"samples"`
}

type relevanceEvaluationSliceRequest struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
}

type relevanceEvaluationSampleRequest struct {
	SampleID          string                              `json:"sample_id"`
	ContentFamilyHash string                              `json:"content_family_hash"`
	ObservedAt        time.Time                           `json:"observed_at"`
	Language          string                              `json:"language"`
	SourceType        string                              `json:"source_type"`
	Relevant          bool                                `json:"relevant"`
	RetrievedAt100    bool                                `json:"retrieved_at_100"`
	HardNegative      bool                                `json:"hard_negative"`
	Candidate         relevanceEvaluationCandidateRequest `json:"candidate"`
}

type relevanceEvaluationCandidateRequest struct {
	DocumentVersionID int64                                    `json:"document_version_id"`
	RRFScore          float64                                  `json:"rrf_score"`
	Signals           []relevanceEvaluationRecallSignalRequest `json:"signals"`
}

type relevanceEvaluationRecallSignalRequest struct {
	Channel          string  `json:"channel"`
	Rank             int     `json:"rank"`
	RawScore         float64 `json:"raw_score"`
	AlgorithmVersion string  `json:"algorithm_version"`
}

type relevanceEvaluationCommandResponse struct {
	ProfileID                int64   `json:"profile_id"`
	EvaluationRunID          int64   `json:"evaluation_run_id"`
	ProfileStatus            string  `json:"profile_status"`
	CalibrationVersion       string  `json:"calibration_version"`
	Activated                bool    `json:"activated"`
	CalibrationSlope         float64 `json:"calibration_slope"`
	CalibrationIntercept     float64 `json:"calibration_intercept"`
	SampleCount              int     `json:"sample_count"`
	PositiveCount            int     `json:"positive_count"`
	NegativeCount            int     `json:"negative_count"`
	RecallAt100              float64 `json:"recall_at_100"`
	Precision                float64 `json:"precision"`
	Recall                   float64 `json:"recall"`
	ExpectedCalibrationError float64 `json:"expected_calibration_error"`
	BrierScore               float64 `json:"brier_score"`
	PrecisionWilsonLower     float64 `json:"precision_wilson_lower"`
	HardNegativePassed       bool    `json:"hard_negative_passed"`
}

func runRelevanceCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer) error {
	if err := cfg.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate relevance command configuration: %w", err)
	}
	if len(args) == 0 || args[0] != "evaluate" {
		return errors.New("relevance command is required: expected evaluate")
	}
	flags := flag.NewFlagSet("hotkey relevance evaluate", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	datasetPath := flags.String("dataset", "", "time-isolated relevance evaluation dataset JSON")
	actorUserID := flags.Int64("actor-user-id", 0, "active administrator user ID")
	activate := flags.Bool("activate", false, "activate the profile only when every quality gate passes")
	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("parse relevance evaluate flags: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected relevance evaluate arguments: %v", flags.Args())
	}
	if strings.TrimSpace(*datasetPath) == "" || *actorUserID <= 0 || output == nil {
		return errors.New("relevance evaluate requires --dataset and a positive --actor-user-id")
	}
	dataset, err := readRelevanceEvaluationDataset(*datasetPath, *actorUserID, *activate)
	if err != nil {
		return err
	}
	runtime, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	repository, err := ingestionpostgres.NewDocumentMatchRepository(runtime)
	if err != nil {
		return err
	}
	reranker := ingestionapplication.NewRankSignalDocumentMatchReranker()
	service, err := ingestionapplication.NewRelevanceCalibrationService(reranker, repository, sharedclock.System{})
	if err != nil {
		return err
	}
	result, err := service.Evaluate(ctx, dataset)
	if err != nil {
		return err
	}
	response := relevanceEvaluationCommandResponse{
		ProfileID: result.Profile.ID, EvaluationRunID: result.Profile.EvaluationRunID,
		ProfileStatus: result.Profile.Status, CalibrationVersion: result.Profile.CalibrationVersion, Activated: result.Activated,
		CalibrationSlope: result.Profile.CalibrationSlope, CalibrationIntercept: result.Profile.CalibrationIntercept,
		SampleCount: result.Metrics.SampleCount, PositiveCount: result.Metrics.PositiveCount,
		NegativeCount: result.Metrics.NegativeCount, RecallAt100: result.Metrics.RecallAt100,
		Precision: result.Metrics.Precision, Recall: result.Metrics.Recall,
		ExpectedCalibrationError: result.Metrics.ECE, BrierScore: result.Metrics.Brier,
		PrecisionWilsonLower: result.Metrics.PrecisionWilsonLower,
		HardNegativePassed:   result.Metrics.HardNegativePassed,
	}
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(true)
	return encoder.Encode(response)
}

func readRelevanceEvaluationDataset(path string, actorUserID int64, activate bool) (ingestionapplication.EvaluateRelevanceCalibrationCommand, error) {
	file, err := os.Open(path)
	if err != nil {
		return ingestionapplication.EvaluateRelevanceCalibrationCommand{}, fmt.Errorf("open relevance dataset: %w", err)
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, maximumRelevanceDatasetBytes+1))
	decoder.DisallowUnknownFields()
	var request relevanceEvaluationDatasetRequest
	if err := decoder.Decode(&request); err != nil {
		return ingestionapplication.EvaluateRelevanceCalibrationCommand{}, fmt.Errorf("decode relevance dataset: %w", err)
	}
	if err := ensureRelevanceDatasetEOF(decoder); err != nil {
		return ingestionapplication.EvaluateRelevanceCalibrationCommand{}, err
	}
	if position, err := file.Seek(0, io.SeekCurrent); err == nil && position > maximumRelevanceDatasetBytes {
		return ingestionapplication.EvaluateRelevanceCalibrationCommand{}, errors.New("relevance dataset exceeds 64 MiB")
	}
	command := ingestionapplication.EvaluateRelevanceCalibrationCommand{
		ActorUserID: actorUserID, ProfileName: request.ProfileName, DatasetVersion: request.DatasetVersion,
		FamilyIsolationHash: request.FamilyIsolationHash, TimeBoundary: request.TimeBoundary,
		AnnotationProtocolVersion: request.AnnotationProtocolVersion,
		AnnotationGuidelineSHA256: request.AnnotationGuidelineSHA256,
		SplitStrategyVersion:      request.SplitStrategyVersion, AnnotatorCount: request.AnnotatorCount,
		AgreementMetric: request.AgreementMetric, AgreementScore: request.AgreementScore,
		RejectThreshold: request.RejectThreshold, AcceptThreshold: request.AcceptThreshold, Activate: activate,
		RequiredSlices: make([]ingestionapplication.RelevanceEvaluationSliceDTO, len(request.RequiredSlices)),
		Samples:        make([]ingestionapplication.RelevanceEvaluationSampleDTO, len(request.Samples)),
	}
	for index, value := range request.RequiredSlices {
		command.RequiredSlices[index] = ingestionapplication.RelevanceEvaluationSliceDTO{Dimension: value.Dimension, Value: value.Value}
	}
	for index, value := range request.Samples {
		signals := make([]ingestionapplication.RecallSignalDTO, len(value.Candidate.Signals))
		for signalIndex, signal := range value.Candidate.Signals {
			signals[signalIndex] = ingestionapplication.RecallSignalDTO{Channel: signal.Channel, Rank: signal.Rank, RawScore: signal.RawScore, AlgorithmVersion: signal.AlgorithmVersion}
		}
		command.Samples[index] = ingestionapplication.RelevanceEvaluationSampleDTO{
			SampleID: value.SampleID, ContentFamilyHash: value.ContentFamilyHash, ObservedAt: value.ObservedAt,
			Language: value.Language, SourceType: value.SourceType, Relevant: value.Relevant,
			RetrievedAt100: value.RetrievedAt100, HardNegative: value.HardNegative,
			Candidate: ingestionapplication.HybridRecallCandidateDTO{
				DocumentVersionID: value.Candidate.DocumentVersionID, RRFScore: value.Candidate.RRFScore, Signals: signals,
			},
		}
	}
	return command, nil
}

func ensureRelevanceDatasetEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode relevance dataset trailing data: %w", err)
	}
	return errors.New("relevance dataset contains more than one JSON value")
}
