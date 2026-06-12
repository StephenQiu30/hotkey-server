package trend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeTrendQueryService struct {
	snapshots []TrendPoint
	err       error
}

func (f fakeTrendQueryService) GetTopicTrends(topicID int64, since time.Time) ([]TrendPoint, error) {
	return f.snapshots, f.err
}

func (f fakeTrendQueryService) GetMonitorTrends(monitorID int64, since time.Time) ([]TrendPoint, error) {
	return f.snapshots, f.err
}

func TestGetTopicTrendsReturnsSnapshots(t *testing.T) {
	fake := fakeTrendQueryService{
		snapshots: []TrendPoint{
			{Time: time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC), HeatScore: 100, TrendVelocity: 0.1, TrendDirection: "rising"},
			{Time: time.Date(2026, 6, 12, 11, 0, 0, 0, time.UTC), HeatScore: 120, TrendVelocity: 0.2, TrendDirection: "rising"},
		},
	}
	handler := NewTrendHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/1/trends", nil)
	rr := httptest.NewRecorder()
	handler.GetTopicTrends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp []TrendPoint
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 trend points, got %d", len(resp))
	}
	if resp[0].TrendDirection != "rising" {
		t.Fatalf("expected 'rising', got '%s'", resp[0].TrendDirection)
	}
}

func TestGetMonitorTrendsReturnsSnapshots(t *testing.T) {
	fake := fakeTrendQueryService{
		snapshots: []TrendPoint{
			{Time: time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC), HeatScore: 200, TrendVelocity: -0.1, TrendDirection: "falling"},
		},
	}
	handler := NewTrendHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors/10/trends", nil)
	rr := httptest.NewRecorder()
	handler.GetMonitorTrends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp []TrendPoint
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 trend point, got %d", len(resp))
	}
}

func TestGetTopicTrendsEmptyResult(t *testing.T) {
	fake := fakeTrendQueryService{snapshots: nil}
	handler := NewTrendHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/1/trends", nil)
	rr := httptest.NewRecorder()
	handler.GetTopicTrends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
