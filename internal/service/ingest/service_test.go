package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/service/filter"
	"github.com/StephenQiu30/hotkey-server/internal/service/normalize"
	"github.com/StephenQiu30/hotkey-server/internal/service/quality"
)

func TestIngestCreatesPrimaryItemAndEmbeddingJob(t *testing.T) {
	repo := content.NewMemoryRepository()
	jobQueue := &recordingQueue{}
	service := NewService(repo, jobQueue)

	result, err := service.Ingest(context.Background(), Input{
		SourceID:    "src-1",
		Title:       "AI 新闻",
		Snippet:     "正文片段",
		URL:         "https://example.com/a?utm_source=rss",
		Language:    "zh",
		PublishedAt: ptrTime(time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if !result.Created || result.Item.Status != content.ItemStatusPrimary {
		t.Fatalf("expected primary created item, got %+v", result)
	}
	if result.Item.CanonicalURL != "https://example.com/a" {
		t.Fatalf("unexpected canonical URL: %q", result.Item.CanonicalURL)
	}
	if len(jobQueue.requests) != 1 {
		t.Fatalf("expected embedding job, got %d", len(jobQueue.requests))
	}
	if jobQueue.requests[0].Type != queue.JobTypeGenerateEmbedding {
		t.Fatalf("unexpected job type: %s", jobQueue.requests[0].Type)
	}
	var payload queue.GenerateEmbeddingPayload
	if err := json.Unmarshal(jobQueue.requests[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ItemID != result.Item.ID {
		t.Fatalf("expected item id payload %q, got %q", result.Item.ID, payload.ItemID)
	}
}

func TestIngestKeepsSinglePrimaryForSameCanonicalURL(t *testing.T) {
	repo := content.NewMemoryRepository()
	service := NewService(repo, &recordingQueue{})
	ctx := context.Background()
	input := Input{SourceID: "src-1", Title: "AI 新闻", Snippet: "正文片段", URL: "https://example.com/a?utm_campaign=x", Language: "zh"}

	first, err := service.Ingest(ctx, input)
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	second, err := service.Ingest(ctx, Input{SourceID: "src-1", Title: "AI 新闻更新", Snippet: "另一个正文片段", URL: "https://example.com/a?utm_source=rss", Language: "zh"})
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	if second.Created {
		t.Fatalf("expected canonical duplicate to reuse existing item, got %+v", second)
	}
	items, err := repo.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != first.Item.ID || items[0].Status != content.ItemStatusPrimary {
		t.Fatalf("expected one primary item, got %+v", items)
	}
}

func TestIngestHandlesConcurrentCanonicalCreateRace(t *testing.T) {
	existing := content.SourceItem{
		ID:           "item-existing",
		SourceID:     "src-1",
		Title:        "AI 新闻",
		Snippet:      "正文片段",
		RawURL:       "https://example.com/a",
		CanonicalURL: "https://example.com/a",
		ContentHash:  content.ContentHash(content.HashInput{Title: "AI 新闻", Snippet: "正文片段"}),
		Language:     "zh",
		Status:       content.ItemStatusPrimary,
	}
	jobQueue := &recordingQueue{}
	service := NewService(&canonicalRaceRepository{existing: existing}, jobQueue)

	result, err := service.Ingest(context.Background(), Input{
		SourceID: "src-1",
		Title:    "AI 新闻",
		Snippet:  "正文片段",
		URL:      "https://example.com/a",
		Language: "zh",
	})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if result.Created || result.Item.ID != existing.ID {
		t.Fatalf("expected existing item after race, got %+v", result)
	}
	if len(jobQueue.requests) != 0 {
		t.Fatalf("expected no embedding job for existing item, got %d", len(jobQueue.requests))
	}
}

func TestIngestMarksSameHashDifferentURLAsDuplicate(t *testing.T) {
	repo := content.NewMemoryRepository()
	service := NewService(repo, &recordingQueue{})
	ctx := context.Background()
	first, err := service.Ingest(ctx, Input{SourceID: "src-1", Title: "AI 新闻", Snippet: "正文片段", URL: "https://example.com/a", Language: "zh"})
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	second, err := service.Ingest(ctx, Input{SourceID: "src-1", Title: "AI 新闻", Snippet: "正文片段", URL: "https://mirror.example.com/a", Language: "zh"})
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	if !second.Created || second.Item.Status != content.ItemStatusDuplicate || second.Item.DuplicateOfItemID != first.Item.ID {
		t.Fatalf("expected duplicate linked to primary %q, got %+v", first.Item.ID, second.Item)
	}
}

func TestIngestRejectsInvalidInputs(t *testing.T) {
	service := NewService(content.NewMemoryRepository(), &recordingQueue{})
	tests := []Input{
		{SourceID: "src-1", Title: "", Snippet: "正文片段", URL: "https://example.com/a"},
		{SourceID: "src-1", Title: "AI 新闻", Snippet: "", URL: "https://example.com/a"},
		{SourceID: "src-1", Title: "AI 新闻", Snippet: "正文片段", URL: "not a url"},
	}
	for _, tt := range tests {
		if _, err := service.Ingest(context.Background(), tt); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput for %+v, got %v", tt, err)
		}
	}
}

type recordingQueue struct {
	requests []queue.EnqueueRequest
}

func (q *recordingQueue) Enqueue(_ context.Context, req queue.EnqueueRequest) (queue.Job, error) {
	q.requests = append(q.requests, req)
	return queue.Job{ID: "job-1", Type: req.Type, Payload: req.Payload, IdempotencyKey: req.IdempotencyKey}, nil
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

type canonicalRaceRepository struct {
	existing     content.SourceItem
	canonicalHit bool
}

func (r *canonicalRaceRepository) FindByCanonicalURL(_ context.Context, canonicalURL string) (content.SourceItem, error) {
	if r.canonicalHit && canonicalURL == r.existing.CanonicalURL {
		return r.existing, nil
	}
	r.canonicalHit = true
	return content.SourceItem{}, content.ErrNotFound
}

func (r *canonicalRaceRepository) FindByID(_ context.Context, id string) (content.SourceItem, error) {
	if id == r.existing.ID {
		return r.existing, nil
	}
	return content.SourceItem{}, content.ErrNotFound
}

func (r *canonicalRaceRepository) FindByContentHash(_ context.Context, contentHash string) (content.SourceItem, error) {
	if contentHash == r.existing.ContentHash {
		return r.existing, nil
	}
	return content.SourceItem{}, content.ErrNotFound
}

func (r *canonicalRaceRepository) Create(context.Context, content.SourceItem) (content.SourceItem, error) {
	return content.SourceItem{}, content.ErrAlreadyExists
}

func TestIngestPipelineNormalizesCleansAndFilters(t *testing.T) {
	repo := content.NewMemoryRepository()
	jobQueue := &recordingQueue{}

	normalizr := normalize.NewService(normalize.DefaultConfig())
	filterSvc := filter.NewService(filter.Config{
		Keywords:        []string{"AI", "人工智能"},
		ExcludeWords:    []string{"广告"},
		MinTitleRunes:   1,
		MinSnippetRunes: 1,
	})
	qualitySvc := quality.NewService(quality.DefaultConfig())

	service := NewService(repo, jobQueue, WithNormalize(normalizr), WithFilter(filterSvc), WithQuality(qualitySvc))

	// HTML content should be cleaned, language detected, and keyword matched
	result, err := service.Ingest(context.Background(), Input{
		SourceID: "src-1",
		Title:    "<b>AI</b>  新闻标题",
		Snippet:  "<p>人工智能最新进展</p>",
		URL:      "https://example.com/news?utm_source=rss",
	})
	if err != nil {
		t.Fatalf("pipeline ingest failed: %v", err)
	}
	if !result.Created {
		t.Fatal("expected item to be created")
	}
	if result.Item.Title == "<b>AI</b>  新闻标题" {
		t.Fatalf("expected HTML to be cleaned, got %q", result.Item.Title)
	}
	if result.Item.Language != "zh" {
		t.Fatalf("expected language zh, got %q", result.Item.Language)
	}
	if result.Item.CanonicalURL != "https://example.com/news" {
		t.Fatalf("expected tracking params removed, got %q", result.Item.CanonicalURL)
	}
	if result.Item.FilterStatus != content.ItemFilterStatusPassed {
		t.Fatalf("expected filter status passed, got %q", result.Item.FilterStatus)
	}
	if result.Item.QualityScore <= 0 {
		t.Fatalf("expected positive quality score, got %f", result.Item.QualityScore)
	}
}

func TestIngestPipelineRejectsFilteredContent(t *testing.T) {
	repo := content.NewMemoryRepository()
	filterSvc := filter.NewService(filter.Config{
		Keywords:        []string{"AI"},
		ExcludeWords:    []string{"广告"},
		MinTitleRunes:   1,
		MinSnippetRunes: 1,
	})

	service := NewService(repo, &recordingQueue{}, WithFilter(filterSvc))

	result, err := service.Ingest(context.Background(), Input{
		SourceID: "src-1",
		Title:    "AI 广告推广",
		Snippet:  "正文片段",
		URL:      "https://example.com/ad",
	})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if result.Created {
		t.Fatal("expected filtered content to not be created")
	}
	if result.Item.FilterStatus != content.ItemFilterStatusFiltered {
		t.Fatalf("expected filter status filtered, got %q", result.Item.FilterStatus)
	}
	if result.Item.FilterReason == "" {
		t.Fatal("expected non-empty filter reason for filtered content")
	}
	items, err := repo.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items in repo, got %d", len(items))
	}
}

func TestIngestPipelineDeduplicatesAcrossPlatforms(t *testing.T) {
	repo := content.NewMemoryRepository()
	jobQueue := &recordingQueue{}
	normalizr := normalize.NewService(normalize.DefaultConfig())

	service := NewService(repo, jobQueue, WithNormalize(normalizr))

	// Same content from different host URLs → different canonical URLs but same content hash
	first, err := service.Ingest(context.Background(), Input{
		SourceID: "src-rss",
		Title:    "AI 新闻",
		Snippet:  "正文片段",
		URL:      "https://example.com/article",
		Language: "zh",
	})
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	if !first.Created || first.Item.Status != content.ItemStatusPrimary {
		t.Fatalf("expected primary created, got %+v", first)
	}

	second, err := service.Ingest(context.Background(), Input{
		SourceID: "src-web",
		Title:    "AI 新闻",
		Snippet:  "正文片段",
		URL:      "https://mirror.example.com/article",
		Language: "zh",
	})
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	// Different canonical URLs, same content hash → created as duplicate linked to primary
	if !second.Created || second.Item.Status != content.ItemStatusDuplicate {
		t.Fatalf("expected duplicate created, got %+v", second)
	}
	if second.Item.DuplicateOfItemID != first.Item.ID {
		t.Fatalf("expected linked to primary %q, got %q", first.Item.ID, second.Item.DuplicateOfItemID)
	}
	// Only the primary item gets an embedding job
	if len(jobQueue.requests) != 1 {
		t.Fatalf("expected 1 embedding job (only primary), got %d", len(jobQueue.requests))
	}
}
