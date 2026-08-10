package application

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MinimumRelevancePositiveSamples = 200
	MinimumRelevanceNegativeSamples = 200
	MinimumRelevanceSliceSamples    = 50
)

type RelevanceEvaluationSampleDTO struct {
	SampleID, ContentFamilyHash string
	ObservedAt                  time.Time
	Language, SourceType        string
	Relevant                    bool
	RetrievedAt100              bool
	HardNegative                bool
	Candidate                   HybridRecallCandidateDTO
}

type RelevanceEvaluationSliceDTO struct{ Dimension, Value string }

type RelevanceEvaluationSliceResultDTO struct {
	Dimension, Value                          string
	SampleCount, PositiveCount, NegativeCount int
	Precision, Recall                         float64
	Passed                                    bool
}

type RelevanceEvaluationMetricsDTO struct {
	SampleCount, PositiveCount, NegativeCount int
	RecallAt100, Precision, Recall            float64
	ECE, Brier, PrecisionWilsonLower          float64
	HardNegativeCount                         int
	HardNegativePassed                        bool
	Passed                                    bool
}

type EvaluateRelevanceCalibrationCommand struct {
	ActorUserID                                                                int64
	ProfileName                                                                string
	DatasetVersion                                                             string
	FamilyIsolationHash                                                        string
	AnnotationProtocolVersion, AnnotationGuidelineSHA256, SplitStrategyVersion string
	AnnotatorCount                                                             int
	AgreementMetric                                                            string
	AgreementScore                                                             float64
	TimeBoundary                                                               time.Time
	RequiredSlices                                                             []RelevanceEvaluationSliceDTO
	RejectThreshold                                                            float64
	AcceptThreshold                                                            float64
	Activate                                                                   bool
	Samples                                                                    []RelevanceEvaluationSampleDTO
}

type PersistRelevanceCalibrationCommand struct {
	ActorUserID                                                   int64
	ProfileName                                                   string
	DatasetVersion                                                string
	DatasetHash                                                   string
	FamilyIsolationHash                                           string
	AnnotationProtocolVersion, AnnotationGuidelineSHA256          string
	SplitStrategyVersion, AgreementMetric                         string
	AnnotatorCount                                                int
	AgreementScore                                                float64
	TimeBoundary                                                  time.Time
	SampleWindowStart, SampleWindowEnd                            time.Time
	MatchingAlgorithmVersion, RerankerVersion, CalibrationVersion string
	RejectThreshold, AcceptThreshold                              float64
	CalibrationSlope, CalibrationIntercept                        float64
	Status                                                        string
	Metrics                                                       RelevanceEvaluationMetricsDTO
	Slices                                                        []RelevanceEvaluationSliceResultDTO
	EvaluatedAt                                                   time.Time
}

type RelevanceCalibrationProfileDTO struct {
	ID, Version, EvaluationRunID                                  int64
	MatchingAlgorithmVersion, RerankerVersion, CalibrationVersion string
	Status                                                        string
	RejectThreshold, AcceptThreshold                              float64
	CalibrationSlope, CalibrationIntercept                        float64
	EvaluationSampleCount                                         int
}

type EvaluateRelevanceCalibrationResult struct {
	Profile   RelevanceCalibrationProfileDTO
	Metrics   RelevanceEvaluationMetricsDTO
	Slices    []RelevanceEvaluationSliceResultDTO
	Activated bool
}

type RelevanceCalibrationRepository interface {
	PersistRelevanceCalibration(context.Context, PersistRelevanceCalibrationCommand) (RelevanceCalibrationProfileDTO, error)
}

type RelevanceCalibrationService struct {
	reranker   DocumentMatchReranker
	repository RelevanceCalibrationRepository
	clock      DocumentMatchClock
}

