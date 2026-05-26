package content

import (
	"testing"
	"time"
)

func TestIngestSourceItemStoresTraceableNormalizedFields(t *testing.T) {
	service := NewService()
	publishedAt := time.Date(2026, 5, 25, 8, 30, 0, 0, time.UTC)
	fetchedAt := time.Date(2026, 5, 25, 9, 0, 0, 0, time.UTC)

	item, result, err := service.IngestSourceItem(IngestSourceItemInput{
		SourceID:    "arxiv-ai",
		OriginalURL: "https://arxiv.org/abs/2605.00001?utm_source=newsletter",
		Title:       "  OpenAI releases a new model  ",
		Summary:     "Model release details",
		PublishedAt: publishedAt,
		FetchedAt:   fetchedAt,
		RawMetadata: map[string]string{"paperId": "2605.00001"},
	})
	if err != nil {
		t.Fatalf("IngestSourceItem returned error: %v", err)
	}
	if result != ResultCreated {
		t.Fatalf("result = %q, want %q", result, ResultCreated)
	}
	if item.SourceID != "arxiv-ai" {
		t.Fatalf("sourceId = %q, want arxiv-ai", item.SourceID)
	}
	if item.OriginalURL != "https://arxiv.org/abs/2605.00001?utm_source=newsletter" {
		t.Fatalf("originalUrl = %q", item.OriginalURL)
	}
	if item.CanonicalURL != "https://arxiv.org/abs/2605.00001" {
		t.Fatalf("canonicalUrl = %q, want query-stripped URL", item.CanonicalURL)
	}
	if item.Title != "OpenAI releases a new model" {
		t.Fatalf("title = %q", item.Title)
	}
	if item.ContentHash == "" {
		t.Fatalf("contentHash is empty")
	}
	if !item.PublishedAt.Equal(publishedAt) || !item.FetchedAt.Equal(fetchedAt) {
		t.Fatalf("time fields not preserved: published=%s fetched=%s", item.PublishedAt, item.FetchedAt)
	}
	if item.RawMetadata["paperId"] != "2605.00001" {
		t.Fatalf("rawMetadata not preserved: %#v", item.RawMetadata)
	}
}

func TestIngestSourceItemDeduplicatesByURLHashAndTitleWindow(t *testing.T) {
	service := NewService()
	baseTime := time.Date(2026, 5, 25, 8, 30, 0, 0, time.UTC)

	first, result, err := service.IngestSourceItem(IngestSourceItemInput{
		SourceID:    "arxiv-ai",
		OriginalURL: "https://example.com/a?utm_campaign=first",
		Title:       "OpenAI releases a new model",
		Summary:     "first summary",
		PublishedAt: baseTime,
		FetchedAt:   baseTime.Add(10 * time.Minute),
	})
	if err != nil || result != ResultCreated {
		t.Fatalf("first ingest result=%q err=%v", result, err)
	}

	duplicateURL, result, err := service.IngestSourceItem(IngestSourceItemInput{
		SourceID:    "github-trending-ai",
		OriginalURL: "https://example.com/a?utm_source=feed",
		Title:       "Different title",
		Summary:     "different summary",
		PublishedAt: baseTime.Add(30 * time.Minute),
		FetchedAt:   baseTime.Add(40 * time.Minute),
	})
	if err != nil || result != ResultDuplicate {
		t.Fatalf("duplicate URL result=%q err=%v", result, err)
	}
	if duplicateURL.ID != first.ID {
		t.Fatalf("duplicate URL id = %q, want %q", duplicateURL.ID, first.ID)
	}

	duplicateHash, result, err := service.IngestSourceItem(IngestSourceItemInput{
		SourceID:    "github-trending-ai",
		OriginalURL: "https://example.com/b",
		Title:       "OpenAI releases a new model",
		Summary:     "first summary",
		PublishedAt: baseTime.Add(20 * time.Minute),
		FetchedAt:   baseTime.Add(30 * time.Minute),
	})
	if err != nil || result != ResultDuplicate {
		t.Fatalf("duplicate hash result=%q err=%v", result, err)
	}
	if duplicateHash.ID != first.ID {
		t.Fatalf("duplicate hash id = %q, want %q", duplicateHash.ID, first.ID)
	}

	duplicateTitle, result, err := service.IngestSourceItem(IngestSourceItemInput{
		SourceID:    "github-trending-ai",
		OriginalURL: "https://example.com/c",
		Title:       "  openai   releases a new MODEL ",
		Summary:     "different summary",
		PublishedAt: baseTime.Add(2 * time.Hour),
		FetchedAt:   baseTime.Add(3 * time.Hour),
	})
	if err != nil || result != ResultDuplicate {
		t.Fatalf("duplicate title result=%q err=%v", result, err)
	}
	if duplicateTitle.ID != first.ID {
		t.Fatalf("duplicate title id = %q, want %q", duplicateTitle.ID, first.ID)
	}
}

func TestIngestSourceItemRejectsMissingTraceFields(t *testing.T) {
	service := NewService()

	_, _, err := service.IngestSourceItem(IngestSourceItemInput{
		SourceID:    "arxiv-ai",
		OriginalURL: " ",
		Title:       "OpenAI releases a new model",
		PublishedAt: time.Now(),
		FetchedAt:   time.Now(),
	})

	if err != ErrInvalidSourceItem {
		t.Fatalf("err = %v, want %v", err, ErrInvalidSourceItem)
	}
}
