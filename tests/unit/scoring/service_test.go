package scoring_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/scoring"
	fakescoring "github.com/StephenQiu30/hotkey-server/tests/testutil/fake/scoring"
)

func TestComputeHeatScoreWeightsInteractions(t *testing.T) {
	score := scoring.ComputeHeatScore(scoring.HeatInput{
		LikeCount:   100,
		ReplyCount:  20,
		RepostCount: 10,
		QuoteCount:  5,
		ViewCount:   2000,
	})
	if score <= 0 {
		t.Fatalf("expected positive heat score, got %f", score)
	}
}

func TestComputeHeatScoreZeroInputs(t *testing.T) {
	score := scoring.ComputeHeatScore(scoring.HeatInput{})
	if score != 0 {
		t.Fatalf("expected zero heat score for zero inputs, got %f", score)
	}
}

func TestComputeHeatScoreLikesOnly(t *testing.T) {
	score := scoring.ComputeHeatScore(scoring.HeatInput{LikeCount: 10})
	// likes * 1.0 = 10
	if score != 10.0 {
		t.Fatalf("expected 10.0, got %f", score)
	}
}

func TestComputeHeatScoreRepliesWeightedHigher(t *testing.T) {
	likes := scoring.ComputeHeatScore(scoring.HeatInput{LikeCount: 10})
	replies := scoring.ComputeHeatScore(scoring.HeatInput{ReplyCount: 10})
	if replies <= likes {
		t.Fatalf("replies should be weighted higher than likes: replies=%f likes=%f", replies, likes)
	}
}

func TestComputeRelevanceScoreFromKeywords(t *testing.T) {
	score := scoring.ComputeRelevanceScore(scoring.RelevanceInput{
		MatchedKeywords: []string{"ai", "gpt"},
		TotalKeywords:   3,
		ContentLength:   200,
	})
	if score <= 0 || score > 1 {
		t.Fatalf("expected relevance in (0,1], got %f", score)
	}
}

func TestComputeRelevanceScoreNoKeywords(t *testing.T) {
	score := scoring.ComputeRelevanceScore(scoring.RelevanceInput{
		MatchedKeywords: []string{},
		TotalKeywords:   3,
	})
	if score != 0 {
		t.Fatalf("expected 0 relevance for no matched keywords, got %f", score)
	}
}

func TestComputeFreshnessScoreRecentPost(t *testing.T) {
	score := scoring.ComputeFreshnessScore(scoring.FreshnessInput{
		PublishedAt: 30, // 30 minutes ago
	})
	if score <= 0 || score > 1 {
		t.Fatalf("expected freshness in (0,1], got %f", score)
	}
}

func TestComputeFreshnessScoreOldPost(t *testing.T) {
	recent := scoring.ComputeFreshnessScore(scoring.FreshnessInput{PublishedAt: 30})
	old := scoring.ComputeFreshnessScore(scoring.FreshnessInput{PublishedAt: 1440}) // 24h ago
	if old >= recent {
		t.Fatalf("old post should have lower freshness: old=%f recent=%f", old, recent)
	}
}

func TestComputeAuthorInfluenceScoreHighFollowers(t *testing.T) {
	score := scoring.ComputeAuthorInfluenceScore(scoring.AuthorInput{
		FollowersCount: 100000,
		Verified:       true,
	})
	if score <= 0 || score > 1 {
		t.Fatalf("expected influence in (0,1], got %f", score)
	}
}

func TestComputeAuthorInfluenceScoreVerifiedBoosts(t *testing.T) {
	unverified := scoring.ComputeAuthorInfluenceScore(scoring.AuthorInput{FollowersCount: 1000, Verified: false})
	verified := scoring.ComputeAuthorInfluenceScore(scoring.AuthorInput{FollowersCount: 1000, Verified: true})
	if verified <= unverified {
		t.Fatalf("verified should boost influence: verified=%f unverified=%f", verified, unverified)
	}
}

func TestComputeFinalScoreWeighted(t *testing.T) {
	final := scoring.ComputeFinalScore(1.0, 0.8, 0.9, 0.5)
	if final <= 0 {
		t.Fatalf("expected positive final score, got %f", final)
	}
}

func TestComputeFinalScoreZeroInputs(t *testing.T) {
	final := scoring.ComputeFinalScore(0, 0, 0, 0)
	if final != 0 {
		t.Fatalf("expected 0 final score, got %f", final)
	}
}

func TestScoreHitComputesAndPersistsAllScores(t *testing.T) {
	repo := &fakescoring.HitRepo{}
	svc := scoring.NewService(repo)

	err := svc.ScoreHit(scoring.ScoreHitInput{
		HitID:               1,
		MonitorID:           10,
		PostID:              100,
		LikeCount:           50,
		ReplyCount:          10,
		RepostCount:         5,
		QuoteCount:          2,
		ViewCount:           1000,
		MatchedKeywords:     []string{"ai", "agent"},
		TotalKeywords:       3,
		PublishedMinutesAgo: 60,
		AuthorFollowers:     5000,
		AuthorVerified:      true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.Saved) != 1 {
		t.Fatalf("expected 1 saved score, got %d", len(repo.Saved))
	}
	s := repo.Saved[0]
	if s.HeatScore <= 0 {
		t.Errorf("expected positive heat, got %f", s.HeatScore)
	}
	if s.RelevanceScore <= 0 || s.RelevanceScore > 1 {
		t.Errorf("expected relevance in (0,1], got %f", s.RelevanceScore)
	}
	if s.FreshnessScore <= 0 || s.FreshnessScore > 1 {
		t.Errorf("expected freshness in (0,1], got %f", s.FreshnessScore)
	}
	if s.AuthorInfluenceScore <= 0 || s.AuthorInfluenceScore > 1 {
		t.Errorf("expected influence in (0,1], got %f", s.AuthorInfluenceScore)
	}
	if s.FinalScore <= 0 {
		t.Errorf("expected positive final score, got %f", s.FinalScore)
	}
}
