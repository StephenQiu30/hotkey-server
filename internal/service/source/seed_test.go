package source

import (
	"context"
	"testing"
)

func TestSeedSourcesCreatesBuiltinSources(t *testing.T) {
	repo := NewMemoryRepository()
	svc := NewService(repo)

	count, err := svc.SeedSources(context.Background())
	if err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one source to be seeded")
	}

	sources, err := svc.ListSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != count {
		t.Fatalf("expected %d sources, got %d", count, len(sources))
	}

	// Verify all sources are enabled
	for _, s := range sources {
		if s.Status != SourceStatusEnabled {
			t.Fatalf("expected source %q to be enabled, got %q", s.Name, s.Status)
		}
		if s.Name == "" {
			t.Fatal("expected source name to be non-empty")
		}
		if s.URL == "" {
			t.Fatal("expected source URL to be non-empty")
		}
	}
}

func TestSeedSourcesIsIdempotent(t *testing.T) {
	repo := NewMemoryRepository()
	svc := NewService(repo)

	firstCount, err := svc.SeedSources(context.Background())
	if err != nil {
		t.Fatalf("first seed failed: %v", err)
	}

	secondCount, err := svc.SeedSources(context.Background())
	if err != nil {
		t.Fatalf("second seed failed: %v", err)
	}
	if secondCount != 0 {
		t.Fatalf("expected 0 new sources on second seed, got %d", secondCount)
	}

	sources, err := svc.ListSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != firstCount {
		t.Fatalf("expected %d sources after idempotent seed, got %d", firstCount, len(sources))
	}
}

func TestSeedSourcesIncludesRSSAndPublicPage(t *testing.T) {
	repo := NewMemoryRepository()
	svc := NewService(repo)

	_, err := svc.SeedSources(context.Background())
	if err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	sources, err := svc.ListSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	hasRSS := false
	hasPublicPage := false
	for _, s := range sources {
		switch s.Type {
		case SourceTypeRSS:
			hasRSS = true
		case SourceTypePublicPage:
			hasPublicPage = true
		}
	}
	if !hasRSS {
		t.Fatal("expected at least one RSS source in seed")
	}
	if !hasPublicPage {
		t.Fatal("expected at least one public_page source in seed")
	}
}
