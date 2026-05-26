package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/report"
	"github.com/StephenQiu30/hotkey-server/internal/tenant"
	"github.com/gin-gonic/gin"
)

func TestTenantEndpointsCreateMembershipAndListUserTenants(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants", bytes.NewBufferString(`{"name":"Alpha Lab","slug":"alpha"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body=%s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created tenant.Tenant
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode tenant: %v", err)
	}

	memberReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants/"+created.ID+"/members", bytes.NewBufferString(`{"userId":"user-1","role":"owner"}`))
	memberReq.Header.Set("Content-Type", "application/json")
	memberRec := httptest.NewRecorder()
	router.ServeHTTP(memberRec, memberReq)
	if memberRec.Code != http.StatusCreated {
		t.Fatalf("member status = %d, want %d; body=%s", memberRec.Code, http.StatusCreated, memberRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/users/user-1/tenants", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var listBody struct {
		Tenants []tenant.Tenant `json:"tenants"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode tenants: %v", err)
	}
	if len(listBody.Tenants) != 1 || listBody.Tenants[0].ID != created.ID {
		t.Fatalf("tenants = %#v, want created tenant", listBody.Tenants)
	}
}

func TestTenantDailyReportEndpointIsTenantScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/tenant-alpha/reports/daily?date=2026-05-25", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var daily report.DailyReport
	if err := json.Unmarshal(rec.Body.Bytes(), &daily); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if daily.TenantID != "tenant-alpha" {
		t.Fatalf("tenantId = %q, want tenant-alpha", daily.TenantID)
	}
	for _, item := range daily.Items {
		if item.TenantID != "tenant-alpha" {
			t.Fatalf("non-tenant item in report: %#v", item)
		}
	}
}
