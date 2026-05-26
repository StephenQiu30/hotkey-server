package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/report"
	"github.com/gin-gonic/gin"
)

func TestReportEndpointsReturnPlatformAndUserDailyReports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	platformReq := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?date=2026-05-25", nil)
	platformRec := httptest.NewRecorder()
	router.ServeHTTP(platformRec, platformReq)
	if platformRec.Code != http.StatusOK {
		t.Fatalf("platform status = %d, want %d; body=%s", platformRec.Code, http.StatusOK, platformRec.Body.String())
	}

	var platform report.DailyReport
	if err := json.Unmarshal(platformRec.Body.Bytes(), &platform); err != nil {
		t.Fatalf("decode platform response: %v", err)
	}
	if platform.Scope != report.ScopePlatform {
		t.Fatalf("platform scope = %q", platform.Scope)
	}
	if len(platform.Items) == 0 {
		t.Fatalf("platform report has no items")
	}
	if len(platform.Items[0].EvidenceIDs) == 0 {
		t.Fatalf("platform report item has no evidence links")
	}

	userReq := httptest.NewRequest(http.MethodGet, "/api/v1/users/user-1/reports/daily?date=2026-05-25&keywords=OpenAI,model", nil)
	userRec := httptest.NewRecorder()
	router.ServeHTTP(userRec, userReq)
	if userRec.Code != http.StatusOK {
		t.Fatalf("user status = %d, want %d; body=%s", userRec.Code, http.StatusOK, userRec.Body.String())
	}

	var user report.DailyReport
	if err := json.Unmarshal(userRec.Body.Bytes(), &user); err != nil {
		t.Fatalf("decode user response: %v", err)
	}
	if user.Scope != report.ScopeUser || user.UserID != "user-1" {
		t.Fatalf("user report scope/user = %q/%q", user.Scope, user.UserID)
	}
	if len(user.Items) == 0 {
		t.Fatalf("user report has no items")
	}
}

func TestReportEndpointRejectsInvalidDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?date=bad-date", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
