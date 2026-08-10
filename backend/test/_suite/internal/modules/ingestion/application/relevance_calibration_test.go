package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRelevanceCalibrationEvaluatesTimeIsolatedDatasetAndActivatesPassingProfile(t *testing.T) {
	t.Parallel()

	repository := &relevanceCalibrationRepositoryFake{}
	service, err := NewRelevanceCalibrationService(NewRankSignalDocumentMatchReranker(), repository, fixedDocumentMatchClock{
		now: time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	command := passingRelevanceCalibrationCommand(200, 200)
	result, err := service.Evaluate(context.Background(), command)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Activated || result.Profile.Status != "active" || result.Profile.EvaluationRunID <= 0 ||
		result.Profile.CalibrationSlope <= 0 || result.Profile.CalibrationSlope != repository.command.CalibrationSlope ||
		(result.Profile.CalibrationSlope == 1 && result.Profile.CalibrationIntercept == 0) ||
		result.Metrics.PositiveCount != 200 || result.Metrics.NegativeCount != 200 || result.Metrics.SampleCount != 400 ||
		result.Metrics.RecallAt100 < .95 || result.Metrics.Precision < .90 || result.Metrics.Recall < .80 ||
		result.Metrics.ECE > .05 || result.Metrics.PrecisionWilsonLower < .87 || !result.Metrics.HardNegativePassed {
		t.Fatalf("calibration result = %#v", result)
	}
	if len(repository.command.Slices) != 3 || repository.command.DatasetHash == "" || repository.command.FamilyIsolationHash != command.FamilyIsolationHash ||
		repository.command.RerankerVersion != CanonicalDocumentMatchRerankerVersion ||
		!strings.HasPrefix(repository.command.CalibrationVersion, CanonicalDocumentMatchCalibrationVersion+":") {
		t.Fatalf("persisted calibration = %#v", repository.command)
	}
}

func TestRelevanceCalibrationKeepsInsufficientOrUnsafeDatasetInShadow(t *testing.T) {
	t.Parallel()

	for name, command := range map[string]EvaluateRelevanceCalibrationCommand{
		"positive sample floor": passingRelevanceCalibrationCommand(199, 200),
		"annotation agreement below gate": func() EvaluateRelevanceCalibrationCommand {
			value := passingRelevanceCalibrationCommand(200, 200)
			value.AgreementScore = .79
			return value
		}(),
		"hard negative false positive": func() EvaluateRelevanceCalibrationCommand {
			value := passingRelevanceCalibrationCommand(200, 200)
			last := len(value.Samples) - 1
			documentVersionID := value.Samples[last].Candidate.DocumentVersionID
			value.Samples[last].Candidate = value.Samples[0].Candidate
			value.Samples[last].Candidate.DocumentVersionID = documentVersionID
			value.Samples[last].HardNegative = true
			return value
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			repository := &relevanceCalibrationRepositoryFake{}
			service, _ := NewRelevanceCalibrationService(NewRankSignalDocumentMatchReranker(), repository, fixedDocumentMatchClock{now: time.Now().UTC()})
			result, err := service.Evaluate(context.Background(), command)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if result.Activated || result.Profile.Status != "shadow" || repository.command.Status != "shadow" {
				t.Fatalf("unsafe dataset activated = %#v", result)
			}
		})
	}
}

func TestRelevanceCalibrationRejectsFamilyLeakageBeforePersistence(t *testing.T) {
	t.Parallel()

	command := passingRelevanceCalibrationCommand(200, 200)
	command.Samples[1].ContentFamilyHash = command.Samples[0].ContentFamilyHash
	repository := &relevanceCalibrationRepositoryFake{}
	service, _ := NewRelevanceCalibrationService(NewRankSignalDocumentMatchReranker(), repository, fixedDocumentMatchClock{now: time.Now().UTC()})
	if _, err := service.Evaluate(context.Background(), command); !errors.Is(err, ErrInvalidDocumentMatchContract) || repository.calls != 0 {
		t.Fatalf("family leakage error/calls = %v/%d", err, repository.calls)
	}
}

func TestRelevanceCalibrationRejectsFutureSamplesOrMissingSafetyDimension(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*EvaluateRelevanceCalibrationCommand){
		"future sample": func(value *EvaluateRelevanceCalibrationCommand) {
			value.Samples[len(value.Samples)-1].ObservedAt = time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
		},
		"missing source slice": func(value *EvaluateRelevanceCalibrationCommand) {
			value.RequiredSlices = value.RequiredSlices[:2]
		},
	} {
		t.Run(name, func(t *testing.T) {
			command := passingRelevanceCalibrationCommand(200, 200)
			mutate(&command)
			repository := &relevanceCalibrationRepositoryFake{}
			service, _ := NewRelevanceCalibrationService(NewRankSignalDocumentMatchReranker(), repository, fixedDocumentMatchClock{
				now: time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC),
			})
			if _, err := service.Evaluate(context.Background(), command); !errors.Is(err, ErrInvalidDocumentMatchContract) || repository.calls != 0 {
				t.Fatalf("unsafe evaluation error/calls = %v/%d", err, repository.calls)
			}
		})
	}
}

