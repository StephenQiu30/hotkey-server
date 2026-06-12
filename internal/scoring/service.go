// Package scoring implements the hotspot scoring engine.
// It computes heat, relevance, freshness, and author influence scores
// for platform posts, then combines them into a final score.
package scoring

import "math"

// HeatInput holds engagement metrics for heat score computation.
type HeatInput struct {
	LikeCount   int
	ReplyCount  int
	RepostCount int
	QuoteCount  int
	ViewCount   int
}

// ComputeHeatScore returns a weighted engagement score.
// Formula: likes*1 + replies*3 + reposts*4 + quotes*4 + views*0.02
func ComputeHeatScore(in HeatInput) float64 {
	return float64(in.LikeCount)*1.0 +
		float64(in.ReplyCount)*3.0 +
		float64(in.RepostCount)*4.0 +
		float64(in.QuoteCount)*4.0 +
		float64(in.ViewCount)*0.02
}

// RelevanceInput holds keyword match data for relevance computation.
type RelevanceInput struct {
	MatchedKeywords []string
	TotalKeywords   int
	ContentLength   int
}

// ComputeRelevanceScore returns a score in [0,1] based on keyword coverage.
// Formula: matched/total, clamped to [0,1].
func ComputeRelevanceScore(in RelevanceInput) float64 {
	if in.TotalKeywords == 0 || len(in.MatchedKeywords) == 0 {
		return 0
	}
	ratio := float64(len(in.MatchedKeywords)) / float64(in.TotalKeywords)
	if ratio > 1 {
		return 1
	}
	return ratio
}

// FreshnessInput holds time-since-publish data.
type FreshnessInput struct {
	// PublishedAt is minutes since publication.
	PublishedAt float64
}

// ComputeFreshnessScore returns a decay score in (0,1].
// Uses exponential decay: e^(-minutes/1440) where 1440 = 24h.
func ComputeFreshnessScore(in FreshnessInput) float64 {
	return math.Exp(-in.PublishedAt / 1440.0)
}

// AuthorInput holds author metadata for influence scoring.
type AuthorInput struct {
	FollowersCount int
	Verified       bool
}

// ComputeAuthorInfluenceScore returns a score in (0,1] based on followers and verification.
// Formula: min(1, log10(followers+1)/6) * verifiedBoost.
// log10(1M)/6 = 1.0, so 1M followers = cap.
func ComputeAuthorInfluenceScore(in AuthorInput) float64 {
	base := math.Log10(float64(in.FollowersCount)+1.0) / 6.0
	if base > 1.0 {
		base = 1.0
	}
	if in.Verified {
		boost := base * 1.2
		if boost > 1.0 {
			return 1.0
		}
		return boost
	}
	return base
}

// ComputeFinalScore combines all dimension scores into a weighted final score.
// Weights: heat=0.4, relevance=0.3, freshness=0.2, authorInfluence=0.1.
func ComputeFinalScore(heat, relevance, freshness, authorInfluence float64) float64 {
	return heat*0.4 + relevance*0.3 + freshness*0.2 + authorInfluence*0.1
}

// SavedScore holds the computed scores to persist on a monitor_post_hits row.
type SavedScore struct {
	HeatScore           float64
	RelevanceScore      float64
	FreshnessScore      float64
	AuthorInfluenceScore float64
	FinalScore          float64
}

// HitRepository abstracts persistence for monitor_post_hits scoring columns.
type HitRepository interface {
	UpdateScores(hitID int64, score SavedScore) error
}

// ScoreHitInput aggregates all raw inputs needed to score a single hit.
type ScoreHitInput struct {
	HitID               int64
	MonitorID           int64
	PostID              int64
	LikeCount           int
	ReplyCount          int
	RepostCount         int
	QuoteCount          int
	ViewCount           int
	MatchedKeywords     []string
	TotalKeywords       int
	PublishedMinutesAgo float64
	AuthorFollowers     int
	AuthorVerified      bool
}

// Service orchestrates scoring computation and persistence.
type Service struct {
	repo HitRepository
}

// NewService creates a scoring Service with the given repository.
func NewService(repo HitRepository) *Service {
	return &Service{repo: repo}
}

// ScoreHit computes all dimension scores for a hit and persists them.
func (s *Service) ScoreHit(in ScoreHitInput) error {
	heat := ComputeHeatScore(HeatInput{
		LikeCount:   in.LikeCount,
		ReplyCount:  in.ReplyCount,
		RepostCount: in.RepostCount,
		QuoteCount:  in.QuoteCount,
		ViewCount:   in.ViewCount,
	})

	relevance := ComputeRelevanceScore(RelevanceInput{
		MatchedKeywords: in.MatchedKeywords,
		TotalKeywords:   in.TotalKeywords,
	})

	freshness := ComputeFreshnessScore(FreshnessInput{
		PublishedAt: in.PublishedMinutesAgo,
	})

	influence := ComputeAuthorInfluenceScore(AuthorInput{
		FollowersCount: in.AuthorFollowers,
		Verified:       in.AuthorVerified,
	})

	final := ComputeFinalScore(heat, relevance, freshness, influence)

	return s.repo.UpdateScores(in.HitID, SavedScore{
		HeatScore:           heat,
		RelevanceScore:      relevance,
		FreshnessScore:      freshness,
		AuthorInfluenceScore: influence,
		FinalScore:          final,
	})
}
