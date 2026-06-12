package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupTestHandler() (*HTTPHandler, *fakeRepo) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	return NewHTTPHandler(svc), repo
}

func TestCreateEndpointRejectsInvalidInterval(t *testing.T) {
	h, _ := setupTestHandler()
	body := `{"name":"AI","query_text":"openai","poll_interval_minutes":7}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/monitors", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	// Set userID in context
	ctx := context.WithValue(req.Context(), "userID", int64(1))
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestCreateEndpointAcceptsValidInput(t *testing.T) {
	h, _ := setupTestHandler()
	body := `{"name":"AI","query_text":"openai","poll_interval_minutes":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/monitors", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), "userID", int64(1))
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	var m Monitor
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if m.Name != "AI" {
		t.Fatalf("expected name AI, got %s", m.Name)
	}
}

func TestListEndpointReturnsMonitors(t *testing.T) {
	repo := &fakeRepo{
		monitors: []Monitor{
			{ID: 1, UserID: 1, Name: "AI", Status: "active"},
			{ID: 2, UserID: 2, Name: "Crypto", Status: "active"},
		},
	}
	svc := NewService(repo)
	h := NewHTTPHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors", nil)
	ctx := context.WithValue(req.Context(), "userID", int64(1))
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var monitors []Monitor
	if err := json.NewDecoder(rr.Body).Decode(&monitors); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(monitors) != 1 {
		t.Fatalf("expected 1 monitor, got %d", len(monitors))
	}
}
