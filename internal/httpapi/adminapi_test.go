package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/adminapi"
	"github.com/StephenQiu30/hotkey-server/internal/report"
	"github.com/gin-gonic/gin"
)

func TestAdminAPIEndpointsListTaskRunsAndTriggerDailyReport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	triggerReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/reports/daily", bytes.NewBufferString(`{"date":"2026-05-25","scope":"platform"}`))
	triggerReq.Header.Set("Content-Type", "application/json")
	triggerRec := httptest.NewRecorder()
	router.ServeHTTP(triggerRec, triggerReq)
	if triggerRec.Code != http.StatusAccepted {
		t.Fatalf("trigger status = %d, want %d; body=%s", triggerRec.Code, http.StatusAccepted, triggerRec.Body.String())
	}

	var triggered struct {
		Report  report.DailyReport `json:"report"`
		TaskRun adminapi.TaskRun   `json:"taskRun"`
	}
	if err := json.Unmarshal(triggerRec.Body.Bytes(), &triggered); err != nil {
		t.Fatalf("decode trigger response: %v", err)
	}
	if triggered.Report.Scope != report.ScopePlatform {
		t.Fatalf("report scope = %q, want %q", triggered.Report.Scope, report.ScopePlatform)
	}
	if triggered.TaskRun.TaskName != "daily-report" {
		t.Fatalf("task name = %q, want daily-report", triggered.TaskRun.TaskName)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/task-runs", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}

	var listBody struct {
		TaskRuns []adminapi.TaskRun `json:"taskRuns"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listBody.TaskRuns) == 0 {
		t.Fatalf("task runs empty")
	}
}

func TestAdminAPITaskRunsCanFilterFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/task-runs?status=failed", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		TaskRuns []adminapi.TaskRun `json:"taskRuns"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode task runs response: %v", err)
	}
	for _, run := range body.TaskRuns {
		if run.Status != adminapi.TaskStatusFailed {
			t.Fatalf("non-failed task in filtered result: %#v", run)
		}
	}
}
