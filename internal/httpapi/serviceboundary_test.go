package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/serviceboundary"
	"github.com/gin-gonic/gin"
)

func TestServiceBoundaryEndpointExposesTopologyAndMessageContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-boundaries", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		Topology serviceboundary.Topology            `json:"topology"`
		Contract serviceboundary.TaskMessageContract `json:"taskMessageContract"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body.Topology.Services[serviceboundary.ServiceAPI].OpenAPISource {
		t.Fatalf("api should be openapi source: %#v", body.Topology)
	}
	if !body.Contract.RequiredFields["tenantId"] {
		t.Fatalf("contract missing tenantId: %#v", body.Contract.RequiredFields)
	}
}
