package fetcher_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
)

func TestXFetcherParsesItemsFromAPIResponse(t *testing.T) {
	response := xAPIResponse{
		Data: []xTweet{
			{
				ID:        "1234567890",
				Text:      "OpenAI releases GPT-5 with breakthrough reasoning capabilities",
				CreatedAt: "2026-06-07T10:00:00.000Z",
				AuthorID:  "author_1",
			},
			{
				ID:        "1234567891",
				Text:      "Google DeepMind announces Gemini 2.5 Ultra",
				CreatedAt: "2026-06-07T09:30:00.000Z",
				AuthorID:  "author_2",
			},
		},
		Meta: xMeta{ResultCount: 2},
	}
	body, _ := json.Marshal(response)

	client := fakeXHTTPClient(http.StatusOK, string(body), nil)

	xFetcher := fetcher.NewXFetcher(client, "test_access_token")
	items, err := xFetcher.Fetch(context.Background(), fetcher.Source{
		ID:             "src_x",
		Type:           fetcher.SourceTypeX,
		URL:            "https://api.x.com/2/tweets/search/recent?query=AI",
		ComplianceNote: "X public API v2.",
	})
	if err != nil {
		t.Fatalf("fetch x: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 x items, got %d", len(items))
	}

	item := items[0]
	if item.ExternalID != "1234567890" {
		t.Fatalf("expected external ID %q, got %q", "1234567890", item.ExternalID)
	}
	if item.Title != "OpenAI releases GPT-5 with breakthrough reasoning capabilities" {
		t.Fatalf("expected title from tweet text, got %q", item.Title)
	}
	if item.URL == "" {
		t.Fatalf("expected non-empty URL")
	}
	if item.PublishedAt == nil {
		t.Fatalf("expected parsed published_at")
	}
}

func TestXFetcherHandlesRateLimitResponse(t *testing.T) {
	client := fakeXHTTPClient(http.StatusTooManyRequests, `{"title":"Too Many Requests","detail":"Rate limit exceeded","type":"about:blank"}`, map[string]string{
		"x-rate-limit-reset": "1717750400",
	})

	xFetcher := fetcher.NewXFetcher(client, "test_access_token")
	_, err := xFetcher.Fetch(context.Background(), fetcher.Source{
		ID:             "src_x",
		Type:           fetcher.SourceTypeX,
		URL:            "https://api.x.com/2/tweets/search/recent?query=AI",
		ComplianceNote: "X public API v2.",
	})
	if err == nil {
		t.Fatalf("expected rate limit error, got nil")
	}
	var rateLimitErr *fetcher.RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
}

func TestXFetcherHandlesUnauthorizedResponse(t *testing.T) {
	client := fakeXHTTPClient(http.StatusUnauthorized, `{"errors":[{"message":"Invalid access token","code":89}]}`, nil)

	xFetcher := fetcher.NewXFetcher(client, "invalid_token")
	_, err := xFetcher.Fetch(context.Background(), fetcher.Source{
		ID:             "src_x",
		Type:           fetcher.SourceTypeX,
		URL:            "https://api.x.com/2/tweets/search/recent?query=AI",
		ComplianceNote: "X public API v2.",
	})
	if err == nil {
		t.Fatalf("expected unauthorized error, got nil")
	}
	var authErr *fetcher.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthError, got %T: %v", err, err)
	}
}

func TestXFetcherReturnsEmptyWhenNoData(t *testing.T) {
	response := xAPIResponse{
		Data: []xTweet{},
		Meta: xMeta{ResultCount: 0},
	}
	body, _ := json.Marshal(response)

	client := fakeXHTTPClient(http.StatusOK, string(body), nil)

	xFetcher := fetcher.NewXFetcher(client, "test_access_token")
	items, err := xFetcher.Fetch(context.Background(), fetcher.Source{
		ID:             "src_x",
		Type:           fetcher.SourceTypeX,
		URL:            "https://api.x.com/2/tweets/search/recent?query=nonexistent",
		ComplianceNote: "X public API v2.",
	})
	if err != nil {
		t.Fatalf("fetch x empty: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items for empty response, got %d", len(items))
	}
}

func TestXFetcherRequiresXSourceType(t *testing.T) {
	xFetcher := fetcher.NewXFetcher(http.DefaultClient, "token")
	_, err := xFetcher.Fetch(context.Background(), fetcher.Source{
		ID:             "src_rss",
		Type:           fetcher.SourceTypeRSS,
		URL:            "https://api.x.com/2/tweets/search/recent",
		ComplianceNote: "X public API v2.",
	})
	if err == nil {
		t.Fatalf("expected error for non-x source type, got nil")
	}
}

func TestXFetcherRequiresAccessToken(t *testing.T) {
	xFetcher := fetcher.NewXFetcher(http.DefaultClient, "")
	_, err := xFetcher.Fetch(context.Background(), fetcher.Source{
		ID:             "src_x",
		Type:           fetcher.SourceTypeX,
		URL:            "https://api.x.com/2/tweets/search/recent",
		ComplianceNote: "X public API v2.",
	})
	if err == nil {
		t.Fatalf("expected error for empty access token, got nil")
	}
}

func TestXFetcherHandlesEmptyDataField(t *testing.T) {
	client := fakeXHTTPClient(http.StatusOK, `{"meta":{"result_count":0}}`, nil)

	xFetcher := fetcher.NewXFetcher(client, "test_access_token")
	items, err := xFetcher.Fetch(context.Background(), fetcher.Source{
		ID:             "src_x",
		Type:           fetcher.SourceTypeX,
		URL:            "https://api.x.com/2/tweets/search/recent?query=AI",
		ComplianceNote: "X public API v2.",
	})
	if err != nil {
		t.Fatalf("fetch x no data field: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items when data field missing, got %d", len(items))
	}
}

// Helper types and functions for X fetcher tests.

type xTweet struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
	AuthorID  string `json:"author_id"`
}

type xMeta struct {
	ResultCount int `json:"result_count"`
}

type xAPIResponse struct {
	Data []xTweet `json:"data"`
	Meta xMeta    `json:"meta"`
}

func fakeXHTTPClient(status int, body string, headers map[string]string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Request:    req,
		}
		for k, v := range headers {
			resp.Header.Set(k, v)
		}
		return resp, nil
	})}
}
