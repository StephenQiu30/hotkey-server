package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/keyword"
	"github.com/StephenQiu30/hotkey-server/internal/source"
	"github.com/gin-gonic/gin"
)

func TestAdminContentEndpointsIngestDeduplicateAndListItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithServices(keyword.NewService(), source.NewService(), content.NewService())

	body := `{
		"sourceId":"arxiv-ai",
		"originalUrl":"https://arxiv.org/abs/2605.00001?utm_source=newsletter",
		"title":"OpenAI releases a new model",
		"summary":"Model release details",
		"publishedAt":"2026-05-25T08:30:00Z",
		"fetchedAt":"2026-05-25T09:00:00Z",
		"rawMetadata":{"paperId":"2605.00001"}
	}`

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/source-items", bytes.NewBufferString(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body=%s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	duplicateReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/source-items", bytes.NewBufferString(body))
	duplicateReq.Header.Set("Content-Type", "application/json")
	duplicateRec := httptest.NewRecorder()
	router.ServeHTTP(duplicateRec, duplicateReq)
	if duplicateRec.Code != http.StatusOK {
		t.Fatalf("duplicate status = %d, want %d; body=%s", duplicateRec.Code, http.StatusOK, duplicateRec.Body.String())
	}

	var duplicateBody struct {
		Result string             `json:"result"`
		Item   content.SourceItem `json:"item"`
	}
	if err := json.Unmarshal(duplicateRec.Body.Bytes(), &duplicateBody); err != nil {
		t.Fatalf("decode duplicate response: %v", err)
	}
	if duplicateBody.Result != content.ResultDuplicate {
		t.Fatalf("result = %q, want %q", duplicateBody.Result, content.ResultDuplicate)
	}
	if duplicateBody.Item.CanonicalURL != "https://arxiv.org/abs/2605.00001" {
		t.Fatalf("canonicalUrl = %q", duplicateBody.Item.CanonicalURL)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/source-items", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}

	var listBody struct {
		Items []content.SourceItem `json:"items"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listBody.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(listBody.Items))
	}
	if listBody.Items[0].RawMetadata["paperId"] != "2605.00001" {
		t.Fatalf("rawMetadata not preserved: %#v", listBody.Items[0].RawMetadata)
	}
}
