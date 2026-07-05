package weibo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleWeiboResponse = `{
  "data": {
    "realtime": [
      {
        "word": "AI监管新规出台",
        "rank": 1,
        "hot_num": "1500000",
        "category": "科技",
        "url": "/weibo/123"
      },
      {
        "word": "华为发布新芯片",
        "rank": 2,
        "hot_num": "1200000",
        "category": "科技",
        "url": "/weibo/456"
      },
      {
        "word": "国庆旅游攻略",
        "rank": 3,
        "hot_num": "1000000",
        "category": "旅游",
        "url": ""
      }
    ]
  }
}`

func TestFetchTrending_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sampleWeiboResponse))
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
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	if items[0].Title != "AI监管新规出台" {
		t.Errorf("items[0].Title = %q, want %q", items[0].Title, "AI监管新规出台")
	}
	if items[0].Rank != 1 {
		t.Errorf("items[0].Rank = %d, want 1", items[0].Rank)
	}
	if items[0].Heat != 1500000 {
		t.Errorf("items[0].Heat = %f, want 1500000", items[0].Heat)
	}
	if items[0].Category != "科技" {
		t.Errorf("items[0].Category = %q, want %q", items[0].Category, "科技")
	}
	if items[0].Platform != "weibo" {
		t.Errorf("items[0].Platform = %q, want 'weibo'", items[0].Platform)
	}
}

func TestFetchTrending_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"realtime":[]}}`))
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
	if c.Name() != "weibo" {
		t.Errorf("Name() = %q, want 'weibo'", c.Name())
	}
}

func TestParseHeat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1500000", 1500000},
		{"", 0},
		{"热", 500000},
		{"沸", 800000},
		{"爆", 1000000},
		{"新", 100000},
		{"荐", 50000},
	}
	for _, tc := range tests {
		got := parseHeat(tc.input)
		if got != tc.want {
			t.Errorf("parseHeat(%q) = %f, want %f", tc.input, got, tc.want)
		}
	}
}
