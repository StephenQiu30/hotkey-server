package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupTestHandler() (*Handler, *fakeRepo) {
	repo := newFakeRepo()
	svc := NewService(repo)
	return NewHandler(svc), repo
}

func requestWithUser(method, path string, body string, userID int64) *http.Request {
	var buf *bytes.Buffer
	if body != "" {
		buf = bytes.NewBufferString(body)
	} else {
		buf = bytes.NewBufferString("")
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(ContextWithUserID(context.Background(), userID))
}

func TestCreateMonitorHTTPSuccess(t *testing.T) {
	h, _ := setupTestHandler()
	body := `{"name":"AI News","query_text":"openai agent","poll_interval_minutes":10,"alert_enabled":true}`
	req := requestWithUser(http.MethodPost, "/api/v1/monitors", body, 1)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	var resp monitorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp.Name != "AI News" {
		t.Fatalf("expected name AI News, got %s", resp.Name)
	}
}

func TestCreateMonitorHTTPInvalidInterval(t *testing.T) {
	h, _ := setupTestHandler()
	body := `{"name":"AI","query_text":"openai","poll_interval_minutes":7}`
	req := requestWithUser(http.MethodPost, "/api/v1/monitors", body, 1)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListMonitorsHTTPSuccess(t *testing.T) {
	h, repo := setupTestHandler()
	repo.monitors = []Monitor{
		{ID: 1, UserID: 1, Name: "M1", QueryText: "q1", Status: "active", PollIntervalMinutes: 5},
		{ID: 2, UserID: 1, Name: "M2", QueryText: "q2", Status: "active", PollIntervalMinutes: 10},
		{ID: 3, UserID: 2, Name: "M3", QueryText: "q3", Status: "active", PollIntervalMinutes: 15},
	}

	req := requestWithUser(http.MethodGet, "/api/v1/monitors", "", 1)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp []monitorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(resp))
	}
}

func TestGetMonitorHTTPSuccess(t *testing.T) {
	h, repo := setupTestHandler()
	repo.monitors = []Monitor{
		{ID: 1, UserID: 1, Name: "M1", QueryText: "q1", Status: "active", PollIntervalMinutes: 5},
	}

	req := requestWithUser(http.MethodGet, "/api/v1/monitors/1", "", 1)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestGetMonitorHTTPForbidden(t *testing.T) {
	h, repo := setupTestHandler()
	repo.monitors = []Monitor{
		{ID: 1, UserID: 1, Name: "M1", QueryText: "q1", Status: "active", PollIntervalMinutes: 5},
	}

	req := requestWithUser(http.MethodGet, "/api/v1/monitors/1", "", 2)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestUpdateMonitorHTTPSuccess(t *testing.T) {
	h, repo := setupTestHandler()
	repo.monitors = []Monitor{
		{ID: 1, UserID: 1, Name: "M1", QueryText: "q1", Status: "active", PollIntervalMinutes: 5},
	}

	body := `{"status":"paused"}`
	req := requestWithUser(http.MethodPatch, "/api/v1/monitors/1", body, 1)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp monitorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp.Status != "paused" {
		t.Fatalf("expected status paused, got %s", resp.Status)
	}
}

func TestMonitorHTTPUnauthorized(t *testing.T) {
	h, _ := setupTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
