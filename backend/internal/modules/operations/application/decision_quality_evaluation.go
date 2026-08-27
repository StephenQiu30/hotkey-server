package application

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	eventdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
)

const (
	ContentFamilyQualityModule        = "content_family"
	MicroEventClusteringQualityModule = "micro_event_clustering"
	EvidenceLocatorQualityModule      = "evidence_locator"
	EvidenceRelationQualityModule     = "evidence_relation"
	HotspotDetectionQualityModule     = "hotspot_detection"
)

var ErrInvalidDecisionQualityDataset = errors.New("decision quality evaluation dataset is invalid")

type DecisionQualityDatasetDTO struct {
	DatasetVersion, DatasetSHA256, AnnotationProtocolVersion, AnnotationGuidelineSHA256 string
	SplitStrategyVersion, FamilyIsolationSHA256, EventIsolationSHA256                   string
	AnnotatorCount                                                                      int
	AgreementMetric                                                                     string
	AgreementScore                                                                      float64
	TimeBoundary                                                                        time.Time
	ContentFamilyProfileVersion, MicroEventProfileVersion                               string
	EvidenceLocatorProfileVersion, EvidenceRelationProfileVersion                       string
	HotspotProfileVersion                                                               string
	DuplicateSamples                                                                    []DuplicateQualitySampleDTO
	MicroEventSamples                                                                   []MicroEventQualitySampleDTO
	EvidenceLocatorSamples                                                              []EvidenceLocatorQualitySampleDTO
	EvidenceRelationSamples                                                             []EvidenceRelationQualitySampleDTO
	HotspotSamples                                                                      []HotspotQualitySampleDTO
}

type DuplicateQualitySampleDTO struct {
	SampleID, Language, SourceType, LeftFixtureText, RightFixtureText string
	ObservedAt                                                        time.Time
	Duplicate, HardNegative                                           bool
}

type MicroEventQualitySampleDTO struct {
	SampleID, Language, SourceType, EventSize string
	ObservedAt                                time.Time
	SameEvent                                 bool
	DenseAvailable, HardConflict              bool
	Features                                  MicroEventQualityFeaturesDTO
}

type MicroEventQualityFeaturesDTO struct {
	SparseSimilarity, DenseSimilarity, EntityOverlap, ActionOverlap float64
	LocationConsistency, IdentifierConsistency, TimeSimilarity      float64
	LineageRelation                                                 float64
}

type EvidenceLocatorQualitySampleDTO struct {
	SampleID, Language, SourceType, SyntheticPlaintext string
	ExactQuote, Prefix, Suffix                         string
	PlaintextSHA256                                    string
	UTF8ByteStart, UTF8ByteEnd                         int64
	ObservedAt                                         time.Time
	CitationFieldsComplete                             bool
}

type EvidenceRelationQualitySampleDTO struct {
	SampleID, Language, SourceType, ExpectedRelation, PredictedRelation string
	ObservedAt                                                          time.Time
}

type HotspotQualitySampleDTO struct {
	SampleID, Language, SourceType string
	ObservedAt                     time.Time
	ExpectedHotspot                bool
	DiscoveryDelaySeconds          float64
	Threshold                      float64
	Input                          EventHeatQualityInputDTO
}

type EventHeatQualityInputDTO struct {
	IndependentLineageRoots, ReportsInWindow, ReportsInPreviousWindow, ReportsInPriorWindow int
	PublisherCoverage, SourceTypeCoverage                                                   int
	NormalizedEngagement                                                                    *float64
	AgeHours                                                                                float64
}

