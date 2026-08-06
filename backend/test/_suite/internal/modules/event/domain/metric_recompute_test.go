package domain

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeEngagementPreservesMissingMetricsAndCapsOneItem(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	profile := MetricCapability{SourceConnectionID: 1, SourceType: "rss", ProfileVersion: "v1", ProfileID: 1, ProfileRecordVer: 2, SupportsViews: true, SupportsLikes: true, IndependenceStrategy: "source_connection", CredibilityWeight: 0.8, MaxSingleItemContribution: 30}
	current, baseline := int64(1_000), int64(0)
	score, fallback, err := NormalizeEngagement(MetricEvidence{ContentID: 1, SourceConnectionID: 1, ContentType: "article", PublishedAt: now, Latest: MetricCounts{Views: &current}, Baseline: MetricCounts{Views: &baseline}}, profile, MetricPopulation{MetricPopulationKey: MetricPopulationKey{SourceConnectionID: 1, ContentType: "article"}, Deltas: []MetricCounts{{Views: metricInt64(10)}, {Views: metricInt64(20)}, {Views: metricInt64(30)}, {Views: metricInt64(40)}, {Views: metricInt64(50)}}})
	if err != nil || fallback || score == nil || *score != 30 {
		t.Fatalf("NormalizeEngagement() = %v/%t/%v, want capped non-fallback 30", score, fallback, err)
	}
	missing, fallback, err := NormalizeEngagement(MetricEvidence{ContentID: 2, SourceConnectionID: 1, ContentType: "article", PublishedAt: now, Latest: MetricCounts{Views: &current}}, profile, MetricPopulation{MetricPopulationKey: MetricPopulationKey{SourceConnectionID: 1, ContentType: "article"}})
	if err != nil || fallback || missing != nil {
		t.Fatalf("NormalizeEngagement(missing baseline) = %v/%t/%v, want nil without fallback", missing, fallback, err)
	}
}

func TestCalculateRecomputedHeatReweightsUnavailableMetrics(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	result, err := CalculateRecomputedHeat(RecomputeHeatInput{EventID: 1, WindowEnd: now, WindowHours: 24, HeatVersion: HeatAlgorithmVersionV1, EvidenceSetHash: strings.Repeat("a", 64), CapabilityProfileSetHash: strings.Repeat("b", 64), EventAgeHours: 12, ActiveEvidenceCount: 1, Evidences: []NormalizedEvidence{{ContentID: 1, SourceConnectionID: 1, IndependenceKey: "source:1", SourceType: "rss", PublishedAt: now.Add(-time.Hour), CredibilityWeight: 0.8, UsedFallback: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.HeatScore <= 0 || result.SourceCount != 1 || result.ContentCount != 1 || !containsReason(result.ReasonCodes, "metrics_unavailable") || !containsReason(result.ReasonCodes, "normalization_fallback") || !containsReason(result.ReasonCodes, "single_source_cap") {
		t.Fatalf("heat result = %#v", result)
	}
}

func TestRecomputeHeatDistinguishesMissingCapabilityFromNoActiveEvidence(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	result, err := CalculateRecomputedHeat(RecomputeHeatInput{EventID: 1, WindowEnd: now, WindowHours: 24, HeatVersion: HeatAlgorithmVersionV1, EvidenceSetHash: strings.Repeat("a", 64), CapabilityProfileSetHash: strings.Repeat("b", 64), EventAgeHours: 12, ActiveEvidenceCount: 2})
	if err != nil || result.ContentCount != 2 || !containsReason(result.ReasonCodes, "metrics_unavailable") || containsReason(result.ReasonCodes, "no_active_evidence") {
		t.Fatalf("missing capability result = %#v/%v", result, err)
	}
}

func TestEvidenceSetHashChangesWhenMetricInputChanges(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	first := MetricEvidence{ContentID: 1, SourceConnectionID: 2, ContentType: "article", PublishedAt: now.Add(-time.Hour), BaselineAt: now.Add(-2 * time.Hour), LatestAt: now, Baseline: MetricCounts{Views: metricInt64(1)}, Latest: MetricCounts{Views: metricInt64(10)}}
	second := first
	second.Latest.Views = metricInt64(11)
	if EvidenceSetHash([]MetricEvidence{first}) == EvidenceSetHash([]MetricEvidence{second}) {
		t.Fatal("evidence-set hash ignored a metric snapshot value")
	}
}

func TestCalculateEMATrendUsesVersionedStatuses(t *testing.T) {
	rising, err := CalculateEMATrend(TrendInput{ShortSeries: []float64{40, 70}, LongSeries: []float64{30, 40}, EventAgeHours: 24, LatestEvidenceAgeHours: 1})
	if err != nil || rising.Status != TrendRising || rising.Score < 15 {
		t.Fatalf("rising trend = %#v/%v", rising, err)
	}
	dormant, err := CalculateEMATrend(TrendInput{ShortSeries: []float64{10}, LongSeries: []float64{10}, EventAgeHours: 120, LatestEvidenceAgeHours: 96})
	if err != nil || dormant.Status != TrendDormant {
		t.Fatalf("dormant trend = %#v/%v", dormant, err)
	}
}

func metricInt64(value int64) *int64 { return &value }
