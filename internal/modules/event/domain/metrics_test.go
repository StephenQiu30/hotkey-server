package domain

import (
	"testing"
	"time"
)

func TestNormalizeSourceMetricPreservesMissingValues(t *testing.T) {
	value := int64(10)
	metric, err := NormalizeSourceMetric(SourceMetric{SourceID: 1, CapturedAt: time.Now().UTC(), ViewCount: &value, Credibility: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(metric.Available) != 1 || metric.Available[0] != "views" {
		t.Fatalf("available = %#v", metric.Available)
	}
	if metric.Value <= 0 || metric.Value >= 100 {
		t.Fatalf("value = %v", metric.Value)
	}
}

func TestCalculateHeatCapsSingleSource(t *testing.T) {
	result, err := CalculateHeat(HeatInput{EventID: 1, Sources: []NormalizedMetric{{SourceID: 1, Value: 100}}, IndependentSources: 1, ContentCount: 1, FreshnessHours: 0, EvidenceSetHash: "hash", HeatVersion: "v1", WindowEnd: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if result.HeatScore > 100 || len(result.ReasonCodes) != 2 {
		t.Fatalf("heat = %#v", result)
	}
}

func TestCalculateTrendAndRanking(t *testing.T) {
	trend, err := CalculateTrend(80, 60)
	if err != nil || trend.Status != TrendRising || trend.Score != 20 {
		t.Fatalf("trend = %#v/%v", trend, err)
	}
	score, reasons, err := RankMonitorEvent(RankingInput{RelevanceScore: 90, HeatScore: 80, TrendScore: 20, FreshnessHours: 1})
	if err != nil || score <= 70 || len(reasons) < 3 {
		t.Fatalf("ranking = %v/%#v/%v", score, reasons, err)
	}
}
