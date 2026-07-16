package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
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

func TestReplaceDraftRouteRequiresExplicitNullableDraftVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &draftVersionMonitorService{readMonitorService: readMonitorService{view: monitorapplication.MonitorView{Monitor: domain.Monitor{ID: 1, Version: 4, Name: "monitor", Status: domain.MonitorStatusDraft}}}}
	router := gin.New()
	RegisterRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 2, SessionID: 2, Role: httptransport.RoleEditor}})

	validDraft := `"name":"monitor","config":{"timezone":"UTC","languages":["en"],"collection_interval_seconds":300,"relevance_threshold":60,"event_threshold":1,"retention_days":30},"rules":[{"rule_type":"keyword","operator":"contains","value":"OpenAI"}],"sources":[{"source_connection_id":7}]`
	for _, test := range []struct {
		name       string
		version    string
		wantStatus int
		wantCalls  int
		wantNil    bool
	}{
		{name: "omitted", version: `"expected_monitor_version":4`, wantStatus: http.StatusBadRequest},
		{name: "zero", version: `"expected_monitor_version":4,"expected_draft_version":0`, wantStatus: http.StatusBadRequest},
		{name: "explicit null", version: `"expected_monitor_version":4,"expected_draft_version":null`, wantStatus: http.StatusOK, wantCalls: 1, wantNil: true},
		{name: "positive", version: `"expected_monitor_version":4,"expected_draft_version":3`, wantStatus: http.StatusOK, wantCalls: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, "/api/v1/monitors/1/draft", strings.NewReader(`{`+test.version+`,`+validDraft+`}`))
			request.Header.Set("Authorization", "Bearer editor")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if service.calls != test.wantCalls {
				t.Fatalf("ReplaceDraft calls = %d, want %d", service.calls, test.wantCalls)
			}
			if test.wantStatus == http.StatusOK && (service.lastDraft == nil) != test.wantNil {
				t.Fatalf("last draft version = %#v, want nil=%v", service.lastDraft, test.wantNil)
			}
		})
	}
}

