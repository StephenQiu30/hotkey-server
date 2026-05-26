package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/workqueue"
	"github.com/gin-gonic/gin"
)

func TestWorkQueueEndpointsEnqueueRunAndListJobs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	enqueueReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/work-queue/jobs", bytes.NewBufferString(`{"type":"collect","priority":90,"payload":{"source":"arxiv-ai"}}`))
	enqueueReq.Header.Set("Content-Type", "application/json")
	enqueueRec := httptest.NewRecorder()
	router.ServeHTTP(enqueueRec, enqueueReq)
	if enqueueRec.Code != http.StatusCreated {
		t.Fatalf("enqueue status = %d, want %d; body=%s", enqueueRec.Code, http.StatusCreated, enqueueRec.Body.String())
	}

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/work-queue/run", bytes.NewBufferString(`{"workers":2,"maxJobs":1}`))
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	router.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("run status = %d, want %d; body=%s", runRec.Code, http.StatusOK, runRec.Body.String())
	}
	var runResult workqueue.WorkerPoolResult
	if err := json.Unmarshal(runRec.Body.Bytes(), &runResult); err != nil {
		t.Fatalf("decode run result: %v", err)
	}
	if runResult.Processed != 1 {
		t.Fatalf("processed = %d, want 1", runResult.Processed)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/work-queue/jobs", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
}