func NewRelevanceCalibrationService(reranker DocumentMatchReranker, repository RelevanceCalibrationRepository, clock DocumentMatchClock) (*RelevanceCalibrationService, error) {
	if reranker == nil || repository == nil || clock == nil {
		return nil, ErrInvalidDocumentMatchContract
	}
	return &RelevanceCalibrationService{reranker: reranker, repository: repository, clock: clock}, nil
}

func (service *RelevanceCalibrationService) Evaluate(ctx context.Context, command EvaluateRelevanceCalibrationCommand) (EvaluateRelevanceCalibrationResult, error) {
	prepared, err := prepareRelevanceCalibration(command)
	if err != nil {
		return EvaluateRelevanceCalibrationResult{}, err
	}
	evaluatedAt := service.clock.Now().UTC().Truncate(time.Microsecond)
	windowStart, windowEnd := relevanceEvaluationWindow(prepared.Samples)
	if evaluatedAt.IsZero() || windowStart.IsZero() || windowEnd.IsZero() || !prepared.TimeBoundary.Before(windowStart) || windowEnd.After(evaluatedAt) {
		return EvaluateRelevanceCalibrationResult{}, ErrInvalidDocumentMatchContract
	}
	baseLogits := make(map[string]float64, len(prepared.Samples))
	for _, sample := range prepared.Samples {
		if !sample.RetrievedAt100 {
			continue
		}
		values, err := service.reranker.RerankDocumentMatches(ctx, RerankDocumentMatchesQuery{
			MonitorID: 1, MonitorVersionID: 1, CompiledProfileID: 1, RelevanceProfileID: 1,
			MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
			RerankerVersion:          CanonicalDocumentMatchRerankerVersion, CalibrationVersion: CanonicalDocumentMatchCalibrationVersion,
			CalibrationSlope: 1, CalibrationIntercept: 0,
			Candidates: []HybridRecallCandidateDTO{cloneHybridRecallCandidates([]HybridRecallCandidateDTO{sample.Candidate})[0]},
		})
		if err != nil || len(values) != 1 || values[0].DocumentVersionID != sample.Candidate.DocumentVersionID {
			return EvaluateRelevanceCalibrationResult{}, ErrInvalidDocumentMatchContract
		}
		probability := values[0].RelevanceProbability
		baseLogits[sample.SampleID] = math.Log(probability / (1 - probability))
	}
	slope, intercept, err := fitPlattCalibration(prepared.Samples, baseLogits)
	if err != nil {
		return EvaluateRelevanceCalibrationResult{}, err
	}
	probabilities := make(map[string]float64, len(prepared.Samples))
	for _, sample := range prepared.Samples {
		if !sample.RetrievedAt100 {
			probabilities[sample.SampleID] = 0
			continue
		}
		probabilities[sample.SampleID] = logisticProbability(slope*baseLogits[sample.SampleID] + intercept)
	}
	metrics := calculateRelevanceMetrics(prepared.Samples, probabilities, prepared.AcceptThreshold)
	slices := calculateRelevanceSlices(prepared, probabilities)
	metrics.Passed = prepared.AgreementScore >= .80 && relevanceMetricsPass(metrics, slices)
	status := "shadow"
	if prepared.Activate && metrics.Passed {
		status = "active"
	}
	datasetHash := relevanceDatasetHash(prepared)
	persist := PersistRelevanceCalibrationCommand{
		ActorUserID: prepared.ActorUserID, ProfileName: prepared.ProfileName,
		DatasetVersion: prepared.DatasetVersion, DatasetHash: datasetHash,
		FamilyIsolationHash: prepared.FamilyIsolationHash, TimeBoundary: prepared.TimeBoundary,
		AnnotationProtocolVersion: prepared.AnnotationProtocolVersion,
		AnnotationGuidelineSHA256: prepared.AnnotationGuidelineSHA256,
		SplitStrategyVersion:      prepared.SplitStrategyVersion, AnnotatorCount: prepared.AnnotatorCount,
		AgreementMetric: prepared.AgreementMetric, AgreementScore: prepared.AgreementScore,
		SampleWindowStart: windowStart, SampleWindowEnd: windowEnd,
		MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		RerankerVersion:          CanonicalDocumentMatchRerankerVersion,
		CalibrationVersion:       CanonicalDocumentMatchCalibrationVersion + ":" + datasetHash[:16],
		RejectThreshold:          prepared.RejectThreshold, AcceptThreshold: prepared.AcceptThreshold,
		CalibrationSlope: slope, CalibrationIntercept: intercept,
		Status: status, Metrics: metrics, Slices: slices, EvaluatedAt: evaluatedAt,
	}
	profile, err := service.repository.PersistRelevanceCalibration(ctx, persist)
	if err != nil {
		return EvaluateRelevanceCalibrationResult{}, err
	}
	if profile.ID <= 0 || profile.Version != 1 || profile.EvaluationRunID <= 0 || profile.Status != status ||
		profile.MatchingAlgorithmVersion != persist.MatchingAlgorithmVersion || profile.RerankerVersion != persist.RerankerVersion ||
		profile.CalibrationVersion != persist.CalibrationVersion || profile.RejectThreshold != persist.RejectThreshold ||
		profile.AcceptThreshold != persist.AcceptThreshold || profile.CalibrationSlope != persist.CalibrationSlope ||
		profile.CalibrationIntercept != persist.CalibrationIntercept || profile.EvaluationSampleCount != metrics.SampleCount {
		return EvaluateRelevanceCalibrationResult{}, ErrInvalidDocumentMatchContract
	}
	return EvaluateRelevanceCalibrationResult{Profile: profile, Metrics: metrics, Slices: slices, Activated: status == "active"}, nil
}

