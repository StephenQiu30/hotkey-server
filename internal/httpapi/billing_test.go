package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/billing"
	"github.com/gin-gonic/gin"
)

func TestBillingEndpointsAssignPlanRecordUsageAndSummarize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	planReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants/tenant-alpha/billing/plan", bytes.NewBufferString(`{
		"planId":"starter",
		"name":"Starter",
		"quotas":{"collection":2,"refresh":1,"ai_call":1}
	}`))
	planReq.Header.Set("Content-Type", "application/json")
	planRec := httptest.NewRecorder()
	router.ServeHTTP(planRec, planReq)
	if planRec.Code != http.StatusOK {
		t.Fatalf("assign plan status = %d, want %d; body=%s", planRec.Code, http.StatusOK, planRec.Body.String())
	}

	usageReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants/tenant-alpha/billing/usage", bytes.NewBufferString(`{"metric":"collection","amount":2}`))
	usageReq.Header.Set("Content-Type", "application/json")
	usageRec := httptest.NewRecorder()
	router.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusAccepted {
		t.Fatalf("usage status = %d, want %d; body=%s", usageRec.Code, http.StatusAccepted, usageRec.Body.String())
	}

	blockedReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants/tenant-alpha/billing/usage", bytes.NewBufferString(`{"metric":"collection","amount":1}`))
	blockedReq.Header.Set("Content-Type", "application/json")
	blockedRec := httptest.NewRecorder()
	router.ServeHTTP(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusPaymentRequired {
		t.Fatalf("blocked status = %d, want %d; body=%s", blockedRec.Code, http.StatusPaymentRequired, blockedRec.Body.String())
	}

	summaryReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tenants/tenant-alpha/billing/usage", nil)
	summaryRec := httptest.NewRecorder()
	router.ServeHTTP(summaryRec, summaryReq)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("summary status = %d, want %d; body=%s", summaryRec.Code, http.StatusOK, summaryRec.Body.String())
	}

	var summary billing.UsageSummary
	if err := json.Unmarshal(summaryRec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.Usage[billing.MetricCollection] != 2 {
		t.Fatalf("usage = %#v", summary.Usage)
	}
}
