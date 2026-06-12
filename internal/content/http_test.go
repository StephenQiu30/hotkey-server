package content

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePostQueryService struct {
	posts []PostSummary
	err   error
}

func (f fakePostQueryService) ListPostsByMonitor(monitorID int64, limit, offset int) ([]PostSummary, error) {
	return f.posts, f.err
}

func TestListPostsByMonitorReturnsContentFlow(t *testing.T) {
	fake := fakePostQueryService{
		posts: []PostSummary{
			{
				ID:              1,
				PlatformPostID:  "tw_001",
				AuthorName:      "Alice",
				AuthorHandle:    "@alice",
				ContentText:     "OpenAI launches new agent framework",
				HeatScore:       150.0,
				RelevanceScore:  0.9,
				FinalScore:      120.5,
				MatchedKeywords: []string{"openai", "agent"},
			},
			{
				ID:              2,
				PlatformPostID:  "tw_002",
				AuthorName:      "Bob",
				AuthorHandle:    "@bob",
				ContentText:     "AI agents are the future",
				HeatScore:       80.0,
				RelevanceScore:  0.7,
				FinalScore:      60.2,
				MatchedKeywords: []string{"ai", "agent"},
			},
		},
	}
	handler := NewPostHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors/1/posts", nil)
	rr := httptest.NewRecorder()
	handler.ListByMonitor(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp []PostSummary
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(resp))
	}
	if resp[0].AuthorHandle != "@alice" {
		t.Fatalf("expected '@alice', got '%s'", resp[0].AuthorHandle)
	}
	if resp[0].FinalScore != 120.5 {
		t.Fatalf("expected final score 120.5, got %f", resp[0].FinalScore)
	}
}

func TestListPostsByMonitorEmptyResult(t *testing.T) {
	fake := fakePostQueryService{posts: nil}
	handler := NewPostHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors/1/posts", nil)
	rr := httptest.NewRecorder()
	handler.ListByMonitor(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp []PostSummary
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("expected 0 posts, got %d", len(resp))
	}
}

func TestListPostsByMonitorInvalidID(t *testing.T) {
	fake := fakePostQueryService{}
	handler := NewPostHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors/abc/posts", nil)
	rr := httptest.NewRecorder()
	handler.ListByMonitor(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListPostsByMonitorRespectsLimitOffset(t *testing.T) {
	fake := fakePostQueryService{
		posts: []PostSummary{
			{ID: 1, PlatformPostID: "tw_001"},
		},
	}
	handler := NewPostHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors/1/posts?limit=5&offset=10", nil)
	rr := httptest.NewRecorder()
	handler.ListByMonitor(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