func relevanceEvaluationWindow(samples []RelevanceEvaluationSampleDTO) (time.Time, time.Time) {
	var start, end time.Time
	for _, sample := range samples {
		if start.IsZero() || sample.ObservedAt.Before(start) {
			start = sample.ObservedAt
		}
		if end.IsZero() || sample.ObservedAt.After(end) {
			end = sample.ObservedAt
		}
	}
	return start, end
}

func fitPlattCalibration(samples []RelevanceEvaluationSampleDTO, logits map[string]float64) (float64, float64, error) {
	slope, intercept := 1.0, 0.0
	const regularization = 1.0
	for iteration := 0; iteration < 50; iteration++ {
		gradientSlope, gradientIntercept := regularization*(slope-1), regularization*intercept
		hessianSlope, hessianCross, hessianIntercept := regularization, 0.0, regularization
		count := 0
		for _, sample := range samples {
			if !sample.RetrievedAt100 {
				continue
			}
			logit, found := logits[sample.SampleID]
			if !found || math.IsNaN(logit) || math.IsInf(logit, 0) {
				return 0, 0, ErrInvalidDocumentMatchContract
			}
			target := 0.0
			if sample.Relevant {
				target = 1
			}
			probability := logisticProbability(slope*logit + intercept)
			weight := probability * (1 - probability)
			gradientSlope += (probability - target) * logit
			gradientIntercept += probability - target
			hessianSlope += weight * logit * logit
			hessianCross += weight * logit
			hessianIntercept += weight
			count++
		}
		if count == 0 {
			return 0, 0, ErrInvalidDocumentMatchContract
		}
		determinant := hessianSlope*hessianIntercept - hessianCross*hessianCross
		if determinant <= 1e-12 || math.IsNaN(determinant) || math.IsInf(determinant, 0) {
			return 0, 0, ErrInvalidDocumentMatchContract
		}
		deltaSlope := (hessianIntercept*gradientSlope - hessianCross*gradientIntercept) / determinant
		deltaIntercept := (-hessianCross*gradientSlope + hessianSlope*gradientIntercept) / determinant
		slope -= deltaSlope
		intercept -= deltaIntercept
		if slope <= 0 {
			slope = 1e-6
		}
		if slope > 100 {
			slope = 100
		}
		intercept = math.Max(-100, math.Min(100, intercept))
		if math.Abs(deltaSlope)+math.Abs(deltaIntercept) < 1e-10 {
			break
		}
	}
	slope = math.Round(slope*1e7) / 1e7
	intercept = math.Round(intercept*1e7) / 1e7
	if slope <= 0 || slope > 100 || math.Abs(intercept) > 100 {
		return 0, 0, ErrInvalidDocumentMatchContract
	}
	return slope, intercept, nil
}

