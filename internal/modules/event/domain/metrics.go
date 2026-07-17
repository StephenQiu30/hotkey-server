package domain

import (
	"fmt"
	"math"
	"time"
)

// HeatResult is the versioned, explainable result of one heat recomputation.
// The input and formula live in metric_recompute.go; keeping this result type
// beside Monitor ranking avoids a second calculation implementation.
type HeatResult struct {
	EventID                  int64
	HeatScore                float64
	TrendScore               float64
	TrendStatus              TrendStatus
	SourceCount              int
	ContentCount             int
	ReasonCodes              []string
	HeatVersion              string
	EvidenceSetHash          string
	CapabilityProfileSetHash string
	WindowHours              int
	WindowEnd                time.Time
}

type TrendStatus string

const (
	TrendEmerging TrendStatus = "emerging"
	TrendRising   TrendStatus = "rising"
	TrendStable   TrendStatus = "stable"
	TrendFalling  TrendStatus = "falling"
	TrendDormant  TrendStatus = "dormant"
)

type TrendResult struct {
	Score  float64
	Status TrendStatus
}

type RankingInput struct {
	RelevanceScore   float64
	MinimumRelevance float64
	HeatScore        float64
	TrendScore       float64
	FreshnessHours   float64
}

func RankMonitorEvent(input RankingInput) (float64, []string, error) {
	if input.RelevanceScore < 0 || input.RelevanceScore > 100 || input.MinimumRelevance < 0 || input.MinimumRelevance > 100 || input.HeatScore < 0 || input.HeatScore > 100 || input.TrendScore < -100 || input.TrendScore > 100 || input.FreshnessHours < 0 || math.IsNaN(input.FreshnessHours) || math.IsInf(input.FreshnessHours, 0) {
		return 0, nil, fmt.Errorf("invalid ranking input")
	}
	if input.RelevanceScore < input.MinimumRelevance {
		// A global heat spike must never promote content that did not meet the
		// Monitor's relevance contract. Keep the relevance-only projection so
		// callers can preserve a deterministic order among rejected records.
		return roundScore(input.RelevanceScore), []string{"below_relevance_threshold"}, nil
	}
	// V1 uses the Design-010 24-hour half-life. Algorithm/version changes
	// produce new snapshots rather than silently rewriting prior scores.
	freshness := 100 * math.Exp(-math.Ln2*input.FreshnessHours/24)
	trend := math.Max(0, input.TrendScore+100) / 2
	score := input.RelevanceScore*0.55 + input.HeatScore*0.25 + trend*0.15 + freshness*0.05
	reasons := []string{"relevance"}
	if input.HeatScore >= 70 {
		reasons = append(reasons, "heat")
	}
	if input.TrendScore >= 5 {
		reasons = append(reasons, "rising")
	}
	if input.FreshnessHours <= 24 {
		reasons = append(reasons, "fresh")
	}
	return roundScore(math.Min(100, score)), reasons, nil
}

func roundScore(value float64) float64 { return math.Round(value*100) / 100 }
