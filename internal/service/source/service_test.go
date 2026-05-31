package source_test

import (
	"context"
	"errors"
	"testing"

	servicesource "github.com/StephenQiu30/hotkey-server/internal/service/source"
)

func TestSourceLifecycleValidationAndCollectionSelection(t *testing.T) {
	ctx := context.Background()
	svc := servicesource.NewService(servicesource.NewMemoryRepository())

	if _, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "Public Page",
		Type:             servicesource.SourceTypePublicPage,
		URL:              "https://example.com/ai",
		FetchIntervalMin: 30,
		RateLimitPerHour: 12,
		ChannelIDs:       []string{"chn_ai_models"},
	}); !errors.Is(err, servicesource.ErrComplianceNoteRequired) {
		t.Fatalf("expected public page compliance note error, got %v", err)
	}

	created, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "OpenAI Blog",
		Type:             servicesource.SourceTypeRSS,
		URL:              "https://example.com/rss.xml",
		FetchIntervalMin: 60,
		ChannelIDs:       []string{"chn_ai_models", "chn_ai_products"},
	})
	if err != nil {
		t.Fatalf("create rss source: %v", err)
	}
	if created.Status != servicesource.SourceStatusEnabled {
		t.Fatalf("expected enabled source, got %s", created.Status)
	}
	if len(created.ChannelIDs) != 2 {
		t.Fatalf("expected source channel links, got %#v", created.ChannelIDs)
	}

	updated, err := svc.UpdateSource(ctx, servicesource.UpdateSourceInput{
		SourceID:         created.ID,
		Name:             "OpenAI News",
		Type:             servicesource.SourceTypePublicPage,
		URL:              "https://example.com/news",
		ComplianceNote:   "Only collect publicly available pages respecting robots and rate limits.",
		FetchIntervalMin: 120,
		RateLimitPerHour: 6,
		ChannelIDs:       []string{"chn_ai_open_source"},
	})
	if err != nil {
		t.Fatalf("update source: %v", err)
	}
	if updated.Type != servicesource.SourceTypePublicPage || updated.ComplianceNote == "" || len(updated.ChannelIDs) != 1 {
		t.Fatalf("expected updated public page source with compliance and links, got %#v", updated)
	}
	if _, err := svc.UpdateSource(ctx, servicesource.UpdateSourceInput{
		SourceID:         "   ",
		Name:             "Invalid",
		Type:             servicesource.SourceTypeRSS,
		URL:              "https://example.com/invalid.xml",
		FetchIntervalMin: 30,
	}); !errors.Is(err, servicesource.ErrInvalidInput) {
		t.Fatalf("expected blank update source id to fail validation, got %v", err)
	}
	if _, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "FTP Feed",
		Type:             servicesource.SourceTypeRSS,
		URL:              "ftp://example.com/feed.xml",
		FetchIntervalMin: 30,
	}); !errors.Is(err, servicesource.ErrInvalidInput) {
		t.Fatalf("expected non-http source URL to fail validation, got %v", err)
	}

	selected, err := svc.ListCollectableSources(ctx)
	if err != nil {
		t.Fatalf("list collectable sources: %v", err)
	}
	if len(selected) != 1 || selected[0].ID != created.ID {
		t.Fatalf("expected enabled source to be collectable, got %#v", selected)
	}

	disabled, err := svc.SetSourceStatus(ctx, servicesource.SetSourceStatusInput{
		SourceID: created.ID,
		Status:   servicesource.SourceStatusDisabled,
	})
	if err != nil {
		t.Fatalf("disable source: %v", err)
	}
	if disabled.Status != servicesource.SourceStatusDisabled {
		t.Fatalf("expected disabled status, got %s", disabled.Status)
	}
	if _, err := svc.SetSourceStatus(ctx, servicesource.SetSourceStatusInput{
		SourceID: "   ",
		Status:   servicesource.SourceStatusEnabled,
	}); !errors.Is(err, servicesource.ErrInvalidInput) {
		t.Fatalf("expected blank status source id to fail validation, got %v", err)
	}

	selected, err = svc.ListCollectableSources(ctx)
	if err != nil {
		t.Fatalf("list collectable sources after disable: %v", err)
	}
	if len(selected) != 0 {
		t.Fatalf("expected disabled source to be excluded, got %#v", selected)
	}
}

func TestCollectionRunsRecordSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	svc := servicesource.NewService(servicesource.NewMemoryRepository())
	created, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "RSS",
		Type:             servicesource.SourceTypeRSS,
		URL:              "https://example.com/rss.xml",
		FetchIntervalMin: 30,
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	success, err := svc.RecordCollectionRun(ctx, servicesource.RecordCollectionRunInput{
		SourceID:     created.ID,
		Status:       servicesource.CollectionRunStatusSuccess,
		ItemsFetched: 3,
		StartedAt:    created.CreatedAt,
		FinishedAt:   created.CreatedAt.Add(2),
	})
	if err != nil {
		t.Fatalf("record success run: %v", err)
	}
	failure, err := svc.RecordCollectionRun(ctx, servicesource.RecordCollectionRunInput{
		SourceID:   created.ID,
		Status:     servicesource.CollectionRunStatusFailed,
		Error:      "upstream returned 500",
		StartedAt:  created.CreatedAt.Add(3),
		FinishedAt: created.CreatedAt.Add(4),
	})
	if err != nil {
		t.Fatalf("record failed run: %v", err)
	}

	runs, err := svc.ListCollectionRuns(ctx, created.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 || runs[0].ID != success.ID || runs[1].ID != failure.ID {
		t.Fatalf("expected success and failure runs in order, got %#v", runs)
	}
	if runs[0].ItemsFetched != 3 || runs[1].Error == "" {
		t.Fatalf("expected run details recorded, got %#v", runs)
	}
	if _, err := svc.RecordCollectionRun(ctx, servicesource.RecordCollectionRunInput{
		SourceID:     created.ID,
		Status:       servicesource.CollectionRunStatusSuccess,
		ItemsFetched: -1,
	}); !errors.Is(err, servicesource.ErrInvalidInput) {
		t.Fatalf("expected negative items fetched to fail validation, got %v", err)
	}
	if _, err := svc.RecordCollectionRun(ctx, servicesource.RecordCollectionRunInput{
		SourceID:   created.ID,
		Status:     servicesource.CollectionRunStatusSuccess,
		StartedAt:  created.CreatedAt.Add(2),
		FinishedAt: created.CreatedAt.Add(1),
	}); !errors.Is(err, servicesource.ErrInvalidInput) {
		t.Fatalf("expected inverted run time range to fail validation, got %v", err)
	}
}
