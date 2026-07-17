package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const HeatAlgorithmVersionV1 = "heat-v1"

var metricWindowsV1 = []int{1, 6, 24}

func MetricWindowsV1() []int { return append([]int(nil), metricWindowsV1...) }

type MetricCounts struct {
	Views, Likes, Comments, Shares *int64
}

type MetricEvidence struct {
	ContentID, SourceConnectionID int64
	AuthorID                      *int64
	ContentType                   string
	PublishedAt                   time.Time
	BaselineAt, LatestAt          time.Time
	Baseline, Latest              MetricCounts
}

type MetricPopulationKey struct {
	SourceConnectionID int64
	ContentType        string
}

type MetricPopulation struct {
	MetricPopulationKey
	Deltas []MetricCounts
}

type MetricEvidenceSet struct {
	EventID     int64
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	Evidence    []MetricEvidence
	Populations []MetricPopulation
}

type MetricCapability struct {
	SourceConnectionID          int64
	SourceType, ProfileVersion  string
	ProfileID, ProfileRecordVer int64
	SupportsViews               bool
	SupportsLikes               bool
	SupportsComments            bool
	SupportsShares              bool
	IndependenceStrategy        string
	CredibilityWeight           float64
	MaxSingleItemContribution   float64
}

func (capability MetricCapability) Validate() error {
	if capability.SourceConnectionID <= 0 || capability.ProfileID <= 0 || capability.ProfileRecordVer <= 0 || strings.TrimSpace(capability.SourceType) == "" || strings.TrimSpace(capability.ProfileVersion) == "" || (capability.IndependenceStrategy != "source_connection" && capability.IndependenceStrategy != "author") || !finiteInRange(capability.CredibilityWeight, 0, 1) || !finiteInRange(capability.MaxSingleItemContribution, 0.01, 100) {
		return fmt.Errorf("invalid metric capability")
	}
	return nil
}

type NormalizedEvidence struct {
	ContentID, SourceConnectionID int64
	IndependenceKey               string
	SourceType                    string
	PublishedAt                   time.Time
	EngagementScore               *float64
	CredibilityWeight             float64
	UsedFallback                  bool
}

type RecomputeHeatInput struct {
	EventID                  int64
	WindowEnd                time.Time
	WindowHours              int
	HeatVersion              string
	EvidenceSetHash          string
	CapabilityProfileSetHash string
	EventAgeHours            float64
	ActiveEvidenceCount      int
	Evidences                []NormalizedEvidence
}

func (input RecomputeHeatInput) Validate() error {
	if input.EventID <= 0 || input.WindowEnd.IsZero() || !validMetricWindow(input.WindowHours) || strings.TrimSpace(input.HeatVersion) == "" || !validHash(input.EvidenceSetHash) || !validHash(input.CapabilityProfileSetHash) || !finiteAtLeast(input.EventAgeHours, 0) || input.ActiveEvidenceCount < len(input.Evidences) {
		return fmt.Errorf("invalid metric recompute input")
	}
	for _, evidence := range input.Evidences {
		if evidence.ContentID <= 0 || evidence.SourceConnectionID <= 0 || strings.TrimSpace(evidence.IndependenceKey) == "" || strings.TrimSpace(evidence.SourceType) == "" || evidence.PublishedAt.IsZero() || !finiteInRange(evidence.CredibilityWeight, 0, 1) {
			return fmt.Errorf("invalid metric evidence")
		}
		if evidence.EngagementScore != nil && !finiteInRange(*evidence.EngagementScore, 0, 100) {
			return fmt.Errorf("invalid normalized engagement")
		}
	}
	return nil
}