func prepareRelevanceCalibration(command EvaluateRelevanceCalibrationCommand) (EvaluateRelevanceCalibrationCommand, error) {
	if command.ActorUserID <= 0 || strings.TrimSpace(command.ProfileName) != command.ProfileName || command.ProfileName == "" || len(command.ProfileName) > 120 ||
		!semanticVersionPattern.MatchString(command.DatasetVersion) || !validLowerHexSHA256(command.FamilyIsolationHash) ||
		!semanticVersionPattern.MatchString(command.AnnotationProtocolVersion) || !validLowerHexSHA256(command.AnnotationGuidelineSHA256) ||
		!semanticVersionPattern.MatchString(command.SplitStrategyVersion) || command.AnnotatorCount < 2 || command.AnnotatorCount > 20 ||
		(command.AgreementMetric != "cohen_kappa" && command.AgreementMetric != "krippendorff_alpha") ||
		math.IsNaN(command.AgreementScore) || math.IsInf(command.AgreementScore, 0) || command.AgreementScore < 0 || command.AgreementScore > 1 || command.TimeBoundary.IsZero() ||
		math.IsNaN(command.RejectThreshold) || math.IsNaN(command.AcceptThreshold) || command.RejectThreshold < 0 || command.AcceptThreshold > 1 ||
		command.RejectThreshold >= command.AcceptThreshold || len(command.Samples) == 0 || len(command.Samples) > 100000 || len(command.RequiredSlices) == 0 || len(command.RequiredSlices) > 64 {
		return EvaluateRelevanceCalibrationCommand{}, ErrInvalidDocumentMatchContract
	}
	command.TimeBoundary = command.TimeBoundary.UTC().Truncate(time.Microsecond)
	command.Samples = append([]RelevanceEvaluationSampleDTO(nil), command.Samples...)
	command.RequiredSlices = append([]RelevanceEvaluationSliceDTO(nil), command.RequiredSlices...)
	seenSamples := make(map[string]struct{}, len(command.Samples))
	seenFamilies := make(map[string]struct{}, len(command.Samples))
	for index := range command.Samples {
		sample := &command.Samples[index]
		sample.SampleID = strings.TrimSpace(sample.SampleID)
		sample.Language = strings.ToLower(strings.TrimSpace(sample.Language))
		sample.SourceType = strings.ToLower(strings.TrimSpace(sample.SourceType))
		sample.ObservedAt = sample.ObservedAt.UTC().Truncate(time.Microsecond)
		if sample.SampleID == "" || len(sample.SampleID) > 128 || !validLowerHexSHA256(sample.ContentFamilyHash) ||
			!sample.ObservedAt.After(command.TimeBoundary) || sample.Language == "" || len(sample.Language) > 64 || sample.SourceType == "" || len(sample.SourceType) > 64 ||
			(sample.RetrievedAt100 && (sample.Candidate.DocumentVersionID <= 0 || len(sample.Candidate.Signals) == 0)) ||
			(!sample.RetrievedAt100 && len(sample.Candidate.Signals) != 0) {
			return EvaluateRelevanceCalibrationCommand{}, ErrInvalidDocumentMatchContract
		}
		if _, duplicate := seenSamples[sample.SampleID]; duplicate {
			return EvaluateRelevanceCalibrationCommand{}, ErrInvalidDocumentMatchContract
		}
		if _, leakage := seenFamilies[sample.ContentFamilyHash]; leakage {
			return EvaluateRelevanceCalibrationCommand{}, ErrInvalidDocumentMatchContract
		}
		seenSamples[sample.SampleID] = struct{}{}
		seenFamilies[sample.ContentFamilyHash] = struct{}{}
	}
	seenSlices := make(map[string]struct{}, len(command.RequiredSlices))
	requiredDimensions := make(map[string]struct{}, 2)
	for index := range command.RequiredSlices {
		slice := &command.RequiredSlices[index]
		slice.Dimension = strings.ToLower(strings.TrimSpace(slice.Dimension))
		slice.Value = strings.ToLower(strings.TrimSpace(slice.Value))
		if (slice.Dimension != "language" && slice.Dimension != "source") || slice.Value == "" || len(slice.Value) > 64 {
			return EvaluateRelevanceCalibrationCommand{}, ErrInvalidDocumentMatchContract
		}
		key := slice.Dimension + ":" + slice.Value
		if _, duplicate := seenSlices[key]; duplicate {
			return EvaluateRelevanceCalibrationCommand{}, ErrInvalidDocumentMatchContract
		}
		seenSlices[key] = struct{}{}
		requiredDimensions[slice.Dimension] = struct{}{}
	}
	if _, found := requiredDimensions["language"]; !found {
		return EvaluateRelevanceCalibrationCommand{}, ErrInvalidDocumentMatchContract
	}
	if _, found := requiredDimensions["source"]; !found {
		return EvaluateRelevanceCalibrationCommand{}, ErrInvalidDocumentMatchContract
	}
	sort.Slice(command.Samples, func(left, right int) bool { return command.Samples[left].SampleID < command.Samples[right].SampleID })
	sort.Slice(command.RequiredSlices, func(left, right int) bool {
		return command.RequiredSlices[left].Dimension+":"+command.RequiredSlices[left].Value < command.RequiredSlices[right].Dimension+":"+command.RequiredSlices[right].Value
	})
	return command, nil
}

