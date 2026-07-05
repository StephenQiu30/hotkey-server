package zhihu

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleZhihuResponse = `{
  "data": [
    {
      "type": "hot-list_item",
      "id": "12345",
      "detail_text": "",
      "metrics_area": {
        "text": "697 万热度"
      },
      "target": {
        "id": 100001,
        "title": "如何看待AI监管新规的出台？",
        "url": "/question/100001",
        "excerpt": "近日国家网信办发布了AI监管新规...",
        "answer_count": 1234,
        "follower_count": 50000
      }
    },
    {
      "type": "hot-list_item",
      "id": "12346",
      "detail_text": "华为芯片突破引发热议",
      "metrics_area": {
        "text": "523 万热度"
      },
      "target": {
        "id": 100002,
        "title": "",
        "url": "/question/100002",
        "excerpt": "",
        "answer_count": 856,
        "follower_count": 30000
      }
    }
  ]
}`

func TestFetchTrending_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sampleZhihuResponse))
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		baseURL:    srv.URL,
	}

	items, err := client.FetchTrending(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	// First item: title from target.title
	if items[0].Title != "如何看待AI监管新规的出台？" {
		t.Errorf("items[0].Title = %q, want %q", items[0].Title, "如何看待AI监管新规的出台？")
	}
	if items[0].Rank != 1 {
		t.Errorf("items[0].Rank = %d, want 1", items[0].Rank)
	}
	if items[0].Heat != 6970000 {
		t.Errorf("items[0].Heat = %f, want 6970000", items[0].Heat)
	}
	if items[0].Platform != "zhihu" {
		t.Errorf("items[0].Platform = %q, want 'zhihu'", items[0].Platform)
	}

	// Second item: title from detail_text
	if items[1].Title != "华为芯片突破引发热议" {
		t.Errorf("items[1].Title = %q, want %q", items[1].Title, "华为芯片突破引发热议")
	}
}

func TestFetchTrending_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		baseURL:    srv.URL,
	}

	_, err := client.FetchTrending(context.Background())
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestFetchTrending_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		baseURL:    srv.URL,
	}

	_, err := client.FetchTrending(context.Background())
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestName(t *testing.T) {
	c := NewClient(0)
	if c.Name() != "zhihu" {
		t.Errorf("Name() = %q, want 'zhihu'", c.Name())
	}
}

func TestParseMetricsHeat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"697 万热度", 6970000},
		{"1234 万热度", 12340000},
		{"0 热度", 0},
		{"", 0},
	}
	for _, tc := range tests {
		got := parseMetricsHeat(tc.input)
		if got != tc.want {
			t.Errorf("parseMetricsHeat(%q) = %f, want %f", tc.input, got, tc.want)
		}
	}
}