type DecisionQualityMetricDTO struct {
	Module                      string   `json:"module"`
	ProfileVersion              string   `json:"profile_version"`
	SampleCount                 int      `json:"sample_count"`
	PositiveCount               int      `json:"positive_count"`
	NegativeCount               int      `json:"negative_count"`
	Precision                   float64  `json:"precision"`
	Recall                      float64  `json:"recall"`
	PrecisionWilsonLower        float64  `json:"precision_wilson_lower"`
	FalseMergeRate              float64  `json:"false_merge_rate"`
	PairwisePrecision           float64  `json:"pairwise_precision"`
	BCubedF1                    float64  `json:"b_cubed_f1"`
	CEAFE                       float64  `json:"ceaf_e"`
	ClusterCountRatio           float64  `json:"cluster_count_ratio"`
	LocatorAccuracy             float64  `json:"locator_accuracy"`
	ProvenanceCompleteness      float64  `json:"provenance_completeness"`
	EvidenceRelationMacroF1     float64  `json:"evidence_relation_macro_f1"`
	HotspotPrecision            float64  `json:"hotspot_precision"`
	MedianDiscoveryDelaySeconds float64  `json:"median_discovery_delay_seconds"`
	Passed                      bool     `json:"passed"`
	AutomaticDecisionAllowed    bool     `json:"automatic_decision_allowed"`
	ReasonCodes                 []string `json:"reason_codes"`
}

type DecisionQualitySliceDTO struct {
	Module      string  `json:"module"`
	Dimension   string  `json:"dimension"`
	Value       string  `json:"value"`
	SampleCount int     `json:"sample_count"`
	Precision   float64 `json:"precision"`
	Recall      float64 `json:"recall"`
	Passed      bool    `json:"passed"`
}

type DecisionQualityEvaluationResult struct {
	DatasetVersion         string                     `json:"dataset_version"`
	DatasetSHA256          string                     `json:"dataset_sha256"`
	Metrics                []DecisionQualityMetricDTO `json:"metrics"`
	Slices                 []DecisionQualitySliceDTO  `json:"slices"`
	AllRequiredGatesPassed bool                       `json:"all_required_gates_passed"`
}

func EvaluateDecisionQuality(dataset DecisionQualityDatasetDTO) (DecisionQualityEvaluationResult, error) {
	if err := validateDecisionQualityDataset(dataset); err != nil {
		return DecisionQualityEvaluationResult{}, err
	}
	duplicate, duplicateSlices, err := evaluateDuplicateQuality(dataset)
	if err != nil {
		return DecisionQualityEvaluationResult{}, err
	}
	microEvent, microEventSlices, err := evaluateMicroEventQuality(dataset)
	if err != nil {
		return DecisionQualityEvaluationResult{}, err
	}
	locator, err := evaluateEvidenceLocatorQuality(dataset)
	if err != nil {
		return DecisionQualityEvaluationResult{}, err
	}
	relation := evaluateEvidenceRelationQuality(dataset)
	hotspot, hotspotSlices, err := evaluateHotspotQuality(dataset)
	if err != nil {
		return DecisionQualityEvaluationResult{}, err
	}
	slices := append(append(duplicateSlices, microEventSlices...), hotspotSlices...)
	metrics := []DecisionQualityMetricDTO{duplicate, microEvent, locator, relation, hotspot}
	for _, qualitySlice := range slices {
		if qualitySlice.Passed {
			continue
		}
		for index := range metrics {
			if metrics[index].Module != qualitySlice.Module {
				continue
			}
			metrics[index].Passed = false
			metrics[index].AutomaticDecisionAllowed = false
			if !containsQualityReason(metrics[index].ReasonCodes, "quality_slice_failed") {
				metrics[index].ReasonCodes = append(metrics[index].ReasonCodes, "quality_slice_failed")
			}
		}
	}
	allPassed := true
	for _, metric := range metrics {
		allPassed = allPassed && metric.Passed
	}
	return DecisionQualityEvaluationResult{DatasetVersion: dataset.DatasetVersion, DatasetSHA256: dataset.DatasetSHA256,
		Metrics: metrics, Slices: slices,
		AllRequiredGatesPassed: allPassed}, nil
}

