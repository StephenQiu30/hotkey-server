package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type governanceServiceFake struct {
	retentionInput operationsapplication.RetentionInput
	retentionRun   operationsapplication.RetentionRunInput
	auditSubject   identitydomain.Subject
	auditQuery     operationsdomain.AuditQuery
	auditPage      operationsdomain.AuditPage
}

func (*governanceServiceFake) Usage(context.Context, identitydomain.Subject) (operationsdomain.UsageOverview, error) {
	return operationsdomain.UsageOverview{Items: []operationsdomain.UsageItem{{Dimension: operationsdomain.DimensionManualSearches, Mode: "observed", Used: "2"}}}, nil
}
func (*governanceServiceFake) RetentionPolicies(context.Context, identitydomain.Subject) ([]operationsdomain.RetentionPolicy, error) {
	return []operationsdomain.RetentionPolicy{{ID: 1, Version: 2, DataClass: "sessions", RetentionDays: 30, Action: "delete", Enabled: true}}, nil
}
func (service *governanceServiceFake) PreviewRetention(_ context.Context, input operationsapplication.RetentionInput) (operationsdomain.CleanupResult, error) {
	if input.BatchSize < 1 || input.BatchSize > 1000 {
		return operationsdomain.CleanupResult{}, sharedrepository.ErrInvalidInput
	}
	service.retentionInput = input
	return operationsdomain.CleanupResult{RunID: 11, PolicyVersion: 2, DataClass: "sessions", Affected: 4, BatchSize: input.BatchSize, CandidateHash: strings.Repeat("a", 64), Status: "pending_approval", DryRun: true}, nil
}
func (service *governanceServiceFake) ApproveRetention(_ context.Context, input operationsapplication.RetentionRunInput) (operationsdomain.CleanupResult, error) {
	service.retentionRun = input
	return operationsdomain.CleanupResult{RunID: input.RunID, DataClass: "sessions", Affected: 4, CandidateHash: input.CandidateHash, Status: "approved", DryRun: true}, nil
}
func (service *governanceServiceFake) ExecuteRetention(_ context.Context, input operationsapplication.RetentionRunInput) (operationsdomain.CleanupResult, error) {
	service.retentionRun = input
	return operationsdomain.CleanupResult{RunID: input.RunID, DataClass: "sessions", Affected: 4, CandidateHash: input.CandidateHash, Status: "completed"}, nil
}
func (service *governanceServiceFake) Audit(_ context.Context, subject identitydomain.Subject, query operationsdomain.AuditQuery) (operationsdomain.AuditPage, error) {
	service.auditSubject, service.auditQuery = subject, query
	page := service.auditPage
	if page.Items == nil {
		page.Items = []operationsdomain.AuditRecord{{ID: 9, Action: "retention.executed", ResourceType: "retention_policy", Result: "success"}}
	}
	return page, nil
}

type governanceAuthenticator struct{ role httptransport.Role }

func (auth governanceAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 7, SessionID: 8, Role: auth.role}, nil
}

func TestGovernanceHandlersExposeSafeContractsAndBoundedInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &governanceServiceFake{}
	router := governanceRouter(service, httptransport.RoleAdmin)
	for _, request := range []struct{ method, path, body, contains string }{
		{http.MethodGet, "/usage", "", `"manual_searches"`},
		{http.MethodGet, "/retention", "", `"data_class":"sessions"`},
		{http.MethodGet, "/audit", "", `"retention.executed"`},
		{http.MethodPost, "/preview/1", `{"expected_version":2,"batch_size":100}`, `"status":"pending_approval"`},
		{http.MethodPost, "/approve/11", `{"candidate_hash":"` + strings.Repeat("a", 64) + `"}`, `"status":"approved"`},
		{http.MethodPost, "/execute/11", `{"candidate_hash":"` + strings.Repeat("a", 64) + `"}`, `"status":"completed"`},
	} {
		recorder := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(request.method, request.path, strings.NewReader(request.body))
		httpRequest.Header.Set("Authorization", "Bearer admin")
		if request.body != "" {
			httpRequest.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(recorder, httpRequest)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), request.contains) {
			t.Fatalf("%s %s = %d %s", request.method, request.path, recorder.Code, recorder.Body.String())
		}
		for _, forbidden := range []string{"before_data", "after_data", "ip_hash", "trace_id", "args", "credential"} {
			if strings.Contains(recorder.Body.String(), forbidden) {
				t.Fatalf("response leaked %q: %s", forbidden, recorder.Body.String())
			}
		}
	}
	if service.retentionInput.PolicyID != 1 || service.retentionInput.ExpectedVersion != 2 || service.retentionInput.BatchSize != 100 || service.retentionInput.Subject.UserID != 7 {
		t.Fatalf("retention input = %#v", service.retentionInput)
	}
	if service.retentionRun.RunID != 11 || service.retentionRun.CandidateHash != strings.Repeat("a", 64) || service.retentionRun.Subject.UserID != 7 {
		t.Fatalf("retention run input = %#v", service.retentionRun)
	}
}

