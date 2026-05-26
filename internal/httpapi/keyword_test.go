package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/keyword"
	"github.com/gin-gonic/gin"
)

func TestAdminKeywordEndpointsCreateListAndDisableKeyword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithKeywordService(keyword.NewService())

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/keywords", bytes.NewBufferString(`{"term":"OpenAI","category":"lab"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body=%s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	id, ok := created["id"].(string)
	if !ok || id == "" {
		t.Fatalf("created id = %#v", created["id"])
	}

	disableReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/keywords/"+id, bytes.NewBufferString(`{"enabled":false}`))
	disableReq.Header.Set("Content-Type", "application/json")
	disableRec := httptest.NewRecorder()
	router.ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want %d; body=%s", disableRec.Code, http.StatusOK, disableRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/keywords", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	var listBody map[string][]map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	keywords := listBody["keywords"]
	if len(keywords) != 1 {
		t.Fatalf("keywords len = %d, want 1", len(keywords))
	}
	if keywords[0]["enabled"] != false {
		t.Fatalf("enabled = %#v, want false", keywords[0]["enabled"])
	}
}

func TestUserKeywordPreferenceEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithKeywordService(keyword.NewService())

	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/api/v1/keywords/follow", body: `{"userId":"user-1","term":"OpenAI"}`},
		{path: "/api/v1/keywords/block", body: `{"userId":"user-1","term":"AI slop"}`},
		{path: "/api/v1/keywords/additional", body: `{"userId":"user-1","term":"Claude Code"}`},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d; body=%s", tc.path, rec.Code, http.StatusOK, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/keywords/preferences?userId=user-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preferences status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		UserID             string   `json:"userId"`
		FollowedKeywords   []string `json:"followedKeywords"`
		BlockedKeywords    []string `json:"blockedKeywords"`
		AdditionalKeywords []string `json:"additionalKeywords"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode preferences response: %v", err)
	}
	if body.UserID != "user-1" {
		t.Fatalf("userId = %q, want user-1", body.UserID)
	}
	if len(body.FollowedKeywords) != 1 || body.FollowedKeywords[0] != "OpenAI" {
		t.Fatalf("followedKeywords = %#v", body.FollowedKeywords)
	}
	if len(body.BlockedKeywords) != 1 || body.BlockedKeywords[0] != "AI slop" {
		t.Fatalf("blockedKeywords = %#v", body.BlockedKeywords)
	}
	if len(body.AdditionalKeywords) != 1 || body.AdditionalKeywords[0] != "Claude Code" {
		t.Fatalf("additionalKeywords = %#v", body.AdditionalKeywords)
	}
}

func TestKeywordEndpointsReturnStructuredErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithKeywordService(keyword.NewService())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/keywords/follow", bytes.NewBufferString(`{"userId":"user-1","term":" "}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body["error"]["code"] != "invalid_keyword" {
		t.Fatalf("error code = %q, want invalid_keyword", body["error"]["code"])
	}
}