func validateDecisionQualityDataset(value DecisionQualityDatasetDTO) error {
	stringsRequired := []string{value.DatasetVersion, value.DatasetSHA256, value.AnnotationProtocolVersion,
		value.AnnotationGuidelineSHA256, value.SplitStrategyVersion, value.FamilyIsolationSHA256,
		value.EventIsolationSHA256, value.AgreementMetric, value.ContentFamilyProfileVersion,
		value.MicroEventProfileVersion, value.EvidenceLocatorProfileVersion,
		value.EvidenceRelationProfileVersion, value.HotspotProfileVersion}
	for _, field := range stringsRequired {
		if strings.TrimSpace(field) == "" {
			return ErrInvalidDecisionQualityDataset
		}
	}
	for _, digest := range []string{value.DatasetSHA256, value.AnnotationGuidelineSHA256, value.FamilyIsolationSHA256, value.EventIsolationSHA256} {
		if !lowerHexSHA256(digest) {
			return ErrInvalidDecisionQualityDataset
		}
	}
	if value.TimeBoundary.IsZero() || value.AnnotatorCount < 2 || value.AgreementScore < 0 || value.AgreementScore > 1 ||
		len(value.DuplicateSamples) < 400 || len(value.MicroEventSamples) < 400 ||
		len(value.EvidenceLocatorSamples) < 200 || len(value.EvidenceRelationSamples) < 350 || len(value.HotspotSamples) < 400 {
		return ErrInvalidDecisionQualityDataset
	}
	seen := make(map[string]struct{})
	check := func(id string, observedAt time.Time) bool {
		id = strings.TrimSpace(id)
		if id == "" || observedAt.IsZero() || !observedAt.After(value.TimeBoundary) {
			return false
		}
		if _, found := seen[id]; found {
			return false
		}
		seen[id] = struct{}{}
		return true
	}
	for _, sample := range value.DuplicateSamples {
		if !check(sample.SampleID, sample.ObservedAt) || !validQualitySlice(sample.Language, sample.SourceType) ||
			strings.TrimSpace(sample.LeftFixtureText) == "" || strings.TrimSpace(sample.RightFixtureText) == "" ||
			len(sample.LeftFixtureText) > 4096 || len(sample.RightFixtureText) > 4096 {
			return ErrInvalidDecisionQualityDataset
		}
	}
	for _, sample := range value.MicroEventSamples {
		if !check(sample.SampleID, sample.ObservedAt) || !validQualitySlice(sample.Language, sample.SourceType) || strings.TrimSpace(sample.EventSize) == "" {
			return ErrInvalidDecisionQualityDataset
		}
	}
	for _, sample := range value.EvidenceLocatorSamples {
		if !check(sample.SampleID, sample.ObservedAt) || !validQualitySlice(sample.Language, sample.SourceType) ||
			strings.TrimSpace(sample.SyntheticPlaintext) == "" || strings.TrimSpace(sample.ExactQuote) == "" || len(sample.SyntheticPlaintext) > 4096 {
			return ErrInvalidDecisionQualityDataset
		}
	}
	for _, sample := range value.EvidenceRelationSamples {
		if !check(sample.SampleID, sample.ObservedAt) || !validQualitySlice(sample.Language, sample.SourceType) ||
			!validEvidenceRelation(sample.ExpectedRelation) || !validEvidenceRelation(sample.PredictedRelation) {
			return ErrInvalidDecisionQualityDataset
		}
	}
	for _, sample := range value.HotspotSamples {
		if !check(sample.SampleID, sample.ObservedAt) || !validQualitySlice(sample.Language, sample.SourceType) ||
			sample.Threshold < 0 || sample.Threshold > 100 || sample.DiscoveryDelaySeconds < 0 {
			return ErrInvalidDecisionQualityDataset
		}
	}
	return nil
}

