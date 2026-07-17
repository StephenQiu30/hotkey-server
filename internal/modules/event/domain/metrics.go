package domain

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// SourceMetric is a source-reported snapshot. Nil values mean that the source
// did not provide the metric; they are intentionally different from zero.
type SourceMetric struct {
	SourceID                                       int64
	CapturedAt                                     time.Time
	ViewCount, LikeCount, CommentCount, ShareCount *int64
	Credibility                                    float64
}

func (metric SourceMetric) Validate() error {
	if metric.SourceID <= 0 || metric.CapturedAt.IsZero() || math.IsNaN(metric.Credibility) || math.IsInf(metric.Credibility, 0) || metric.Credibility < 0 || metric.Credibility > 1 {
		return fmt.Errorf("invalid source metric")
	}
	for _, value := range []*int64{metric.ViewCount, metric.LikeCount, metric.CommentCount, metric.ShareCount} {
		if value != nil && *value < 0 {
			return fmt.Errorf("source metric cannot be negative")
		}
	}
	return nil
}

type NormalizedMetric struct {
	SourceID  int64
	Value     float64
	Available []string
}

// NormalizeSourceMetric uses log scaling so one unusually large source cannot
// dominate an Event. Missing components are omitted and the remaining values
// are reweighted by their available count.
func NormalizeSourceMetric(metric SourceMetric) (NormalizedMetric, error) {
	if err := metric.Validate(); err != nil {
		return NormalizedMetric{}, err
	}
	type component struct {
		name   string
		value  *int64
		weight float64
	}
	components := []component{
		{"views", metric.ViewCount, 0.20},
		{"likes", metric.LikeCount, 0.30},
		{"comments", metric.CommentCount, 0.25},
		{"shares", metric.ShareCount, 0.25},
	}
	var total, weights float64
	available := make([]string, 0, len(components))
	for _, component := range components {
		if component.value == nil {
			continue
		}
		// log1p(1e6) maps a million interactions to 100. Values above
		// that are deliberately capped before the weighted sum.
		score := math.Min(100, math.Log1p(float64(*component.value))/math.Log1p(1_000_000)*100)
		total += score * component.weight
		weights += component.weight
		available = append(available, component.name)
	}
	if weights == 0 {
		return NormalizedMetric{SourceID: metric.SourceID}, nil
	}
	sort.Strings(available)
	return NormalizedMetric{SourceID: metric.SourceID, Value: math.Min(100, total/weights*metric.Credibility), Available: available}, nil
}

type HeatInput struct {
	EventID            int64
	Sources            []NormalizedMetric
	IndependentSources int
	ContentCount       int
	FreshnessHours     float64
	EvidenceSetHash    string
	HeatVersion        string
	WindowEnd          time.Time
}

type HeatResult struct {
	EventID         int64
	HeatScore       float64
	TrendScore      float64
	SourceCount     int
	ContentCount    int
	ReasonCodes     []string
	HeatVersion     string
	EvidenceSetHash string
	WindowEnd       time.Time
}

func (input HeatInput) Validate() error {
	if input.EventID <= 0 || input.IndependentSources < 0 || input.ContentCount < 0 || math.IsNaN(input.FreshnessHours) || math.IsInf(input.FreshnessHours, 0) || input.FreshnessHours < 0 || strings.TrimSpace(input.EvidenceSetHash) == "" || strings.TrimSpace(input.HeatVersion) == "" || input.WindowEnd.IsZero() {
		return fmt.Errorf("invalid heat input")
	}
	return nil
}

// CalculateHeat caps source contribution, diversity and freshness separately.
// This keeps one source or one content from producing an unbounded score.
func CalculateHeat(input HeatInput) (HeatResult, error) {
	if err := input.Validate(); err != nil {
		return HeatResult{}, err
	}
	var sourceTotal float64
	for _, source := range input.Sources {
		if source.Value < 0 || source.Value > 100 {
			return HeatResult{}, fmt.Errorf("invalid normalized source value")
		}
		sourceTotal += source.Value
	}
	if len(input.Sources) > 0 {
		sourceTotal /= float64(len(input.Sources))
	}
	diversity := math.Min(20, float64(input.IndependentSources)*5)
	contentBreadth := math.Min(10, float64(input.ContentCount)*2)
	freshness := math.Max(0, 10-math.Min(10, input.FreshnessHours/24))
	heat := math.Min(100, sourceTotal*0.60+diversity+contentBreadth+freshness)
	reasons := make([]string, 0, 4)
	if len(input.Sources) == 0 {
		reasons = append(reasons, "metrics_unavailable")
	}
	if input.IndependentSources <= 1 {
		reasons = append(reasons, "single_source_cap")
	}
	if input.ContentCount <= 1 {
		reasons = append(reasons, "single_content_cap")
	}
	if freshness < 5 {
		reasons = append(reasons, "stale_evidence")
	}
	return HeatResult{EventID: input.EventID, HeatScore: roundScore(heat), SourceCount: len(input.Sources), ContentCount: input.ContentCount, ReasonCodes: reasons, HeatVersion: input.HeatVersion, EvidenceSetHash: input.EvidenceSetHash, WindowEnd: input.WindowEnd}, nil
}

type TrendStatus string

const (
	TrendRising  TrendStatus = "rising"
	TrendStable  TrendStatus = "stable"
	TrendFalling TrendStatus = "falling"
)

type TrendResult struct {
	Score  float64
	Status TrendStatus
}

func CalculateTrend(current, previous float64) (TrendResult, error) {
	if current < 0 || current > 100 || previous < 0 || previous > 100 || math.IsNaN(current) || math.IsNaN(previous) {
		return TrendResult{}, fmt.Errorf("invalid trend values")
	}
	delta := math.Max(-100, math.Min(100, current-previous))
	status := TrendStable
	if delta >= 5 {
		status = TrendRising
	} else if delta <= -5 {
		status = TrendFalling
	}
	return TrendResult{Score: roundScore(delta), Status: status}, nil
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