func calculateRelevanceMetrics(samples []RelevanceEvaluationSampleDTO, probabilities map[string]float64, acceptThreshold float64) RelevanceEvaluationMetricsDTO {
	metrics := RelevanceEvaluationMetricsDTO{SampleCount: len(samples)}
	accepted, trueAccepted, retrievedPositive := 0, 0, 0
	binsCount, binsProbability, binsOutcome := [10]int{}, [10]float64{}, [10]float64{}
	for _, sample := range samples {
		probability := probabilities[sample.SampleID]
		outcome := 0.0
		if sample.Relevant {
			metrics.PositiveCount++
			outcome = 1
			if sample.RetrievedAt100 {
				retrievedPositive++
			}
		} else {
			metrics.NegativeCount++
		}
		predictedAccepted := sample.RetrievedAt100 && probability >= acceptThreshold
		if predictedAccepted {
			accepted++
			if sample.Relevant {
				trueAccepted++
			}
		}
		if sample.HardNegative {
			metrics.HardNegativeCount++
			if predictedAccepted {
				metrics.HardNegativePassed = false
			}
		}
		delta := probability - outcome
		metrics.Brier += delta * delta
		bin := int(probability * 10)
		if bin == 10 {
			bin = 9
		}
		binsCount[bin]++
		binsProbability[bin] += probability
		binsOutcome[bin] += outcome
	}
	metrics.HardNegativePassed = metrics.HardNegativeCount >= MinimumRelevanceSliceSamples
	for _, sample := range samples {
		if sample.HardNegative && sample.RetrievedAt100 && probabilities[sample.SampleID] >= acceptThreshold {
			metrics.HardNegativePassed = false
		}
	}
	if metrics.PositiveCount > 0 {
		metrics.RecallAt100 = float64(retrievedPositive) / float64(metrics.PositiveCount)
		metrics.Recall = float64(trueAccepted) / float64(metrics.PositiveCount)
	}
	if accepted > 0 {
		metrics.Precision = float64(trueAccepted) / float64(accepted)
		metrics.PrecisionWilsonLower = wilsonLower(trueAccepted, accepted)
	}
	if len(samples) > 0 {
		metrics.Brier /= float64(len(samples))
		for index, count := range binsCount {
			if count > 0 {
				metrics.ECE += float64(count) / float64(len(samples)) * math.Abs(binsProbability[index]/float64(count)-binsOutcome[index]/float64(count))
			}
		}
	}
	return metrics
}