func containsQualityReason(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func evaluateDuplicateQuality(dataset DecisionQualityDatasetDTO) (DecisionQualityMetricDTO, []DecisionQualitySliceDTO, error) {
	confusion := binaryConfusion{}
	slices := make(map[string]*binaryConfusion)
	for index, sample := range dataset.DuplicateSamples {
		left, err := ingestiondomain.BuildContentFingerprint(sample.LeftFixtureText, "quality-fingerprint-v1")
		if err != nil {
			return DecisionQualityMetricDTO{}, nil, ErrInvalidDecisionQualityDataset
		}
		right, err := ingestiondomain.BuildContentFingerprint(sample.RightFixtureText, "quality-fingerprint-v1")
		if err != nil {
			return DecisionQualityMetricDTO{}, nil, ErrInvalidDecisionQualityDataset
		}
		decision, err := ingestiondomain.DecideContentFamily(ingestiondomain.ContentFamilyDecisionInput{
			DocumentVersionID: int64(index*2 + 2), Fingerprint: right,
			Candidates: []ingestiondomain.ContentFamilyCandidate{{FamilyID: int64(index + 1), FamilyVersion: 1,
				RootDocumentVersionID: int64(index*2 + 1), Fingerprint: left}},
			DecisionProfileVersion: dataset.ContentFamilyProfileVersion, HardConflict: sample.HardNegative,
		})
		if err != nil {
			return DecisionQualityMetricDTO{}, nil, err
		}
		predicted := decision.Action == ingestiondomain.ContentFamilyActionJoin
		confusion.add(sample.Duplicate, predicted)
		qualitySliceConfusion(slices, "language", sample.Language).add(sample.Duplicate, predicted)
		qualitySliceConfusion(slices, "source_type", sample.SourceType).add(sample.Duplicate, predicted)
	}
	metric := qualityMetricFromConfusion(ContentFamilyQualityModule, dataset.ContentFamilyProfileVersion, confusion)
	metric.FalseMergeRate = ratio(confusion.falsePositive, confusion.falsePositive+confusion.trueNegative)
	metric.Passed = confusion.positive() >= 200 && confusion.negative() >= 200 && metric.Precision >= .995 &&
		metric.Recall >= .90 && metric.PrecisionWilsonLower >= .965 && metric.FalseMergeRate < .005
	metric.AutomaticDecisionAllowed = metric.Passed
	metric.ReasonCodes = qualityGateReasons(metric.Passed)
	return metric, qualitySlices(ContentFamilyQualityModule, slices, .965, .90), nil
}

func evaluateMicroEventQuality(dataset DecisionQualityDatasetDTO) (DecisionQualityMetricDTO, []DecisionQualitySliceDTO, error) {
	confusion := binaryConfusion{}
	slices := make(map[string]*binaryConfusion)
	bcubedPrecision, bcubedRecall, ceafNumerator := 0.0, 0.0, 0.0
	trueClusters, predictedClusters := 0, 0
	for index, sample := range dataset.MicroEventSamples {
		features := eventdomain.MicroEventFeatures{SparseSimilarity: sample.Features.SparseSimilarity,
			DenseSimilarity: sample.Features.DenseSimilarity, EntityOverlap: sample.Features.EntityOverlap,
			ActionOverlap: sample.Features.ActionOverlap, LocationConsistency: sample.Features.LocationConsistency,
			IdentifierConsistency: sample.Features.IdentifierConsistency, TimeSimilarity: sample.Features.TimeSimilarity,
			LineageRelation: sample.Features.LineageRelation}
		decision, err := eventdomain.DecideMicroEventMembership(eventdomain.MicroEventDecisionInput{
			ContentFamilyID: int64(index + 1), ProfileVersion: dataset.MicroEventProfileVersion,
			Candidates: []eventdomain.MicroEventCandidate{{MicroEventID: int64(index + 1), EventVersion: 1,
				Features: features, DenseAvailable: sample.DenseAvailable, HardConflict: sample.HardConflict,
				HardConflictReasons: qualityConflictReasons(sample.HardConflict)}},
		})
		if err != nil {
			return DecisionQualityMetricDTO{}, nil, err
		}
		predictedSame := decision.Action == eventdomain.MicroEventActionJoin
		confusion.add(sample.SameEvent, predictedSame)
		qualitySliceConfusion(slices, "language", sample.Language).add(sample.SameEvent, predictedSame)
		qualitySliceConfusion(slices, "source_type", sample.SourceType).add(sample.SameEvent, predictedSame)
		qualitySliceConfusion(slices, "event_size", sample.EventSize).add(sample.SameEvent, predictedSame)
		trueCount, predictedCount := 2, 2
		itemPrecision, itemRecall := 2.0, 2.0
		if sample.SameEvent {
			trueCount = 1
		}
		if predictedSame {
			predictedCount = 1
		}
		localCEAF := float64(trueCount)
		if sample.SameEvent != predictedSame {
			if sample.SameEvent {
				itemPrecision, itemRecall = 2, 1
			} else {
				itemPrecision, itemRecall = 1, 2
			}
			localCEAF = 2.0 / 3.0
		}
		bcubedPrecision += itemPrecision
		bcubedRecall += itemRecall
		ceafNumerator += localCEAF
		trueClusters += trueCount
		predictedClusters += predictedCount
	}
	metric := qualityMetricFromConfusion(MicroEventClusteringQualityModule, dataset.MicroEventProfileVersion, confusion)
	metric.PairwisePrecision = metric.Precision
	itemCount := float64(len(dataset.MicroEventSamples) * 2)
	p, r := bcubedPrecision/itemCount, bcubedRecall/itemCount
	metric.BCubedF1 = harmonicMean(p, r)
	metric.CEAFE = ceafNumerator / float64(maxInt(trueClusters, predictedClusters))
	metric.ClusterCountRatio = float64(predictedClusters) / float64(trueClusters)
	metric.Passed = confusion.positive() >= 200 && confusion.negative() >= 200 && metric.PairwisePrecision >= .95 &&
		metric.PrecisionWilsonLower >= .92 && metric.BCubedF1 >= .90
	metric.AutomaticDecisionAllowed = metric.Passed
	metric.ReasonCodes = qualityGateReasons(metric.Passed)
	return metric, qualitySlices(MicroEventClusteringQualityModule, slices, .92, .80), nil
}

func evaluateEvidenceLocatorQuality(dataset DecisionQualityDatasetDTO) (DecisionQualityMetricDTO, error) {
	correct, complete := 0, 0
	for _, sample := range dataset.EvidenceLocatorSamples {
		selector, err := ingestiondomain.BuildTextQuoteSelector(sample.SyntheticPlaintext, ingestiondomain.TextQuoteSelectorCandidate{
			PlaintextSHA256: sample.PlaintextSHA256, ExactQuote: sample.ExactQuote, Prefix: sample.Prefix, Suffix: sample.Suffix,
			UTF8ByteStart: sample.UTF8ByteStart, UTF8ByteEnd: sample.UTF8ByteEnd,
			NormalizationVersion: ingestiondomain.CanonicalTextQuoteNormalizationVersion,
		})
		if err == nil && selector.ExactQuote == sample.ExactQuote && selector.UTF8ByteStart == sample.UTF8ByteStart &&
			selector.UTF8ByteEnd == sample.UTF8ByteEnd && selector.Prefix == sample.Prefix && selector.Suffix == sample.Suffix {
			correct++
		}
		if sample.CitationFieldsComplete {
			complete++
		}
	}
	metric := DecisionQualityMetricDTO{Module: EvidenceLocatorQualityModule, ProfileVersion: dataset.EvidenceLocatorProfileVersion,
		SampleCount: len(dataset.EvidenceLocatorSamples), LocatorAccuracy: ratio(correct, len(dataset.EvidenceLocatorSamples)),
		ProvenanceCompleteness: ratio(complete, len(dataset.EvidenceLocatorSamples))}
	metric.Passed = metric.SampleCount >= 200 && metric.LocatorAccuracy >= .98 && metric.ProvenanceCompleteness == 1
	metric.AutomaticDecisionAllowed = metric.Passed
	metric.ReasonCodes = qualityGateReasons(metric.Passed)
	return metric, nil
}

func evaluateEvidenceRelationQuality(dataset DecisionQualityDatasetDTO) DecisionQualityMetricDTO {
	classes := []string{"asserts", "attributes_to", "mentions", "contradicts", "corrects", "withdraws", "unknown"}
	macro := 0.0
	minimumClass := math.MaxInt
	for _, class := range classes {
		tp, fp, fn, count := 0, 0, 0, 0
		for _, sample := range dataset.EvidenceRelationSamples {
			if sample.ExpectedRelation == class {
				count++
				if sample.PredictedRelation == class {
					tp++
				} else {
					fn++
				}
			} else if sample.PredictedRelation == class {
				fp++
			}
		}
		minimumClass = minInt(minimumClass, count)
		precision, recall := ratio(tp, tp+fp), ratio(tp, tp+fn)
		macro += harmonicMean(precision, recall)
	}
	metric := DecisionQualityMetricDTO{Module: EvidenceRelationQualityModule, ProfileVersion: dataset.EvidenceRelationProfileVersion,
		SampleCount: len(dataset.EvidenceRelationSamples), EvidenceRelationMacroF1: macro / float64(len(classes))}
	metric.Passed = minimumClass >= 50 && metric.EvidenceRelationMacroF1 >= .90
	metric.AutomaticDecisionAllowed = metric.Passed
	metric.ReasonCodes = qualityGateReasons(metric.Passed)
	return metric
}

func evaluateHotspotQuality(dataset DecisionQualityDatasetDTO) (DecisionQualityMetricDTO, []DecisionQualitySliceDTO, error) {
	confusion := binaryConfusion{}
	slices := make(map[string]*binaryConfusion)
	delays := make([]float64, 0)
	for _, sample := range dataset.HotspotSamples {
		calculated, err := eventdomain.CalculateEventHeat(eventdomain.EventHeatInput{
			IndependentLineageRoots: sample.Input.IndependentLineageRoots, ReportsInWindow: sample.Input.ReportsInWindow,
			ReportsInPreviousWindow: sample.Input.ReportsInPreviousWindow, ReportsInPriorWindow: sample.Input.ReportsInPriorWindow,
			PublisherCoverage: sample.Input.PublisherCoverage, SourceTypeCoverage: sample.Input.SourceTypeCoverage,
			NormalizedEngagement: sample.Input.NormalizedEngagement, AgeHours: sample.Input.AgeHours,
			TemporalBaselineAvailable: true, ProfileVersion: dataset.HotspotProfileVersion,
		})
		if err != nil {
			return DecisionQualityMetricDTO{}, nil, err
		}
		predicted := calculated.Score >= sample.Threshold
		confusion.add(sample.ExpectedHotspot, predicted)
		qualitySliceConfusion(slices, "language", sample.Language).add(sample.ExpectedHotspot, predicted)
		qualitySliceConfusion(slices, "source_type", sample.SourceType).add(sample.ExpectedHotspot, predicted)
		if sample.ExpectedHotspot && predicted {
			delays = append(delays, sample.DiscoveryDelaySeconds)
		}
	}
	metric := qualityMetricFromConfusion(HotspotDetectionQualityModule, dataset.HotspotProfileVersion, confusion)
	metric.HotspotPrecision = metric.Precision
	metric.MedianDiscoveryDelaySeconds = median(delays)
	metric.Passed = confusion.positive() >= 200 && confusion.negative() >= 200 && metric.HotspotPrecision >= .80 &&
		metric.PrecisionWilsonLower >= .77 && len(delays) > 0 && metric.MedianDiscoveryDelaySeconds < 300
	metric.AutomaticDecisionAllowed = metric.Passed
	metric.ReasonCodes = qualityGateReasons(metric.Passed)
	return metric, qualitySlices(HotspotDetectionQualityModule, slices, .77, .70), nil
}

type binaryConfusion struct{ truePositive, falsePositive, trueNegative, falseNegative int }

func (value *binaryConfusion) add(expected, predicted bool) {
	switch {
	case expected && predicted:
		value.truePositive++
	case !expected && predicted:
		value.falsePositive++
	case !expected && !predicted:
		value.trueNegative++
	default:
		value.falseNegative++
	}
}
func (value binaryConfusion) positive() int { return value.truePositive + value.falseNegative }
func (value binaryConfusion) negative() int { return value.trueNegative + value.falsePositive }

func qualityMetricFromConfusion(module, profile string, value binaryConfusion) DecisionQualityMetricDTO {
	precision := ratio(value.truePositive, value.truePositive+value.falsePositive)
	return DecisionQualityMetricDTO{Module: module, ProfileVersion: profile,
		SampleCount: value.positive() + value.negative(), PositiveCount: value.positive(), NegativeCount: value.negative(),
		Precision: precision, Recall: ratio(value.truePositive, value.positive()),
		PrecisionWilsonLower: wilsonLower(value.truePositive, value.truePositive+value.falsePositive)}
}

func qualitySliceConfusion(values map[string]*binaryConfusion, dimension, value string) *binaryConfusion {
	key := dimension + "\x00" + value
	if values[key] == nil {
		values[key] = &binaryConfusion{}
	}
	return values[key]
}

func qualitySlices(module string, values map[string]*binaryConfusion, precisionGate, recallGate float64) []DecisionQualitySliceDTO {
	result := make([]DecisionQualitySliceDTO, 0, len(values))
	for key, confusion := range values {
		parts := strings.SplitN(key, "\x00", 2)
		precision := ratio(confusion.truePositive, confusion.truePositive+confusion.falsePositive)
		recall := ratio(confusion.truePositive, confusion.positive())
		result = append(result, DecisionQualitySliceDTO{Module: module, Dimension: parts[0], Value: parts[1],
			SampleCount: confusion.positive() + confusion.negative(), Precision: precision, Recall: recall,
			Passed: confusion.positive()+confusion.negative() >= 50 && precision >= precisionGate && recall >= recallGate})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Module != result[right].Module {
			return result[left].Module < result[right].Module
		}
		if result[left].Dimension != result[right].Dimension {
			return result[left].Dimension < result[right].Dimension
		}
		return result[left].Value < result[right].Value
	})
	return result
}

