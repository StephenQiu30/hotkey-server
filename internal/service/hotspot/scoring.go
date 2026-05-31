package hotspot

import (
	"context"
	"encoding/json"
	"math"
	"time"

	domainhotspot "github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
)

// ScoringConfig holds weights for each scoring dimension.
// All weights should sum to 1.0 for meaningful total scores.
type ScoringConfig struct {
	SourceCountWeight   float64
	FreshnessWeight     float64
	RelevanceWeight     float64
	PropagationWeight   float64
	QualityWeight       float64
	FreshnessDecayHours float64
}

func (c ScoringConfig) withDefaults() ScoringConfig {
	if c.SourceCountWeight <= 0 {
		c.SourceCountWeight = 0.3
	}
	if c.FreshnessWeight <= 0 {
		c.FreshnessWeight = 0.2
	}
	if c.RelevanceWeight <= 0 {
		c.RelevanceWeight = 0.2
	}
	if c.PropagationWeight <= 0 {
		c.PropagationWeight = 0.2
	}
	if c.QualityWeight <= 0 {
		c.QualityWeight = 0.1
	}
	if c.FreshnessDecayHours <= 0 {
		c.FreshnessDecayHours = 24
	}
	return c
}

// HotspotScore represents a computed score for a cluster.
type HotspotScore struct {
	ID               string
	ClusterID        string
	TotalScore       float64
	SourceCountScore float64
	FreshnessScore   float64
	RelevanceScore   float64
	PropagationScore float64
	QualityScore     float64
	Explanation      string
	ScoreVersion     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ScoreExplanation is the structured explanation JSON.
type ScoreExplanation struct {
	SourceCount     int     `json:"sourceCount"`
	UniqueSources   int     `json:"uniqueSources"`
	AvgSimilarity   float64 `json:"avgSimilarity"`
	HoursSinceLatest float64 `json:"hoursSinceLatest"`
	KeywordCount    int     `json:"keywordCount"`
	Weights         ScoringConfig `json:"weights"`
}

// ScoreRepository stores computed hotspot scores.
type ScoreRepository interface {
	SaveScore(context.Context, HotspotScore) (HotspotScore, error)
	ListScores(context.Context) ([]HotspotScore, error)
	ListScoresByWindow(context.Context, time.Time, time.Time) ([]HotspotScore, error)
	FindScoreByClusterID(context.Context, string) (HotspotScore, error)
}

// ClusterRepository reads clusters and their items for scoring.
type ClusterRepository interface {
	ListClusters(context.Context) ([]domainhotspot.Cluster, error)
	ListClusterItems(context.Context, string) ([]domainhotspot.ClusterItem, error)
}

// ScoringService computes scores for hotspot clusters.
type ScoringService struct {
	cfg        ScoringConfig
	clusters   ClusterRepository
	scores     ScoreRepository
	now        func() time.Time
}

// NewScoringService creates a scoring service with the given config and repositories.
func NewScoringService(cfg ScoringConfig, clusterRepo ClusterRepository, scoreRepo ScoreRepository) *ScoringService {
	cfg = cfg.withDefaults()
	return &ScoringService{
		cfg:      cfg,
		clusters: clusterRepo,
		scores:   scoreRepo,
		now:      time.Now,
	}
}

// SetClock overrides the time source, primarily for testing.
func (s *ScoringService) SetClock(clock func() time.Time) {
	s.now = clock
}

// ScoreClusters computes scores for all clusters within the given time window.
// Items outside the window are excluded from scoring.
func (s *ScoringService) ScoreClusters(ctx context.Context, windowStart, windowEnd time.Time) ([]HotspotScore, error) {
	clusters, err := s.clusters.ListClusters(ctx)
	if err != nil {
		return nil, err
	}

	var scores []HotspotScore
	for _, cluster := range clusters {
		items, err := s.clusters.ListClusterItems(ctx, cluster.ID)
		if err != nil {
			return nil, err
		}

		// Filter items by window (inclusive on both ends)
		filtered := make([]domainhotspot.ClusterItem, 0, len(items))
		for _, item := range items {
			if !item.CreatedAt.Before(windowStart) && !item.CreatedAt.After(windowEnd) {
				filtered = append(filtered, item)
			}
		}

		score := s.scoreCluster(cluster, filtered)
		if s.scores != nil {
			saved, err := s.scores.SaveScore(ctx, score)
			if err != nil {
				return nil, err
			}
			score = saved
		}
		scores = append(scores, score)
	}
	return scores, nil
}

// ListScores returns all saved scores sorted by total_score descending.
func (s *ScoringService) ListScores(ctx context.Context) ([]HotspotScore, error) {
	if s.scores == nil {
		return nil, nil
	}
	return s.scores.ListScores(ctx)
}

// ListScoresByWindow returns scores within a time window, sorted by total_score descending.
func (s *ScoringService) ListScoresByWindow(ctx context.Context, start, end time.Time) ([]HotspotScore, error) {
	if s.scores == nil {
		return nil, nil
	}
	return s.scores.ListScoresByWindow(ctx, start, end)
}

// FindScoreByClusterID returns the score for a specific cluster.
func (s *ScoringService) FindScoreByClusterID(ctx context.Context, clusterID string) (HotspotScore, error) {
	if s.scores == nil {
		return HotspotScore{}, ErrScoreNotFound
	}
	return s.scores.FindScoreByClusterID(ctx, clusterID)
}

func (s *ScoringService) scoreCluster(cluster domainhotspot.Cluster, items []domainhotspot.ClusterItem) HotspotScore {
	now := s.now().UTC()

	sourceCountScore := s.computeSourceCountScore(items)
	freshnessScore := s.computeFreshnessScore(cluster, now)
	relevanceScore := s.computeRelevanceScore(items)
	propagationScore := s.computePropagationScore(items)
	qualityScore := s.computeQualityScore(items)

	totalScore := s.cfg.SourceCountWeight*sourceCountScore +
		s.cfg.FreshnessWeight*freshnessScore +
		s.cfg.RelevanceWeight*relevanceScore +
		s.cfg.PropagationWeight*propagationScore +
		s.cfg.QualityWeight*qualityScore

	uniqueSources := make(map[string]struct{})
	for _, item := range items {
		if item.SourceID != "" {
			uniqueSources[item.SourceID] = struct{}{}
		}
	}

	avgSim := 0.0
	for _, item := range items {
		avgSim += item.Similarity
	}
	if len(items) > 0 {
		avgSim /= float64(len(items))
	}

	hoursSinceLatest := now.Sub(cluster.UpdatedAt).Hours()
	if hoursSinceLatest < 0 {
		hoursSinceLatest = 0
	}

	explanation := ScoreExplanation{
		SourceCount:      len(items),
		UniqueSources:    len(uniqueSources),
		AvgSimilarity:    avgSim,
		HoursSinceLatest: hoursSinceLatest,
		KeywordCount:     len(cluster.Keywords),
		Weights:          s.cfg,
	}
	explanationJSON, _ := json.Marshal(explanation)

	return HotspotScore{
		ID:               "score-" + cluster.ID,
		ClusterID:        cluster.ID,
		TotalScore:       totalScore,
		SourceCountScore: sourceCountScore,
		FreshnessScore:   freshnessScore,
		RelevanceScore:   relevanceScore,
		PropagationScore: propagationScore,
		QualityScore:     qualityScore,
		Explanation:      string(explanationJSON),
		ScoreVersion:     "v1",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// computeSourceCountScore returns a 0-1 score based on the number of unique source items.
// Uses logarithmic scaling so that going from 1->2 items matters more than 10->11.
func (s *ScoringService) computeSourceCountScore(items []domainhotspot.ClusterItem) float64 {
	if len(items) == 0 {
		return 0
	}
	// log2(n+1) / log2(max+1), capped at 1
	count := float64(len(items))
	return math.Min(math.Log2(count+1)/math.Log2(11), 1.0)
}

// computeFreshnessScore returns a 0-1 score based on how recently the cluster was updated.
// Decays exponentially with FreshnessDecayHours half-life.
func (s *ScoringService) computeFreshnessScore(cluster domainhotspot.Cluster, now time.Time) float64 {
	hoursSince := now.Sub(cluster.UpdatedAt).Hours()
	if hoursSince < 0 {
		hoursSince = 0
	}
	return math.Exp(-hoursSince / s.cfg.FreshnessDecayHours)
}

// computeRelevanceScore returns a 0-1 score based on the average similarity of items in the cluster.
func (s *ScoringService) computeRelevanceScore(items []domainhotspot.ClusterItem) float64 {
	if len(items) == 0 {
		return 0
	}
	total := 0.0
	for _, item := range items {
		total += item.Similarity
	}
	return total / float64(len(items))
}

// computePropagationScore returns a 0-1 score based on how many items are in the cluster.
// More items = more propagation signal.
func (s *ScoringService) computePropagationScore(items []domainhotspot.ClusterItem) float64 {
	if len(items) <= 1 {
		return 0
	}
	return math.Min(float64(len(items)-1)/9.0, 1.0)
}

// computeQualityScore returns a 0-1 quality signal.
// Currently based on average similarity as a proxy for content coherence.
func (s *ScoringService) computeQualityScore(items []domainhotspot.ClusterItem) float64 {
	if len(items) == 0 {
		return 0
	}
	total := 0.0
	for _, item := range items {
		total += item.Similarity
	}
	avg := total / float64(len(items))
	// Quality is high when average similarity is high
	return avg
}
