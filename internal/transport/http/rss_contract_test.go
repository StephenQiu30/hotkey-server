package http_test

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
	servicerss "github.com/StephenQiu30/hotkey-server/internal/service/rss"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

func TestRSSHTTPContractForPublicAndPrivateFeeds(t *testing.T) {
	reportRepo := servicereport.NewMemoryReportRepository()
	createdAt := time.Date(2026, 5, 31, 1, 2, 3, 0, time.UTC)
	_, _ = reportRepo.SaveReport(context.Background(), servicereport.DailyReport{
		ID:        "rpt-channel-1",
		Date:      "2026-05-31",
		ChannelID: "ai-models",
		Body:      "公开频道日报",
		Status:    servicereport.ReportStatusSucceeded,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})
	rssService := servicerss.NewService(servicerss.NewMemoryFeedRepository(), reportRepo, servicerss.Config{BaseURL: "https://hotkey.test"})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{RSSService: rssService})

	public := getJSON(router, "/rss/channels/ai-models.xml")
	if public.Code != http.StatusOK || !strings.Contains(public.Header().Get("Content-Type"), "application/rss+xml") {
		t.Fatalf("expected rss response, got %d %s %s", public.Code, public.Header().Get("Content-Type"), public.Body.String())
	}
	var decoded struct {
		Channel struct {
			Items []struct {
				GUID struct {
					IsPermaLink bool   `xml:"isPermaLink,attr"`
					Value       string `xml:",chardata"`
				} `xml:"guid"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(public.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("public rss invalid: %v", err)
	}
	if len(decoded.Channel.Items) != 1 || decoded.Channel.Items[0].GUID.Value != "daily-report:rpt-channel-1" || decoded.Channel.Items[0].GUID.IsPermaLink {
		t.Fatalf("unexpected public items: %#v", decoded.Channel.Items)
	}

	token := registerAndLogin(t, router, "rss-user@example.com")
	reset := postJSONWithBearer(t, router, "/api/v1/me/rss/reset", token, map[string]string{})
	if reset.Code != http.StatusOK {
		t.Fatalf("expected reset 200, got %d %s", reset.Code, reset.Body.String())
	}
	rssToken := jsonStringAt(t, reset.Body.Bytes(), "token")
	userID := jsonStringAt(t, reset.Body.Bytes(), "userId")
	_, _ = reportRepo.SaveReport(context.Background(), servicereport.DailyReport{
		ID:        "rpt-user-1",
		Date:      "2026-05-31",
		UserID:    userID,
		Body:      "私有用户日报",
		Status:    servicereport.ReportStatusSucceeded,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})

	private := getJSON(router, "/rss/users/"+rssToken+".xml")
	if private.Code != http.StatusOK || !strings.Contains(private.Body.String(), "私有用户日报") {
		t.Fatalf("expected private rss, got %d %s", private.Code, private.Body.String())
	}
	disable := deleteWithBearer(router, "/api/v1/me/rss", token)
	if disable.Code != http.StatusNoContent {
		t.Fatalf("expected disable 204, got %d %s", disable.Code, disable.Body.String())
	}
	disabled := getJSON(router, "/rss/users/"+rssToken+".xml")
	if disabled.Code != http.StatusNotFound {
		t.Fatalf("expected disabled token to return 404, got %d %s", disabled.Code, disabled.Body.String())
	}
}

func TestDefaultRSSServiceSharesInjectedReportServiceRepository(t *testing.T) {
	reportRepo := servicereport.NewMemoryReportRepository()
	createdAt := time.Date(2026, 5, 31, 1, 2, 3, 0, time.UTC)
	_, _ = reportRepo.SaveReport(context.Background(), servicereport.DailyReport{
		ID:        "rpt-shared-1",
		Date:      "2026-05-31",
		ChannelID: "ai-models",
		Body:      "共享仓库日报",
		Status:    servicereport.ReportStatusSucceeded,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		ReportService: servicereport.NewService(reportRepo, nil, nil, nil, nil, nil),
	})

	public := getJSON(router, "/rss/channels/ai-models.xml")
	if public.Code != http.StatusOK || !strings.Contains(public.Body.String(), "共享仓库日报") {
		t.Fatalf("expected RSS to use injected report service repository, got %d %s", public.Code, public.Body.String())
	}
}

func TestPublicRSSRepositoryFailureReturnsServerError(t *testing.T) {
	rssService := servicerss.NewService(servicerss.NewMemoryFeedRepository(), failingRSSReportRepository{}, servicerss.Config{})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{RSSService: rssService})

	public := getJSON(router, "/rss/channels/ai-models.xml")
	if public.Code != http.StatusInternalServerError {
		t.Fatalf("expected repository failure to return 500, got %d %s", public.Code, public.Body.String())
	}
}

type failingRSSReportRepository struct{}

func (failingRSSReportRepository) ListReportsByChannel(context.Context, string) ([]servicereport.DailyReport, error) {
	return nil, errors.New("repository unavailable")
}

func (failingRSSReportRepository) ListReportsByUser(context.Context, string) ([]servicereport.DailyReport, error) {
	return nil, errors.New("repository unavailable")
}
