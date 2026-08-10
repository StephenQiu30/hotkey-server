package application

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
)

func TestDecisionQualityEvaluationRunsProductionAlgorithmsAndPassesEveryGate(t *testing.T) {
	dataset := passingDecisionQualityDataset(t)
	result, err := EvaluateDecisionQuality(dataset)
	if err != nil {
		t.Fatalf("EvaluateDecisionQuality() error = %v", err)
	}
	if !result.AllRequiredGatesPassed || len(result.Metrics) != 5 || len(result.Slices) != 26 {
		t.Fatalf("quality result = %#v", result)
	}
	for _, metric := range result.Metrics {
		if !metric.Passed || !metric.AutomaticDecisionAllowed {
			t.Fatalf("quality metric did not pass: %#v", metric)
		}
	}
	if metric := qualityMetricByModule(t, result, ContentFamilyQualityModule); metric.Precision != 1 || metric.Recall != 1 || metric.FalseMergeRate != 0 {
		t.Fatalf("content-family metrics = %#v", metric)
	}
	if metric := qualityMetricByModule(t, result, MicroEventClusteringQualityModule); metric.PairwisePrecision != 1 || metric.BCubedF1 != 1 || metric.CEAFE != 1 || metric.ClusterCountRatio != 1 {
		t.Fatalf("micro-event metrics = %#v", metric)
	}
	if metric := qualityMetricByModule(t, result, EvidenceLocatorQualityModule); metric.LocatorAccuracy != 1 || metric.ProvenanceCompleteness != 1 {
		t.Fatalf("evidence-locator metrics = %#v", metric)
	}
	if metric := qualityMetricByModule(t, result, EvidenceRelationQualityModule); metric.EvidenceRelationMacroF1 != 1 {
		t.Fatalf("evidence-relation metrics = %#v", metric)
	}
	if metric := qualityMetricByModule(t, result, HotspotDetectionQualityModule); metric.HotspotPrecision != 1 || metric.MedianDiscoveryDelaySeconds != 120 {
		t.Fatalf("hotspot metrics = %#v", metric)
	}
}

func TestDecisionQualityEvaluationDisablesOnlyTheFailedAutomaticProfile(t *testing.T) {
	dataset := passingDecisionQualityDataset(t)
	for index := 200; index < 205; index++ {
		dataset.DuplicateSamples[index].RightFixtureText = dataset.DuplicateSamples[index].LeftFixtureText
	}
	result, err := EvaluateDecisionQuality(dataset)
	if err != nil {
		t.Fatal(err)
	}
	metric := qualityMetricByModule(t, result, ContentFamilyQualityModule)
	if metric.Passed || metric.AutomaticDecisionAllowed || result.AllRequiredGatesPassed || metric.FalseMergeRate <= 0 {
		t.Fatalf("failed content-family gate = %#v result=%#v", metric, result)
	}
	if other := qualityMetricByModule(t, result, MicroEventClusteringQualityModule); !other.Passed {
		t.Fatalf("unrelated quality module changed = %#v", other)
	}
}

func TestDecisionQualityDatasetRejectsDuplicateIDsAndPreBoundarySamples(t *testing.T) {
	for _, mutate := range []func(*DecisionQualityDatasetDTO){
		func(value *DecisionQualityDatasetDTO) {
			value.DuplicateSamples[1].SampleID = value.DuplicateSamples[0].SampleID
		},
		func(value *DecisionQualityDatasetDTO) {
			value.HotspotSamples[0].ObservedAt = value.TimeBoundary.Add(-time.Second)
		},
		func(value *DecisionQualityDatasetDTO) {
			value.HotspotSamples[0].ObservedAt = value.TimeBoundary
		},
	} {
		dataset := passingDecisionQualityDataset(t)
		mutate(&dataset)
		if _, err := EvaluateDecisionQuality(dataset); err == nil {
			t.Fatal("invalid time-isolated quality dataset was accepted")
		}
	}
}

func TestDecisionQualityEvaluationFailsAutomaticProfileWhenRequiredSliceFails(t *testing.T) {
	dataset := passingDecisionQualityDataset(t)
	changed := 0
	for index := range dataset.DuplicateSamples {
		if dataset.DuplicateSamples[index].Language != "en" || dataset.DuplicateSamples[index].Duplicate {
			continue
		}
		dataset.DuplicateSamples[index].RightFixtureText = dataset.DuplicateSamples[index].LeftFixtureText
		changed++
		if changed == 2 {
			break
		}
	}
	result, err := EvaluateDecisionQuality(dataset)
	if err != nil {
		t.Fatal(err)
	}
	metric := qualityMetricByModule(t, result, ContentFamilyQualityModule)
	if metric.Passed || metric.AutomaticDecisionAllowed || result.AllRequiredGatesPassed ||
		!containsQualityReason(metric.ReasonCodes, "quality_slice_failed") {
		t.Fatalf("failed slice did not close automatic decisions: metric=%#v result=%#v", metric, result)
	}
}

