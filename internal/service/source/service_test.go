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

<<<<<<< HEAD
func TestXSourceTypeAcceptedByCreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	svc := servicesource.NewService(servicesource.NewMemoryRepository())

	created, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "X AI News",
		Type:             servicesource.SourceTypeX,
		URL:              "https://api.x.com/2/tweets/search/recent",
		ComplianceNote:   "X public API v2 recent search; OAuth 2.0 PKCE authorized.",
		FetchIntervalMin: 30,
		RateLimitPerHour: 30,
		ChannelIDs:       []string{"chn_ai_models"},
	})
	if err != nil {
		t.Fatalf("create x source: %v", err)
	}
	if created.Type != servicesource.SourceTypeX {
		t.Fatalf("expected x source type, got %s", created.Type)
	}
	if created.ComplianceNote == "" {
		t.Fatalf("expected compliance note for x source")
	}

	updated, err := svc.UpdateSource(ctx, servicesource.UpdateSourceInput{
		SourceID:         created.ID,
		Name:             "X AI News Updated",
		Type:             servicesource.SourceTypeX,
		URL:              "https://api.x.com/2/tweets/search/recent",
		ComplianceNote:   "X public API v2 recent search; OAuth 2.0 PKCE authorized.",
		FetchIntervalMin: 60,
		RateLimitPerHour: 15,
		ChannelIDs:       []string{"chn_ai_models", "chn_ai_products"},
	})
	if err != nil {
		t.Fatalf("update x source: %v", err)
	}
	if updated.Type != servicesource.SourceTypeX {
		t.Fatalf("expected x source type after update, got %s", updated.Type)
	}
	if updated.FetchIntervalMin != 60 {
		t.Fatalf("expected updated fetch interval 60, got %d", updated.FetchIntervalMin)
	}
}

func TestXSourceTypeRequiresComplianceNote(t *testing.T) {
	ctx := context.Background()
	svc := servicesource.NewService(servicesource.NewMemoryRepository())

	_, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "X No Compliance",
		Type:             servicesource.SourceTypeX,
		URL:              "https://api.x.com/2/tweets/search/recent",
		FetchIntervalMin: 30,
		RateLimitPerHour: 30,
	})
	if !errors.Is(err, servicesource.ErrComplianceNoteRequired) {
		t.Fatalf("expected compliance note required for x source, got %v", err)
	}
}

func TestXSourceTypeCollectableWhenEnabled(t *testing.T) {
	ctx := context.Background()
	svc := servicesource.NewService(servicesource.NewMemoryRepository())

	created, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "X Source",
		Type:             servicesource.SourceTypeX,
		URL:              "https://api.x.com/2/tweets/search/recent",
		ComplianceNote:   "X public API v2.",
		FetchIntervalMin: 30,
		RateLimitPerHour: 30,
	})
	if err != nil {
		t.Fatalf("create x source: %v", err)
	}

	collectable, err := svc.ListCollectableSources(ctx)
	if err != nil {
		t.Fatalf("list collectable: %v", err)
	}
	if len(collectable) != 1 || collectable[0].ID != created.ID {
		t.Fatalf("expected x source to be collectable, got %#v", collectable)
	}

	_, err = svc.SetSourceStatus(ctx, servicesource.SetSourceStatusInput{
		SourceID: created.ID,
		Status:   servicesource.SourceStatusDisabled,
	})
	if err != nil {
		t.Fatalf("disable x source: %v", err)
	}

	collectable, err = svc.ListCollectableSources(ctx)
	if err != nil {
		t.Fatalf("list collectable after disable: %v", err)
	}
	if len(collectable) != 0 {
		t.Fatalf("expected disabled x source excluded, got %#v", collectable)
	}
}

func TestHackerNewsSourceDoesNotRequireComplianceNote(t *testing.T) {
	ctx := context.Background()
	svc := servicesource.NewService(servicesource.NewMemoryRepository())

	created, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "Hacker News Top",
		Type:             servicesource.SourceTypeHackerNews,
		URL:              "https://hacker-news.firebaseio.com/v0/topstories.json",
		FetchIntervalMin: 30,
		ChannelIDs:       []string{"chn_hn"},
	})
	if err != nil {
		t.Fatalf("create hackernews source without compliance note: %v", err)
	}
	if created.Type != servicesource.SourceTypeHackerNews {
		t.Fatalf("expected hackernews type, got %s", created.Type)
	}
	if created.ComplianceNote != "" {
		t.Fatalf("expected empty compliance note for hackernews, got %q", created.ComplianceNote)
	}
	if created.Status != servicesource.SourceStatusEnabled {
		t.Fatalf("expected enabled status, got %s", created.Status)
	}

	collectable, err := svc.ListCollectableSources(ctx)
	if err != nil {
		t.Fatalf("list collectable: %v", err)
	}
	if len(collectable) != 1 || collectable[0].ID != created.ID {
		t.Fatalf("expected hackernews source to be collectable, got %#v", collectable)
	}
}

func TestWeChatMPSourceRequiresComplianceNote(t *testing.T) {
	ctx := context.Background()
	svc := servicesource.NewService(servicesource.NewMemoryRepository())

	if _, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "WeChat MP",
		Type:             servicesource.SourceTypeWeChatMP,
		URL:              "https://mp.weixin.qq.com/s/abc123",
		FetchIntervalMin: 60,
	}); !errors.Is(err, servicesource.ErrComplianceNoteRequired) {
		t.Fatalf("expected wechat_mp compliance note error, got %v", err)
	}

	created, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "WeChat MP",
		Type:             servicesource.SourceTypeWeChatMP,
		URL:              "https://mp.weixin.qq.com/s/abc123",
		ComplianceNote:   "Only collect publicly available WeChat articles.",
		FetchIntervalMin: 60,
		ChannelIDs:       []string{"chn_wechat"},
	})
	if err != nil {
		t.Fatalf("create wechat_mp source: %v", err)
	}
	if created.Type != servicesource.SourceTypeWeChatMP {
		t.Fatalf("expected wechat_mp type, got %s", created.Type)
	}
	if created.ComplianceNote == "" {
		t.Fatal("expected compliance note to be set")
	}
}

func TestXiaohongshuSourceRequiresComplianceNote(t *testing.T) {
	ctx := context.Background()
	svc := servicesource.NewService(servicesource.NewMemoryRepository())

	// xiaohongshu without compliance note should fail
	if _, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "小红书美食探店",
		Type:             servicesource.SourceTypeXiaohongshu,
		URL:              "https://www.xiaohongshu.com/user/profile/user-001",
		FetchIntervalMin: 30,
		RateLimitPerHour: 12,
	}); !errors.Is(err, servicesource.ErrComplianceNoteRequired) {
		t.Fatalf("expected xiaohongshu compliance note error, got %v", err)
	}

	// xiaohongshu with compliance note should succeed
	created, err := svc.CreateSource(ctx, servicesource.CreateSourceInput{
		Name:             "小红书美食探店",
		Type:             servicesource.SourceTypeXiaohongshu,
		URL:              "https://www.xiaohongshu.com/user/profile/user-001",
		ComplianceNote:   "Only collect publicly visible notes; respect rate limits and robots.txt.",
		FetchIntervalMin: 30,
		RateLimitPerHour: 12,
	})
	if err != nil {
		t.Fatalf("create xiaohongshu source with compliance note: %v", err)
	}
	if created.Type != servicesource.SourceTypeXiaohongshu {
		t.Fatalf("expected xiaohongshu source type, got %s", created.Type)
	}
	if created.ComplianceNote == "" {
		t.Fatal("expected compliance note to be stored")
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