// NormalizeEngagement applies the Design-010 percentile rank to the metric
// deltas supported by the selected profile. Missing data remains absent: it
// is never turned into an artificial zero. Small rolling populations use the
// fixed heat-v1 log fallback and report that fact to the caller.
func NormalizeEngagement(evidence MetricEvidence, capability MetricCapability, population MetricPopulation) (*float64, bool, error) {
	if err := capability.Validate(); err != nil {
		return nil, false, err
	}
	if evidence.ContentID <= 0 || evidence.SourceConnectionID != capability.SourceConnectionID || strings.TrimSpace(evidence.ContentType) == "" {
		return nil, false, fmt.Errorf("invalid metric evidence for capability")
	}
	if population.SourceConnectionID != evidence.SourceConnectionID || population.ContentType != evidence.ContentType {
		return nil, false, fmt.Errorf("metric population does not match evidence")
	}
	type metric struct {
		enabled bool
		latest  *int64
		base    *int64
		values  func(MetricCounts) *int64
		weight  float64
	}
	metrics := []metric{
		{capability.SupportsViews, evidence.Latest.Views, evidence.Baseline.Views, func(counts MetricCounts) *int64 { return counts.Views }, 0.20},
		{capability.SupportsLikes, evidence.Latest.Likes, evidence.Baseline.Likes, func(counts MetricCounts) *int64 { return counts.Likes }, 0.30},
		{capability.SupportsComments, evidence.Latest.Comments, evidence.Baseline.Comments, func(counts MetricCounts) *int64 { return counts.Comments }, 0.25},
		{capability.SupportsShares, evidence.Latest.Shares, evidence.Baseline.Shares, func(counts MetricCounts) *int64 { return counts.Shares }, 0.25},
	}
	var total, weights float64
	usedFallback := false
	for _, item := range metrics {
		if !item.enabled {
			continue
		}
		delta, available := metricDelta(item.latest, item.base)
		if !available {
			continue
		}
		populationDeltas := make([]float64, 0, len(population.Deltas))
		for _, candidate := range population.Deltas {
			if candidateDelta, available := populationDelta(item.values(candidate)); available {
				populationDeltas = append(populationDeltas, candidateDelta)
			}
		}
		score, fallback := normalizeDelta(delta, populationDeltas)
		total += score * item.weight
		weights += item.weight
		usedFallback = usedFallback || fallback
	}
	if weights == 0 {
		return nil, usedFallback, nil
	}
	value := math.Min(capability.MaxSingleItemContribution, total/weights)
	return &value, usedFallback, nil
}

func metricDelta(latest, baseline *int64) (float64, bool) {
	if latest == nil || baseline == nil {
		return 0, false
	}
	if *latest < *baseline {
		return 0, false
	}
	return float64(*latest - *baseline), true
}

func populationDelta(value *int64) (float64, bool) {
	if value == nil || *value < 0 {
		return 0, false
	}
	return float64(*value), true
}

func normalizeDelta(delta float64, population []float64) (float64, bool) {
	if delta < 0 || math.IsNaN(delta) || math.IsInf(delta, 0) {
		return 0, true
	}
	valid := make([]float64, 0, len(population))
	for _, value := range population {
		if value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
			valid = append(valid, value)
		}
	}
	if len(valid) < 5 {
		return math.Min(100, math.Log1p(delta)/math.Log1p(1000)*100), true
	}
	sort.Float64s(valid)
	count := sort.Search(len(valid), func(index int) bool { return valid[index] > delta })
	return math.Min(100, math.Max(0, float64(count)/float64(len(valid))*100)), false
}