func TestMonitorDraftRequestsAcceptZeroThresholdAndDefaultOmittedPriorities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &captureMonitorService{readMonitorService: readMonitorService{view: monitorapplication.MonitorView{Monitor: domain.Monitor{ID: 1, Version: 4, Name: "monitor", Status: domain.MonitorStatusDraft}}}}
	router := gin.New()
	RegisterRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 2, SessionID: 2, Role: httptransport.RoleEditor}})

	for _, test := range []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name: "create explicit zero", method: http.MethodPost, path: "/api/v1/monitors",
			body:       `{"name":"zero create","config":{"timezone":"UTC","languages":["en"],"collection_interval_seconds":300,"relevance_threshold":60,"event_threshold":0,"retention_days":30},"rules":[{"rule_type":"keyword","operator":"contains","value":"OpenAI"}],"sources":[{"source_connection_id":7}]}`,
			wantStatus: http.StatusCreated,
		},
		{
			name: "replace explicit zero", method: http.MethodPut, path: "/api/v1/monitors/1/draft",
			body:       `{"expected_monitor_version":4,"expected_draft_version":3,"name":"zero replace","config":{"timezone":"UTC","languages":["en"],"collection_interval_seconds":300,"relevance_threshold":60,"event_threshold":0,"retention_days":30},"rules":[{"rule_type":"keyword","operator":"contains","value":"OpenAI"}],"sources":[{"source_connection_id":7}]}`,
			wantStatus: http.StatusOK,
		},
		{
			name: "missing threshold", method: http.MethodPost, path: "/api/v1/monitors",
			body:       `{"name":"missing threshold","config":{"timezone":"UTC","languages":["en"],"collection_interval_seconds":300,"relevance_threshold":60,"retention_days":30},"rules":[{"rule_type":"keyword","operator":"contains","value":"OpenAI"}],"sources":[{"source_connection_id":7}]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "replace missing threshold", method: http.MethodPut, path: "/api/v1/monitors/1/draft",
			body:       `{"expected_monitor_version":4,"expected_draft_version":3,"name":"missing threshold","config":{"timezone":"UTC","languages":["en"],"collection_interval_seconds":300,"relevance_threshold":60,"retention_days":30},"rules":[{"rule_type":"keyword","operator":"contains","value":"OpenAI"}],"sources":[{"source_connection_id":7}]}`,
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer editor")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if test.wantStatus == http.StatusBadRequest {
				var result struct {
					Code int             `json:"code"`
					Data json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatalf("decode Result error: %v", err)
				}
				if result.Code == 0 || string(result.Data) != "null" {
					t.Fatalf("invalid request Result = %s, want nonzero code and null data", response.Body.String())
				}
			}
		})
	}

	if len(service.creates) != 1 || len(service.replacements) != 1 {
		t.Fatalf("create/replace calls = %d/%d, want 1/1", len(service.creates), len(service.replacements))
	}
	for _, draft := range []monitorapplication.DraftInput{service.creates[0].Draft, service.replacements[0].Draft} {
		if draft.Config.EventThreshold != 0 {
			t.Fatalf("event threshold = %v, want explicit zero", draft.Config.EventThreshold)
		}
		if draft.Rules[0].Priority != 100 || draft.Sources[0].Priority != 100 {
			t.Fatalf("omitted priorities = rule %d source %d, want 100", draft.Rules[0].Priority, draft.Sources[0].Priority)
		}
	}
}

func TestMonitorDraftRequestPriorityPresenceKeepsExplicitZeroAndHashesLikeExplicitDefault(t *testing.T) {
	zeroThreshold := float64(0)
	config := MonitorConfigRequest{Timezone: "UTC", Languages: []string{"en"}, CollectionIntervalSeconds: 300, RelevanceThreshold: 60, EventThreshold: &zeroThreshold, RetentionDays: 30}
	omittedRules, omittedSources := monitorRules([]MonitorRuleRequest{{RuleType: "keyword", Operator: "contains", Value: "OpenAI"}}), monitorSources([]MonitorSourceRequest{{SourceConnectionID: 7}})
	explicitPriority := int16(100)
	explicitRules, explicitSources := monitorRules([]MonitorRuleRequest{{RuleType: "keyword", Operator: "contains", Value: "OpenAI", Priority: &explicitPriority}}), monitorSources([]MonitorSourceRequest{{SourceConnectionID: 7, Priority: &explicitPriority}})
	zero := int16(0)
	zeroRules, zeroSources := monitorRules([]MonitorRuleRequest{{RuleType: "keyword", Operator: "contains", Value: "OpenAI", Priority: &zero}}), monitorSources([]MonitorSourceRequest{{SourceConnectionID: 7, Priority: &zero}})
	if zeroRules[0].Priority != 0 || zeroSources[0].Priority != 0 {
		t.Fatalf("explicit zero priority was not preserved: rules=%d sources=%d", zeroRules[0].Priority, zeroSources[0].Priority)
	}
	omittedHash, err := domain.CanonicalConfigHash(domain.ConfigHashInput{MonitorID: 1, Revision: 1, Config: monitorConfig(config), Rules: omittedRules, Sources: omittedSources})
	if err != nil {
		t.Fatalf("CanonicalConfigHash(omitted) error: %v", err)
	}
	explicitHash, err := domain.CanonicalConfigHash(domain.ConfigHashInput{MonitorID: 1, Revision: 1, Config: monitorConfig(config), Rules: explicitRules, Sources: explicitSources})
	if err != nil {
		t.Fatalf("CanonicalConfigHash(explicit) error: %v", err)
	}
	if omittedHash != explicitHash {
		t.Fatalf("omitted/default config hash = %s, explicit 100 hash = %s", omittedHash, explicitHash)
	}
}

func TestMonitorResponseNeverContainsSourceExecutionFields(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(monitorResponse(monitorapplication.MonitorView{Monitor: domain.Monitor{ID: 1, Version: 1, Name: "monitor", Status: domain.MonitorStatusDraft}}))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	for _, forbidden := range []string{"endpoint", "credential_ref", "config", "health_diagnostic"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("monitor response exposes %q: %s", forbidden, encoded)
		}
	}
}

func TestMonitorReadRoutesProjectPublishedAndDraftByRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	published := monitorapplication.ConfigurationView{Config: domain.MonitorConfigVersion{ID: 10, Revision: 2, Config: domain.MonitorConfig{Timezone: "UTC", Languages: []string{"en"}, CollectionIntervalSeconds: 300, RelevanceThreshold: 60, EventThreshold: 0, RetentionDays: 30}}, Sources: []monitorapplication.MonitorSourceView{{MonitorSource: domain.MonitorSource{ID: 4, SourceConnectionID: 7, Enabled: true}, SourceName: "RSS", SourceType: "rss"}}}
	draft := monitorapplication.ConfigurationView{Config: domain.MonitorConfigVersion{ID: 11, Revision: 3, Config: published.Config.Config}}
	view := monitorapplication.MonitorView{Monitor: domain.Monitor{ID: 1, Version: 4, Name: "monitor", Status: domain.MonitorStatusPaused}, Published: &published, Draft: &draft}
	service := &readMonitorService{view: view}

	viewerRouter := gin.New()
	RegisterRoutes(viewerRouter, service, testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleViewer}})
	viewerRequest := httptest.NewRequest(http.MethodGet, "/api/v1/monitors", nil)
	viewerRequest.Header.Set("Authorization", "Bearer viewer")
	viewerResponse := httptest.NewRecorder()
	viewerRouter.ServeHTTP(viewerResponse, viewerRequest)
	if viewerResponse.Code != http.StatusOK {
		t.Fatalf("viewer list status = %d: %s", viewerResponse.Code, viewerResponse.Body.String())
	}
	if strings.Contains(viewerResponse.Body.String(), `"draft"`) || !strings.Contains(viewerResponse.Body.String(), `"source_type":"rss"`) || !strings.Contains(viewerResponse.Body.String(), `"name":"RSS"`) {
		t.Fatalf("viewer response did not preserve safe published-only source projection: %s", viewerResponse.Body.String())
	}

	editorRouter := gin.New()
	RegisterRoutes(editorRouter, service, testAuthenticator{subject: httptransport.Subject{UserID: 2, SessionID: 2, Role: httptransport.RoleEditor}})
	editorRequest := httptest.NewRequest(http.MethodGet, "/api/v1/monitors/1", nil)
	editorRequest.Header.Set("Authorization", "Bearer editor")
	editorResponse := httptest.NewRecorder()
	editorRouter.ServeHTTP(editorResponse, editorRequest)
	if editorResponse.Code != http.StatusOK || !strings.Contains(editorResponse.Body.String(), `"draft"`) {
		t.Fatalf("editor response = %d %s, want safe draft metadata", editorResponse.Code, editorResponse.Body.String())
	}
	for _, forbidden := range []string{"endpoint", "credential_ref", "health_diagnostic"} {
		if strings.Contains(editorResponse.Body.String(), forbidden) {
			t.Fatalf("editor monitor response leaked %q: %s", forbidden, editorResponse.Body.String())
		}
	}
}

type readMonitorService struct {
	view monitorapplication.MonitorView
}

type draftVersionMonitorService struct {
	readMonitorService
	calls     int
	lastDraft *int64
}

type captureMonitorService struct {
	readMonitorService
	creates      []monitorapplication.CreateInput
	replacements []monitorapplication.ReplaceDraftInput
}

func (service *captureMonitorService) Create(_ context.Context, input monitorapplication.CreateInput) (*domain.Monitor, *domain.MonitorConfigVersion, error) {
	service.creates = append(service.creates, input)
	return &domain.Monitor{ID: 1}, &domain.MonitorConfigVersion{}, nil
}

func (service *captureMonitorService) ReplaceDraft(_ context.Context, input monitorapplication.ReplaceDraftInput) (*domain.Monitor, *domain.MonitorConfigVersion, error) {
	service.replacements = append(service.replacements, input)
	return &domain.Monitor{ID: input.MonitorID}, &domain.MonitorConfigVersion{}, nil
}

func (service *draftVersionMonitorService) ReplaceDraft(_ context.Context, input monitorapplication.ReplaceDraftInput) (*domain.Monitor, *domain.MonitorConfigVersion, error) {
	service.calls++
	service.lastDraft = input.Expected.DraftVersion
	return &domain.Monitor{ID: input.MonitorID}, nil, nil
}

func (service *readMonitorService) List(_ context.Context, input monitorapplication.ListInput) (monitorapplication.MonitorPage, error) {
	view := service.view
	if input.Subject.Role == identitydomain.RoleViewer {
		view.Draft = nil
	}
	return monitorapplication.MonitorPage{Items: []monitorapplication.MonitorView{view}}, nil
}
func (service *readMonitorService) Get(_ context.Context, input identitydomain.Subject, _ int64) (monitorapplication.MonitorView, error) {
	view := service.view
	if input.Role == identitydomain.RoleViewer {
		view.Draft = nil
	}
	return view, nil
}
func (*readMonitorService) Create(context.Context, monitorapplication.CreateInput) (*domain.Monitor, *domain.MonitorConfigVersion, error) {
	return nil, nil, nil
}
func (*readMonitorService) ReplaceDraft(context.Context, monitorapplication.ReplaceDraftInput) (*domain.Monitor, *domain.MonitorConfigVersion, error) {
	return nil, nil, nil
}
func (*readMonitorService) AddAICandidate(context.Context, monitorapplication.AICandidateInput) (*domain.MonitorConfigVersion, *domain.MonitorRule, error) {
	return nil, nil, nil
}
func (*readMonitorService) ApproveAICandidate(context.Context, monitorapplication.ApprovalInput) (*domain.MonitorConfigVersion, error) {
	return nil, nil
}
func (*readMonitorService) Preview(context.Context, identitydomain.Subject, int64) (monitorapplication.PreviewResult, error) {
	return monitorapplication.PreviewResult{}, nil
}
func (*readMonitorService) Publish(context.Context, monitorapplication.PublishInput) (*domain.Monitor, *domain.MonitorConfigVersion, error) {
	return nil, nil, nil
}
func (*readMonitorService) Pause(context.Context, monitorapplication.LifecycleInput) (*domain.Monitor, error) {
	return nil, nil
}
func (*readMonitorService) Resume(context.Context, monitorapplication.LifecycleInput) (*domain.Monitor, error) {
	return nil, nil
}
func (*readMonitorService) Archive(context.Context, monitorapplication.LifecycleInput) (*domain.Monitor, error) {
	return nil, nil
}
func (*readMonitorService) Restore(context.Context, monitorapplication.LifecycleInput) (*domain.Monitor, error) {
	return nil, nil
}

type testAuthenticator struct{ subject httptransport.Subject }

func (auth testAuthenticator) Authenticate(_ context.Context, _ string) (httptransport.Subject, error) {
	return auth.subject, nil
}