func wilsonLower(success, total int) float64 {
	if total <= 0 {
		return 0
	}
	z := 1.959963984540054
	p := float64(success) / float64(total)
	denominator := 1 + z*z/float64(total)
	center := p + z*z/(2*float64(total))
	margin := z * math.Sqrt((p*(1-p)+z*z/(4*float64(total)))/float64(total))
	return (center - margin) / denominator
}

func ratio(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
func harmonicMean(left, right float64) float64 {
	if left+right == 0 {
		return 0
	}
	return 2 * left * right / (left + right)
}
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	values = append([]float64(nil), values...)
	sort.Float64s(values)
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}
func qualityGateReasons(passed bool) []string {
	if passed {
		return []string{"quality_gates_passed"}
	}
	return []string{"quality_gates_failed", "automatic_decision_disabled"}
}
func qualityConflictReasons(conflict bool) []string {
	if conflict {
		return []string{"fixture_hard_conflict"}
	}
	return []string{}
}
func lowerHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
func validQualitySlice(language, sourceType string) bool {
	return strings.TrimSpace(language) != "" && strings.TrimSpace(sourceType) != "" && len(language) <= 32 && len(sourceType) <= 32
}
func validEvidenceRelation(value string) bool {
	switch value {
	case "asserts", "attributes_to", "mentions", "contradicts", "corrects", "withdraws", "unknown":
		return true
	default:
		return false
	}
}
func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