// CalculateRecomputedHeat applies the six Design-010 components. Engagement
// is reweighted away only when every source lacks a supported, valid metric.
func CalculateRecomputedHeat(input RecomputeHeatInput) (HeatResult, error) {
	if err := input.Validate(); err != nil {
		return HeatResult{}, err
	}
	if len(input.Evidences) == 0 {
		reasons := []string{"metrics_unavailable"}
		if input.ActiveEvidenceCount == 0 {
			reasons = append([]string{"no_active_evidence"}, reasons...)
		}
		return HeatResult{EventID: input.EventID, ContentCount: input.ActiveEvidenceCount, HeatVersion: input.HeatVersion, EvidenceSetHash: input.EvidenceSetHash, CapabilityProfileSetHash: input.CapabilityProfileSetHash, WindowHours: input.WindowHours, WindowEnd: input.WindowEnd.UTC(), ReasonCodes: reasons}, nil
	}
	groups := map[string]NormalizedEvidence{}
	sourceTypes := map[string]struct{}{}
	var latestEvidence time.Time
	anyFallback := false
	for _, evidence := range input.Evidences {
		current, found := groups[evidence.IndependenceKey]
		if !found || engagementValue(evidence.EngagementScore) > engagementValue(current.EngagementScore) {
			groups[evidence.IndependenceKey] = evidence
		}
		sourceTypes[evidence.SourceType] = struct{}{}
		if latestEvidence.IsZero() || evidence.PublishedAt.After(latestEvidence) {
			latestEvidence = evidence.PublishedAt
		}
		anyFallback = anyFallback || evidence.UsedFallback
	}
	independentCount := len(groups)
	contentCount := len(input.Evidences)
	independence := math.Min(100, float64(independentCount)*25)
	contentVelocity := math.Min(100, float64(contentCount)/float64(input.WindowHours)*24*20)
	sourceBreadth := math.Min(100, float64(len(sourceTypes))*50)
	var engagementTotal, credibilityTotal float64
	var engagementCount int
	for _, group := range groups {
		if group.EngagementScore != nil {
			engagementTotal += *group.EngagementScore
			engagementCount++
		}
		credibilityTotal += group.CredibilityWeight * 100
	}
	var engagement *float64
	if engagementCount > 0 {
		value := engagementTotal / float64(engagementCount)
		engagement = &value
	}
	credibility := 0.0
	if independentCount > 0 {
		credibility = credibilityTotal / float64(independentCount)
	}
	freshnessHours := float64(input.WindowHours)
	if !latestEvidence.IsZero() {
		freshnessHours = math.Max(0, input.WindowEnd.Sub(latestEvidence).Hours())
	}
	recency := 100 * math.Exp(-math.Ln2*freshnessHours/24)

	components := []struct {
		value  float64
		weight float64
		valid  bool
	}{
		{independence, 0.25, true},
		{contentVelocity, 0.25, true},
		{sourceBreadth, 0.15, true},
		{engagementValue(engagement), 0.15, engagement != nil},
		{recency, 0.15, true},
		{credibility, 0.05, true},
	}
	var score, weights float64
	for _, component := range components {
		if component.valid {
			score += component.value * component.weight
			weights += component.weight
		}
	}
	if weights > 0 {
		score /= weights
	}
	reasons := make([]string, 0, 5)
	if engagement == nil {
		reasons = append(reasons, "metrics_unavailable")
	}
	if anyFallback {
		reasons = append(reasons, "normalization_fallback")
	}
	if independentCount <= 1 {
		reasons = append(reasons, "single_source_cap")
	}
	if contentCount <= 1 {
		reasons = append(reasons, "single_content_cap")
	}
	if recency < 50 {
		reasons = append(reasons, "stale_evidence")
	}
	return HeatResult{EventID: input.EventID, HeatScore: roundScore(math.Min(100, score)), SourceCount: independentCount, ContentCount: contentCount, ReasonCodes: reasons, HeatVersion: input.HeatVersion, EvidenceSetHash: input.EvidenceSetHash, CapabilityProfileSetHash: input.CapabilityProfileSetHash, WindowHours: input.WindowHours, WindowEnd: input.WindowEnd.UTC()}, nil
}

type TrendInput struct {
	ShortSeries, LongSeries []float64 // oldest to newest, including this recomputation
	EventAgeHours           float64
	LatestEvidenceAgeHours  float64
}