func calculateRelevanceSlices(command EvaluateRelevanceCalibrationCommand, probabilities map[string]float64) []RelevanceEvaluationSliceResultDTO {
	result := make([]RelevanceEvaluationSliceResultDTO, 0, len(command.RequiredSlices))
	for _, required := range command.RequiredSlices {
		values := []RelevanceEvaluationSampleDTO{}
		for _, sample := range command.Samples {
			value := sample.Language
			if required.Dimension == "source" {
				value = sample.SourceType
			}
			if value == required.Value {
				values = append(values, sample)
			}
		}
		metrics := calculateRelevanceMetrics(values, probabilities, command.AcceptThreshold)
		result = append(result, RelevanceEvaluationSliceResultDTO{
			Dimension: required.Dimension, Value: required.Value, SampleCount: metrics.SampleCount,
			PositiveCount: metrics.PositiveCount, NegativeCount: metrics.NegativeCount,
			Precision: metrics.Precision, Recall: metrics.Recall,
			Passed: metrics.SampleCount >= MinimumRelevanceSliceSamples && metrics.Precision >= .90 && metrics.Recall >= .80,
		})
	}
	return result
}

func relevanceMetricsPass(metrics RelevanceEvaluationMetricsDTO, slices []RelevanceEvaluationSliceResultDTO) bool {
	if metrics.PositiveCount < MinimumRelevancePositiveSamples || metrics.NegativeCount < MinimumRelevanceNegativeSamples ||
		metrics.RecallAt100 < .95 || metrics.Precision < .90 || metrics.Recall < .80 || metrics.ECE > .05 ||
		metrics.PrecisionWilsonLower < .87 || !metrics.HardNegativePassed {
		return false
	}
	for _, slice := range slices {
		if !slice.Passed {
			return false
		}
	}
	return true
}

func wilsonLower(successes, total int) float64 {
	if total <= 0 {
		return 0
	}
	z := 1.959963984540054
	p := float64(successes) / float64(total)
	denominator := 1 + z*z/float64(total)
	center := p + z*z/(2*float64(total))
	margin := z * math.Sqrt((p*(1-p)+z*z/(4*float64(total)))/float64(total))
	return (center - margin) / denominator
}

func relevanceDatasetHash(command EvaluateRelevanceCalibrationCommand) string {
	parts := []string{"relevance-evaluation-dataset-v2", command.DatasetVersion, command.FamilyIsolationHash,
		command.AnnotationProtocolVersion, command.AnnotationGuidelineSHA256, command.SplitStrategyVersion,
		strconv.Itoa(command.AnnotatorCount), command.AgreementMetric, strconv.FormatFloat(command.AgreementScore, 'f', 7, 64),
		command.TimeBoundary.Format(time.RFC3339Nano)}
	for _, slice := range command.RequiredSlices {
		parts = append(parts, slice.Dimension, slice.Value)
	}
	for _, sample := range command.Samples {
		parts = append(parts, sample.SampleID, sample.ContentFamilyHash, sample.ObservedAt.Format(time.RFC3339Nano), sample.Language, sample.SourceType,
			strconv.FormatBool(sample.Relevant), strconv.FormatBool(sample.RetrievedAt100), strconv.FormatBool(sample.HardNegative), strconv.FormatInt(sample.Candidate.DocumentVersionID, 10))
		for _, signal := range sample.Candidate.Signals {
			parts = append(parts, signal.Channel, strconv.Itoa(signal.Rank), strconv.FormatFloat(signal.RawScore, 'g', -1, 64), signal.AlgorithmVersion)
		}
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\x1f"))))
}