func passingRelevanceCalibrationCommand(positiveCount, negativeCount int) EvaluateRelevanceCalibrationCommand {
	boundary := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	samples := make([]RelevanceEvaluationSampleDTO, 0, positiveCount+negativeCount)
	for index := 0; index < positiveCount+negativeCount; index++ {
		positive := index < positiveCount
		language := "en"
		if index%2 == 1 {
			language = "zh"
		}
		candidate := HybridRecallCandidateDTO{DocumentVersionID: int64(index + 1), RRFScore: .04}
		if positive {
			candidate.Signals = []RecallSignalDTO{
				{Channel: "lexical", Rank: 1, RawScore: .9, AlgorithmVersion: LexicalRecallAlgorithmVersion},
				{Channel: "semantic", Rank: 1, RawScore: .9, AlgorithmVersion: SemanticRecallAlgorithmVersion},
				{Channel: "structured", Rank: 1, RawScore: 3, AlgorithmVersion: StructuredRecallAlgorithmVersion},
			}
		} else {
			candidate.Signals = []RecallSignalDTO{{Channel: "lexical", Rank: 100, RawScore: .01, AlgorithmVersion: LexicalRecallAlgorithmVersion}}
		}
		samples = append(samples, RelevanceEvaluationSampleDTO{
			SampleID: fmt.Sprintf("sample-%04d", index+1), ContentFamilyHash: fmt.Sprintf("%064x", index+1),
			ObservedAt: boundary.Add(time.Duration(index+1) * time.Minute), Language: language, SourceType: "rss",
			Relevant: positive, RetrievedAt100: true, HardNegative: !positive, Candidate: candidate,
		})
	}
	return EvaluateRelevanceCalibrationCommand{
		ActorUserID: 7, ProfileName: "time split relevance 2026-08", DatasetVersion: "relevance-time-split-2026-08-v1",
		FamilyIsolationHash: fmt.Sprintf("%x", [32]byte{1}), TimeBoundary: boundary,
		AnnotationProtocolVersion: "dual-review-relevance-v1", AnnotationGuidelineSHA256: strings.Repeat("a", 64),
		SplitStrategyVersion: "time-family-event-isolated-v1", AnnotatorCount: 2,
		AgreementMetric: "cohen_kappa", AgreementScore: .96,
		RequiredSlices:  []RelevanceEvaluationSliceDTO{{Dimension: "language", Value: "en"}, {Dimension: "language", Value: "zh"}, {Dimension: "source", Value: "rss"}},
		RejectThreshold: .4, AcceptThreshold: .8, Activate: true, Samples: samples,
	}
}

type relevanceCalibrationRepositoryFake struct {
	command PersistRelevanceCalibrationCommand
	calls   int
}

func (repository *relevanceCalibrationRepositoryFake) PersistRelevanceCalibration(_ context.Context, command PersistRelevanceCalibrationCommand) (RelevanceCalibrationProfileDTO, error) {
	repository.calls++
	repository.command = command
	return RelevanceCalibrationProfileDTO{
		ID: 91, Version: 1, EvaluationRunID: 81, Status: command.Status,
		MatchingAlgorithmVersion: command.MatchingAlgorithmVersion, RerankerVersion: command.RerankerVersion,
		CalibrationVersion: command.CalibrationVersion, RejectThreshold: command.RejectThreshold,
		AcceptThreshold: command.AcceptThreshold, CalibrationSlope: command.CalibrationSlope,
		CalibrationIntercept: command.CalibrationIntercept, EvaluationSampleCount: command.Metrics.SampleCount,
	}, nil
}
