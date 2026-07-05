package baidu

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleBaiduHTML = `<html><head></head><body>
<script id="san-data">{
  "data": {
    "cards": [
      {
        "card": [
          {
            "title": "AI监管新规正式实施",
            "desc": "国家网信办发布人工智能监管新规...",
            "url": "/detail/123",
            "index": "1",
            "heatScore": "950000"
          },
          {
            "title": "华为发布新款麒麟芯片",
            "desc": "华为在深圳召开新品发布会...",
            "url": "/detail/456",
            "index": "2",
            "heatScore": "870000"
          },
          {
            "title": "国庆假期旅游数据出炉",
            "desc": "",
            "url": "",
            "index": "3",
            "heatScore": "780000"
          }
        ]
      }
    ]
  }
}</script>
</body></html>`

func TestFetchTrending_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(sampleBaiduHTML))
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
	if items[0].Title != "AI监管新规正式实施" {
		t.Errorf("items[0].Title = %q, want %q", items[0].Title, "AI监管新规正式实施")
	}
	if items[0].Rank != 1 {
		t.Errorf("items[0].Rank = %d, want 1", items[0].Rank)
	}
	if items[0].Heat != 950000 {
		t.Errorf("items[0].Heat = %f, want 950000", items[0].Heat)
	}
	if items[0].Platform != "baidu" {
		t.Errorf("items[0].Platform = %q, want 'baidu'", items[0].Platform)
	}
}

func TestFetchTrending_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body></body></html>`))
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		baseURL:    srv.URL,
	}

	_, err := client.FetchTrending(context.Background())
	if err == nil {
		t.Fatal("expected error for missing san-data")
	}
}

func TestFetchTrending_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		baseURL:    srv.URL,
	}

	_, err := client.FetchTrending(context.Background())
	if err == nil {
		t.Fatal("expected error for 503")
	}
}

func TestName(t *testing.T) {
	c := NewClient(0)
	if c.Name() != "baidu" {
		t.Errorf("Name() = %q, want 'baidu'", c.Name())
	}
}
