package rss

import (
	"context"
	"encoding/xml"
	"errors"
	"strings"
	"testing"
	"time"

	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
)

func TestPublicChannelFeedRendersValidRSSFromReports(t *testing.T) {
	reports := servicereport.NewMemoryReportRepository()
	createdAt := time.Date(2026, 5, 31, 1, 2, 3, 0, time.UTC)
	_, err := reports.SaveReport(context.Background(), servicereport.DailyReport{
		ID:        "rpt-channel-1",
		Date:      "2026-05-31",
		ChannelID: "ai-models",
		Body:      "AI 模型日报正文",
		Status:    servicereport.ReportStatusSucceeded,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("save report: %v", err)
	}
	service := NewService(NewMemoryFeedRepository(), reports, Config{BaseURL: "https://hotkey.test"})

	feed, err := service.PublicChannelFeed(context.Background(), "ai-models")
	if err != nil {
		t.Fatalf("public feed: %v", err)
	}
	body, err := feed.XML()
	if err != nil {
		t.Fatalf("rss xml: %v", err)
	}
	var decoded rssDocument
	if err := xml.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("rss is invalid xml: %v\n%s", err, string(body))
	}
	if decoded.Channel.Title != "HotKey ai-models 日报" || len(decoded.Channel.Items) != 1 {
		t.Fatalf("unexpected feed: %#v", decoded.Channel)
	}
	item := decoded.Channel.Items[0]
	if item.GUID.Value != "daily-report:rpt-channel-1" || item.GUID.IsPermaLink || !strings.Contains(item.Description, "AI 模型日报正文") {
		t.Fatalf("unexpected item: %#v", item)
	}
}

func TestPrivateTokenLifecycleControlsFeedAccess(t *testing.T) {
	ctx := context.Background()
	reports := servicereport.NewMemoryReportRepository()
	createdAt := time.Date(2026, 5, 31, 1, 2, 3, 0, time.UTC)
	_, err := reports.SaveReport(ctx, servicereport.DailyReport{
		ID:        "rpt-user-1",
		Date:      "2026-05-31",
		UserID:    "usr-1",
		Body:      "用户私有日报",
		Status:    servicereport.ReportStatusSucceeded,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("save report: %v", err)
	}
	accessedAt := time.Date(2026, 5, 31, 2, 3, 4, 0, time.UTC)
	service := NewService(NewMemoryFeedRepository(), reports, Config{
		BaseURL: "https://hotkey.test",
		Now:     func() time.Time { return accessedAt },
	})

	feed, token, err := service.ResetUserFeed(ctx, "usr-1")
	if err != nil {
		t.Fatalf("reset feed: %v", err)
	}
	if token == "" || feed.TokenHash == token {
		t.Fatalf("expected opaque token and stored hash, got token=%q feed=%#v", token, feed)
	}
	privateFeed, err := service.PrivateUserFeed(ctx, token)
	if err != nil {
		t.Fatalf("private feed: %v", err)
	}
	if len(privateFeed.Channel.Items) != 1 || privateFeed.Channel.Items[0].GUID.Value != "daily-report:rpt-user-1" {
		t.Fatalf("unexpected private feed: %#v", privateFeed.Channel.Items)
	}
	touchedFeed, err := service.UserFeed(ctx, "usr-1")
	if err != nil {
		t.Fatalf("get touched feed: %v", err)
	}
	if touchedFeed.LastAccessedAt == nil || !touchedFeed.LastAccessedAt.Equal(accessedAt) || !touchedFeed.UpdatedAt.Equal(accessedAt) {
		t.Fatalf("expected touch to update last_accessed_at and updated_at, got %#v", touchedFeed)
	}

	_, newToken, err := service.ResetUserFeed(ctx, "usr-1")
	if err != nil {
		t.Fatalf("reset token: %v", err)
	}
	if newToken == token {
		t.Fatal("expected reset to rotate token")
	}
	if _, err := service.PrivateUserFeed(ctx, token); !errors.Is(err, ErrFeedNotFound) {
		t.Fatalf("old token should be invalid, got %v", err)
	}

	if err := service.DisableUserFeed(ctx, "usr-1"); err != nil {
		t.Fatalf("disable feed: %v", err)
	}
	if _, err := service.PrivateUserFeed(ctx, newToken); !errors.Is(err, ErrFeedDisabled) {
		t.Fatalf("disabled feed should fail, got %v", err)
	}
}

func TestEmptyPublicFeedIsValidRSS(t *testing.T) {
	service := NewService(NewMemoryFeedRepository(), servicereport.NewMemoryReportRepository(), Config{BaseURL: "https://hotkey.test"})
	feed, err := service.PublicChannelFeed(context.Background(), "missing")
	if err != nil {
		t.Fatalf("public feed: %v", err)
	}
	body, err := feed.XML()
	if err != nil {
		t.Fatalf("rss xml: %v", err)
	}
	var decoded rssDocument
	if err := xml.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("empty rss is invalid xml: %v\n%s", err, string(body))
	}
	if len(decoded.Channel.Items) != 0 {
		t.Fatalf("expected empty feed, got %#v", decoded.Channel.Items)
	}
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	GUID        rssGUID `xml:"guid"`
	Description string  `xml:"description"`
}

type rssGUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}
