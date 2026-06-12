package content

import (
	"context"
	"testing"
)

func TestUpsertMonitorHitCreatesHit(t *testing.T) {
	repo := &fakeHitRepo{}
	err := repo.UpsertHit(context.Background(), MonitorHit{
		MonitorID:      1,
		PostID:         2,
		MatchedKeywords: []string{"openai", "agent"},
		RelevanceScore: 0.9,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(repo.hits))
	}
	if repo.hits[0].MonitorID != 1 {
		t.Errorf("expected monitor ID 1, got %d", repo.hits[0].MonitorID)
	}
	if repo.hits[0].PostID != 2 {
		t.Errorf("expected post ID 2, got %d", repo.hits[0].PostID)
	}
}

func TestUpsertMonitorHitDeduplicates(t *testing.T) {
	repo := &fakeHitRepo{}
	hit := MonitorHit{MonitorID: 1, PostID: 2, RelevanceScore: 0.8}

	// First insert
	err := repo.UpsertHit(context.Background(), hit)
	if err != nil {
		t.Fatalf("unexpected error on first upsert: %v", err)
	}

	// Second insert with same monitor+post should update, not duplicate
	hit.RelevanceScore = 0.95
	err = repo.UpsertHit(context.Background(), hit)
	if err != nil {
		t.Fatalf("unexpected error on second upsert: %v", err)
	}

	if len(repo.hits) != 1 {
		t.Fatalf("expected 1 hit after dedup, got %d", len(repo.hits))
	}
	if repo.hits[0].RelevanceScore != 0.95 {
		t.Errorf("expected updated relevance 0.95, got %f", repo.hits[0].RelevanceScore)
	}
}

func TestUpsertMonitorHitMultipleMonitors(t *testing.T) {
	repo := &fakeHitRepo{}

	_ = repo.UpsertHit(context.Background(), MonitorHit{MonitorID: 1, PostID: 2})
	_ = repo.UpsertHit(context.Background(), MonitorHit{MonitorID: 2, PostID: 2})

	if len(repo.hits) != 2 {
		t.Fatalf("expected 2 hits for different monitors, got %d", len(repo.hits))
	}
}

// fakeHitRepo for testing
type fakeHitRepo struct {
	hits []MonitorHit
}

func (f *fakeHitRepo) UpsertHit(_ context.Context, hit MonitorHit) error {
	// Check for existing hit with same monitor+post
	for i, existing := range f.hits {
		if existing.MonitorID == hit.MonitorID && existing.PostID == hit.PostID {
			f.hits[i] = hit
			return nil
		}
	}
	f.hits = append(f.hits, hit)
	return nil
}
