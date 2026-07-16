package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	monitorapplication "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// TestMonitorRoutesRequireTheDesignRoles protects the public contract before
// the concrete transport is implemented. The service is deliberately nil: a
// rejected request must not reach application code.
func TestMonitorRoutesRequireTheDesignRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, (*monitorapplication.Service)(nil), testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleViewer}})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/monitors", nil)
	request.Header.Set("Authorization", "Bearer viewer")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestExpectedDraftVersionRequiresExplicitJSONNullOrPositiveInteger(t *testing.T) {
	t.Parallel()
	cases := []struct {
		body    string
		wantNil bool
		wantErr bool
	}{
		{`{"expected_monitor_version":3,"expected_draft_version":null}`, true, false},
		{`{"expected_monitor_version":3,"expected_draft_version":9}`, false, false},
		{`{"expected_monitor_version":3}`, false, true},
		{`{"expected_monitor_version":3,"expected_draft_version":0}`, false, true},
	}
	for _, test := range cases {
		var request ExpectedDraftRequest
		if err := json.Unmarshal([]byte(test.body), &request); err != nil {
			t.Fatalf("decode %s: %v", test.body, err)
		}
		expected, err := expectedVersions(request)
		if (err != nil) != test.wantErr {
			t.Fatalf("expectedVersions(%s) error = %v, wantErr %v", test.body, err, test.wantErr)
		}
		if !test.wantErr && (expected.DraftVersion == nil) != test.wantNil {
			t.Fatalf("expected draft nil = %v, want %v", expected.DraftVersion == nil, test.wantNil)
		}
	}
}

func TestEmbeddedExpectedDraftVersionRetainsExplicitNull(t *testing.T) {
	t.Parallel()
	var request ReplaceDraftRequest
	if err := json.Unmarshal([]byte(`{"expected_monitor_version":3,"expected_draft_version":null}`), &request); err != nil {
		t.Fatalf("decode replacement request: %v", err)
	}
	expected, err := expectedVersions(request.ExpectedDraftRequest)
	if err != nil || expected.DraftVersion != nil {
		t.Fatalf("embedded expected draft = %#v, %v; want explicit null", expected, err)
	}
}

func TestMonitorResponseNeverContainsSourceExecutionFields(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(monitorResponse(domain.Monitor{ID: 1, Version: 1, Name: "monitor", Status: domain.MonitorStatusDraft}, nil, nil, nil, false))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	for _, forbidden := range []string{"endpoint", "credential_ref", "config", "health_diagnostic"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("monitor response exposes %q: %s", forbidden, encoded)
		}
	}
}

type testAuthenticator struct{ subject httptransport.Subject }

func (auth testAuthenticator) Authenticate(_ context.Context, _ string) (httptransport.Subject, error) {
	return auth.subject, nil
}
