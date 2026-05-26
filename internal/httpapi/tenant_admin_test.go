package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/keyword"
	"github.com/StephenQiu30/hotkey-server/internal/source"
	"github.com/StephenQiu30/hotkey-server/internal/tenant"
	"github.com/gin-gonic/gin"
)

func TestTenantAdminCanManageTenantScopedKeywords(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants/tenant-alpha/keywords", bytes.NewBufferString(`{"term":"OpenAI","category":"lab"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body=%s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	betaReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants/tenant-beta/keywords", bytes.NewBufferString(`{"term":"Claude","category":"lab"}`))
	betaReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), betaReq)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tenants/tenant-alpha/keywords", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var body struct {
		Keywords []keyword.PlatformKeyword `json:"keywords"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode keywords: %v", err)
	}
	if len(body.Keywords) != 1 || body.Keywords[0].TenantID != "tenant-alpha" || body.Keywords[0].Term != "OpenAI" {
		t.Fatalf("tenant keywords = %#v", body.Keywords)
	}
}

func TestTenantAdminCanManageTenantScopedSources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants/tenant-alpha/sources", bytes.NewBufferString(`{
		"id":"alpha-feed",
		"name":"Alpha Feed",
		"layer":"fact",
		"region":"global",
		"language":"en",
		"categories":["ai"],
		"accessMode":"public_feed",
		"rateLimitPerHour":10,
		"refreshIntervalMinutes":60
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create source status = %d, want %d; body=%s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	enabled := false
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/tenants/tenant-alpha/sources/alpha-feed", bytes.NewBufferString(`{"enabled":false}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}
	_ = enabled

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tenants/tenant-alpha/sources", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	var body struct {
		Sources []source.Source `json:"sources"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode sources: %v", err)
	}
	if len(body.Sources) != 1 || body.Sources[0].TenantID != "tenant-alpha" || body.Sources[0].Enabled {
		t.Fatalf("tenant sources = %#v", body.Sources)
	}
}

func TestPlatformAdminCanListAllTenants(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	for _, body := range []string{
		`{"name":"Alpha Lab","slug":"alpha"}`,
		`{"name":"Beta Studio","slug":"beta"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tenants", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list tenants status = %d, want %d; body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var listBody struct {
		Tenants []tenant.Tenant `json:"tenants"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode tenants: %v", err)
	}
	if len(listBody.Tenants) != 2 {
		t.Fatalf("tenants len = %d, want 2", len(listBody.Tenants))
	}
}