func CalculateEMATrend(input TrendInput) (TrendResult, error) {
	if len(input.ShortSeries) == 0 || len(input.LongSeries) == 0 || !finiteAtLeast(input.EventAgeHours, 0) || !finiteAtLeast(input.LatestEvidenceAgeHours, 0) {
		return TrendResult{}, fmt.Errorf("invalid trend input")
	}
	shortEMA, err := exponentialMovingAverage(input.ShortSeries, 0.6)
	if err != nil {
		return TrendResult{}, err
	}
	longEMA, err := exponentialMovingAverage(input.LongSeries, 0.25)
	if err != nil {
		return TrendResult{}, err
	}
	acceleration := 0.0
	if len(input.ShortSeries) > 1 {
		acceleration = math.Max(-25, math.Min(25, input.ShortSeries[len(input.ShortSeries)-1]-input.ShortSeries[len(input.ShortSeries)-2]))
	}
	score := math.Max(-100, math.Min(100, shortEMA-longEMA+acceleration))
	status := TrendStable
	currentHeat := input.ShortSeries[len(input.ShortSeries)-1]
	switch {
	case currentHeat <= 20 && input.LatestEvidenceAgeHours >= 72:
		status = TrendDormant
	case input.EventAgeHours <= 6 && score >= 15:
		status = TrendEmerging
	case score >= 15:
		status = TrendRising
	case score <= -15:
		status = TrendFalling
	}
	return TrendResult{Score: roundScore(score), Status: status}, nil
}

func exponentialMovingAverage(values []float64, alpha float64) (float64, error) {
	if alpha <= 0 || alpha > 1 || len(values) == 0 {
		return 0, fmt.Errorf("invalid EMA input")
	}
	value := values[0]
	if !finiteInRange(value, 0, 100) {
		return 0, fmt.Errorf("invalid EMA score")
	}
	for _, next := range values[1:] {
		if !finiteInRange(next, 0, 100) {
			return 0, fmt.Errorf("invalid EMA score")
		}
		value = alpha*next + (1-alpha)*value
	}
	return value, nil
}

func EvidenceSetHash(evidence []MetricEvidence) string {
	items := make([]string, 0, len(evidence))
	for _, item := range evidence {
		items = append(items, fmt.Sprintf("%d:%d:%d:%s:%s:%s:%s:%s:%s", item.ContentID, item.SourceConnectionID, valueOrZero(item.AuthorID), item.ContentType, item.PublishedAt.UTC().Format(time.RFC3339Nano), item.BaselineAt.UTC().Format(time.RFC3339Nano), item.LatestAt.UTC().Format(time.RFC3339Nano), metricCountsIdentity(item.Baseline), metricCountsIdentity(item.Latest)))
	}
	sort.Strings(items)
	return hashMetricIdentity(items)
}

func metricCountsIdentity(counts MetricCounts) string {
	return strings.Join([]string{metricValueIdentity(counts.Views), metricValueIdentity(counts.Likes), metricValueIdentity(counts.Comments), metricValueIdentity(counts.Shares)}, ":")
}

func metricValueIdentity(value *int64) string {
	if value == nil {
		return "null"
	}
	return strconv.FormatInt(*value, 10)
}

func CapabilityProfileSetHash(capabilities []MetricCapability) string {
	items := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		items = append(items, fmt.Sprintf("%d:%s:%d:%d", capability.SourceConnectionID, capability.ProfileVersion, capability.ProfileID, capability.ProfileRecordVer))
	}
	sort.Strings(items)
	return hashMetricIdentity(items)
}

func hashMetricIdentity(items []string) string {
	digest := sha256.Sum256([]byte(strings.Join(items, "\n")))
	return hex.EncodeToString(digest[:])
}

func validMetricWindow(value int) bool { return value == 1 || value == 6 || value == 24 }
func validHash(value string) bool      { return len(value) == 64 && !strings.ContainsAny(value, " \t\r\n") }
func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
func engagementValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
func finiteAtLeast(value, minimum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum
}
func finiteInRange(value, minimum, maximum float64) bool {
	return finiteAtLeast(value, minimum) && value <= maximum
}