func TestGovernanceRoutesEnforceRolesAndRejectInvalidBatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		role httptransport.Role
		path string
		want int
	}{
		{httptransport.RoleViewer, "/usage", http.StatusForbidden},
		{httptransport.RoleEditor, "/usage", http.StatusOK},
		{httptransport.RoleEditor, "/retention", http.StatusForbidden},
		{httptransport.RoleViewer, "/audit", http.StatusForbidden},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set("Authorization", "Bearer member")
		governanceRouter(&governanceServiceFake{}, test.role).ServeHTTP(recorder, request)
		if recorder.Code != test.want {
			t.Fatalf("%s %s = %d, want %d", test.role, test.path, recorder.Code, test.want)
		}
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/preview/1", strings.NewReader(`{"expected_version":2,"batch_size":1001}`))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	governanceRouter(&governanceServiceFake{}, httptransport.RoleAdmin).ServeHTTP(recorder, request)
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || result.Code != 10000 {
		t.Fatalf("invalid batch = %d/%d", recorder.Code, result.Code)
	}
}

func TestGovernanceAuditForwardsOpaqueSubjectBoundCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &governanceServiceFake{auditPage: operationsdomain.AuditPage{NextCursor: "signed.next.cursor"}}
	request := httptest.NewRequest(http.MethodGet, "/audit?cursor=signed.input.cursor&limit=2&action=monitor.published&resource_type=monitor&result=success", nil)
	request.Header.Set("Authorization", "Bearer admin")
	recorder := httptest.NewRecorder()
	governanceRouter(service, httptransport.RoleAdmin).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"next_cursor":"signed.next.cursor"`) {
		t.Fatalf("audit response = %d %s", recorder.Code, recorder.Body.String())
	}
	if service.auditSubject.UserID != 7 || service.auditQuery.SubjectUserID != 7 || service.auditQuery.Cursor != "signed.input.cursor" || service.auditQuery.Limit != 2 || service.auditQuery.Action != "monitor.published" || service.auditQuery.ResourceType != "monitor" || service.auditQuery.Result != "success" {
		t.Fatalf("audit subject/query = %#v/%#v", service.auditSubject, service.auditQuery)
	}
}

func governanceRouter(service governanceService, role httptransport.Role) *gin.Engine {
	handler := NewGovernanceHandler(service)
	router := gin.New()
	auth := httptransport.RequireAuthentication(governanceAuthenticator{role: role})
	router.GET("/usage", auth, httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin), httptransport.Wrap(handler.Usage))
	router.GET("/retention", auth, httptransport.RequireRoles(httptransport.RoleAdmin), httptransport.Wrap(handler.RetentionPolicies))
	router.GET("/audit", auth, httptransport.RequireRoles(httptransport.RoleAdmin), httptransport.Wrap(handler.Audit))
	router.POST("/preview/:id", auth, httptransport.RequireRoles(httptransport.RoleAdmin), httptransport.Wrap(handler.PreviewRetention))
	router.POST("/approve/:id", auth, httptransport.RequireRoles(httptransport.RoleAdmin), httptransport.Wrap(handler.ApproveRetention))
	router.POST("/execute/:id", auth, httptransport.RequireRoles(httptransport.RoleAdmin), httptransport.Wrap(handler.ExecuteRetention))
	return router
}
