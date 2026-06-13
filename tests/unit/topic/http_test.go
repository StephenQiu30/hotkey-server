package topic_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/topic"
	faketopic "github.com/StephenQiu30/hotkey-server/tests/testutil/fake/topic"
)

func TestListTopicsReturnsCurrentHeatAndTrend(t *testing.T) {
	fake := &faketopic.QueryService{
		Topics: []topic.TopicSummary{
			{ID: 1, Title: "ai agents", Summary: "AI agent launches", CurrentHeat: 150.5, TrendDirection: "rising", PostCount: 10},
			{ID: 2, Title: "crypto defi", Summary: "DeFi updates", CurrentHeat: 80.0, TrendDirection: "flat", PostCount: 5},
		},
	}
	handler := topic.NewTopicHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors/1/topics", nil)
	rr := httptest.NewRecorder()
	handler.ListByMonitor(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp []topic.TopicSummary
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(resp))
	}
	if resp[0].Title != "ai agents" {
		t.Fatalf("expected 'ai agents', got '%s'", resp[0].Title)
	}
	if resp[0].TrendDirection != "rising" {
		t.Fatalf("expected 'rising', got '%s'", resp[0].TrendDirection)
	}
}

func TestListTopicsEmptyResult(t *testing.T) {
	fake := &faketopic.QueryService{}
	handler := topic.NewTopicHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors/1/topics", nil)
	rr := httptest.NewRecorder()
	handler.ListByMonitor(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp []topic.TopicSummary
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("expected 0 topics, got %d", len(resp))
	}
}