func passingDecisionQualityDataset(t *testing.T) DecisionQualityDatasetDTO {
	t.Helper()
	boundary := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	observedAt := boundary.Add(24 * time.Hour)
	dataset := DecisionQualityDatasetDTO{
		DatasetVersion: "decision-quality-time-isolated-v1", DatasetSHA256: strings.Repeat("a", 64),
		AnnotationProtocolVersion: "dual-review-v1", AnnotationGuidelineSHA256: strings.Repeat("b", 64),
		SplitStrategyVersion: "time-family-event-isolated-v1", FamilyIsolationSHA256: strings.Repeat("c", 64),
		EventIsolationSHA256: strings.Repeat("d", 64), AnnotatorCount: 2, AgreementMetric: "cohen_kappa",
		AgreementScore: .98, TimeBoundary: boundary,
		ContentFamilyProfileVersion:    "content-family-conservative-v1",
		MicroEventProfileVersion:       "same-event-cold-start-v1",
		EvidenceLocatorProfileVersion:  ingestiondomain.CanonicalTextQuoteSelectorVersion,
		EvidenceRelationProfileVersion: "claim-evidence-relation-v1",
		HotspotProfileVersion:          "event-heat-v2",
	}
	for index := 0; index < 400; index++ {
		language, sourceType := qualityTestSlice(index)
		duplicate := index < 200
		left := fmt.Sprintf("fixture document %03d postgres release notes with stable evidence chain", index)
		right := fmt.Sprintf("independent fixture %03d climate market summary with different identifiers", index)
		if duplicate {
			right = left
		}
		dataset.DuplicateSamples = append(dataset.DuplicateSamples, DuplicateQualitySampleDTO{
			SampleID: fmt.Sprintf("duplicate-%03d", index), Language: language, SourceType: sourceType,
			LeftFixtureText: left, RightFixtureText: right, ObservedAt: observedAt.Add(time.Duration(index) * time.Minute),
			Duplicate: duplicate, HardNegative: !duplicate && index%2 == 0,
		})
		value := .20
		if duplicate {
			value = 1
		}
		dataset.MicroEventSamples = append(dataset.MicroEventSamples, MicroEventQualitySampleDTO{
			SampleID: fmt.Sprintf("micro-event-%03d", index), Language: language, SourceType: sourceType,
			EventSize: []string{"small", "large"}[index%2], ObservedAt: observedAt.Add(time.Duration(500+index) * time.Minute),
			SameEvent: duplicate, DenseAvailable: true, HardConflict: !duplicate && index%2 == 0,
			Features: MicroEventQualityFeaturesDTO{SparseSimilarity: value, DenseSimilarity: value, EntityOverlap: value,
				ActionOverlap: value, LocationConsistency: value, IdentifierConsistency: value,
				TimeSimilarity: value, LineageRelation: value},
		})
		high := duplicate
		engagement := .95
		heat := EventHeatQualityInputDTO{IndependentLineageRoots: 1, ReportsInWindow: 1, ReportsInPreviousWindow: 1,
			ReportsInPriorWindow: 1, PublisherCoverage: 1, SourceTypeCoverage: 1, NormalizedEngagement: nil, AgeHours: 48}
		if high {
			heat = EventHeatQualityInputDTO{IndependentLineageRoots: 5, ReportsInWindow: 10, ReportsInPreviousWindow: 3,
				ReportsInPriorWindow: 1, PublisherCoverage: 5, SourceTypeCoverage: 4, NormalizedEngagement: &engagement, AgeHours: .2}
		}
		dataset.HotspotSamples = append(dataset.HotspotSamples, HotspotQualitySampleDTO{
			SampleID: fmt.Sprintf("hotspot-%03d", index), Language: language, SourceType: sourceType,
			ObservedAt: observedAt.Add(time.Duration(1000+index) * time.Minute), ExpectedHotspot: high,
			DiscoveryDelaySeconds: 120, Threshold: 70, Input: heat,
		})
	}
	for index := 0; index < 240; index++ {
		language, sourceType := qualityTestSlice(index)
		plain := fmt.Sprintf("前缀 %03d Café PostgreSQL 正式发布 优化功能 后缀", index)
		exact := "Café PostgreSQL 正式发布"
		start := strings.Index(plain, exact)
		digest := sha256.Sum256([]byte(plain))
		dataset.EvidenceLocatorSamples = append(dataset.EvidenceLocatorSamples, EvidenceLocatorQualitySampleDTO{
			SampleID: fmt.Sprintf("locator-%03d", index), Language: language, SourceType: sourceType,
			SyntheticPlaintext: plain, ExactQuote: exact, Prefix: plain[:start], Suffix: plain[start+len(exact):],
			PlaintextSHA256: hex.EncodeToString(digest[:]), UTF8ByteStart: int64(start), UTF8ByteEnd: int64(start + len(exact)),
			ObservedAt: observedAt.Add(time.Duration(1500+index) * time.Minute), CitationFieldsComplete: true,
		})
	}
	relations := []string{"asserts", "attributes_to", "mentions", "contradicts", "corrects", "withdraws", "unknown"}
	for index := 0; index < 420; index++ {
		language, sourceType := qualityTestSlice(index)
		relation := relations[index%len(relations)]
		dataset.EvidenceRelationSamples = append(dataset.EvidenceRelationSamples, EvidenceRelationQualitySampleDTO{
			SampleID: fmt.Sprintf("relation-%03d", index), Language: language, SourceType: sourceType,
			ExpectedRelation: relation, PredictedRelation: relation,
			ObservedAt: observedAt.Add(time.Duration(2000+index) * time.Minute),
		})
	}
	return dataset
}

func qualityTestSlice(index int) (string, string) {
	languages := []string{"zh", "en", "cross_language", "opposite_expression"}
	sources := []string{"feed", "platform", "search", "discussion"}
	return languages[index%len(languages)], sources[(index/4)%len(sources)]
}

func qualityMetricByModule(t *testing.T, result DecisionQualityEvaluationResult, module string) DecisionQualityMetricDTO {
	t.Helper()
	for _, metric := range result.Metrics {
		if metric.Module == module {
			return metric
		}
	}
	t.Fatalf("missing quality metric %s", module)
	return DecisionQualityMetricDTO{}
}
