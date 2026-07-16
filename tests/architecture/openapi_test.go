package architecture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type openAPIOperation struct {
	Security  []map[string][]string `json:"security"`
	Responses map[string]struct {
		Schema struct {
			Ref string `json:"$ref"`
		} `json:"schema"`
	} `json:"responses"`
}

func TestOpenAPIContract(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "openapi", "swagger.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var document struct {
		Swagger             string                     `json:"swagger"`
		SecurityDefinitions map[string]json.RawMessage `json:"securityDefinitions"`
		Paths               map[string]json.RawMessage `json:"paths"`
		Definitions         map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	if document.Swagger != "2.0" {
		t.Errorf("swagger version = %q, want 2.0", document.Swagger)
	}
	if _, ok := document.SecurityDefinitions["BearerAuth"]; !ok {
		t.Fatal("missing BearerAuth security definition")
	}

	required := map[string]map[string][]string{
		"/api/v1/capabilities":                                 {"get": {"200"}},
		"/api/v1/auth/email-verifications":                     {"post": {"200", "400", "429", "503"}},
		"/api/v1/auth/email-verifications/confirm":             {"post": {"200", "400", "429", "503"}},
		"/api/v1/auth/registrations":                           {"post": {"201", "400", "409", "503"}},
		"/api/v1/auth/login":                                   {"post": {"200", "400", "401", "503"}},
		"/api/v1/auth/refresh":                                 {"post": {"200", "401", "403", "503"}},
		"/api/v1/auth/logout":                                  {"post": {"200", "403", "503"}},
		"/api/v1/auth/me":                                      {"get": {"200", "401"}},
		"/api/v1/auth/password":                                {"post": {"200", "400", "401", "503"}},
		"/api/v1/auth/password-resets/confirm":                 {"post": {"200", "400", "503"}},
		"/api/v1/users":                                        {"get": {"200", "401", "403", "503"}},
		"/api/v1/users/{id}":                                   {"patch": {"200", "400", "401", "403", "409", "503"}, "delete": {"200", "401", "403", "409", "503"}},
		"/api/v1/users/{id}/restore":                           {"post": {"200", "401", "403", "409", "503"}},
		"/api/v1/monitors":                                     {"get": {"200", "400", "401", "503"}, "post": {"201", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}":                                {"get": {"200", "400", "401", "409", "503"}},
		"/api/v1/monitors/{id}/draft":                          {"put": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/draft/ai-candidates":            {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/draft/rules/{rule_id}/approval": {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/preview":                        {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/publish":                        {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/pause":                          {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/resume":                         {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/archive":                        {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/restore":                        {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-connections":                           {"get": {"200", "400", "401", "503"}, "post": {"201", "400", "401", "403", "409", "503"}},
		"/api/v1/source-connections/{id}":                      {"get": {"200", "400", "401", "409", "503"}, "patch": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-connections/{id}/enable":               {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-connections/{id}/disable":              {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-connections/{id}/archive":              {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-connections/{id}/restore":              {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/collection-runs":                              {"get": {"200", "400", "401", "403", "503"}},
		"/api/v1/collection-runs/{id}/retry":                   {"post": {"200", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/source-connections/{id}/health":               {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/contents":                                     {"get": {"200", "400", "401", "404", "503"}},
		"/api/v1/contents/{id}":                                {"get": {"200", "400", "401", "404", "503"}},
		"/api/v1/ai/model-profiles":                            {"get": {"200", "401", "403", "503"}, "post": {"201", "400", "401", "403", "503"}},
		"/api/v1/ai/model-profiles/{id}":                       {"get": {"200", "400", "401", "403", "503"}, "patch": {"200", "400", "401", "403", "409", "503"}, "delete": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/ai/model-profiles/{id}/restore":               {"post": {"200", "400", "401", "403", "409", "503"}},
	}
	if len(document.Paths) != len(required) {
		t.Fatalf("public path count = %d, want %d (%v)", len(document.Paths), len(required), document.Paths)
	}
	for route, methods := range required {
		rawPath, ok := document.Paths[route]
		if !ok {
			t.Errorf("missing %s", route)
			continue
		}
		var operations map[string]openAPIOperation
		if err := json.Unmarshal(rawPath, &operations); err != nil {
			t.Errorf("decode %s: %v", route, err)
			continue
		}
		for method, statuses := range methods {
			operation, ok := operations[method]
			if !ok {
				t.Errorf("missing %s %s", strings.ToUpper(method), route)
				continue
			}
			for _, status := range statuses {
				response, ok := operation.Responses[status]
				if !ok || response.Schema.Ref == "" {
					t.Errorf("%s %s response %s lacks a concrete Result schema", strings.ToUpper(method), route, status)
					continue
				}
				result := document.Definitions[strings.TrimPrefix(response.Schema.Ref, "#/definitions/")]
				for _, field := range []string{"code", "message", "data"} {
					if _, ok := result.Properties[field]; !ok {
						t.Errorf("%s %s response %s result misses %q", strings.ToUpper(method), route, status, field)
					}
				}
			}
		}
	}

	for _, route := range []string{"/api/v1/auth/me", "/api/v1/auth/password", "/api/v1/users", "/api/v1/users/{id}", "/api/v1/users/{id}/restore", "/api/v1/monitors", "/api/v1/monitors/{id}", "/api/v1/monitors/{id}/draft", "/api/v1/monitors/{id}/draft/ai-candidates", "/api/v1/monitors/{id}/draft/rules/{rule_id}/approval", "/api/v1/monitors/{id}/preview", "/api/v1/monitors/{id}/publish", "/api/v1/monitors/{id}/pause", "/api/v1/monitors/{id}/resume", "/api/v1/monitors/{id}/archive", "/api/v1/monitors/{id}/restore", "/api/v1/source-connections", "/api/v1/source-connections/{id}", "/api/v1/source-connections/{id}/enable", "/api/v1/source-connections/{id}/disable", "/api/v1/source-connections/{id}/archive", "/api/v1/source-connections/{id}/restore", "/api/v1/collection-runs", "/api/v1/collection-runs/{id}/retry", "/api/v1/source-connections/{id}/health", "/api/v1/contents", "/api/v1/contents/{id}", "/api/v1/ai/model-profiles", "/api/v1/ai/model-profiles/{id}", "/api/v1/ai/model-profiles/{id}/restore"} {
		var operations map[string]openAPIOperation
		if err := json.Unmarshal(document.Paths[route], &operations); err != nil {
			t.Fatalf("decode protected path %s: %v", route, err)
		}
		for method, operation := range operations {
			if !usesBearerAuth(operation.Security) {
				t.Errorf("%s %s is missing BearerAuth", strings.ToUpper(method), route)
			}
		}
	}

	for _, route := range []string{"/healthz", "/readyz", "/metrics"} {
		if _, exists := document.Paths[route]; exists {
			t.Errorf("operational path %s must not be in OpenAPI", route)
		}
	}
	assertSafeIdentityOpenAPIDefinitions(t, document.Definitions)
	assertSafeMonitorSourceOpenAPIDefinitions(t, document.Definitions)
	assertSafeCollectionOpenAPIDefinitions(t, document.Definitions)
	assertSafeContentOpenAPIDefinitions(t, document.Definitions)
	assertSafeModelProfileOpenAPIDefinitions(t, document.Definitions)
	assertDraftExpectedVersionOpenAPI(t, document.Definitions)
	assertMonitorDraftDefaultsOpenAPI(t, document.Definitions)
}

func assertSafeContentOpenAPIDefinitions(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	content, ok := definitions["http.ContentResponse"]
	if !ok {
		t.Fatal("missing http.ContentResponse")
	}
	allowed := map[string]bool{
		"id": true, "source_type": true, "source_name": true, "external_id": true,
		"content_type": true, "title": true, "canonical_url": true, "language": true,
		"published_at": true, "fetched_at": true, "metrics": true, "dedupe_status": true,
		"dedupe_reason": true, "dedupe_version": true,
	}
	for field := range content.Properties {
		if !allowed[field] {
			t.Errorf("safe Content response exposes %q", field)
		}
	}
	for field := range allowed {
		if _, exists := content.Properties[field]; !exists {
			t.Errorf("safe Content response misses %q", field)
		}
	}
	metrics, ok := definitions["http.ContentMetricsResponse"]
	if !ok {
		t.Fatal("missing http.ContentMetricsResponse")
	}
	for field := range metrics.Properties {
		if field != "view_count" && field != "like_count" && field != "comment_count" && field != "share_count" {
			t.Errorf("safe Content metrics exposes %q", field)
		}
	}
	for _, forbidden := range []string{"excerpt", "body", "object_key", "asset", "minio", "endpoint", "credential", "stack", "error"} {
		if _, exists := content.Properties[forbidden]; exists {
			t.Errorf("safe Content response exposes forbidden %q", forbidden)
		}
	}
}

func assertSafeModelProfileOpenAPIDefinitions(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	response, ok := definitions["http.ModelProfileResponse"]
	if !ok {
		t.Fatal("missing http.ModelProfileResponse")
	}
	allowedResponse := map[string]bool{
		"id": true, "version": true, "name": true, "task_type": true, "provider": true,
		"model_name": true, "model_version": true, "embedding_dimensions": true,
		"timeout_seconds": true, "max_attempts": true, "max_cost": true, "daily_budget": true,
		"fallback_priority": true, "enabled": true, "deleted": true, "created_at": true, "updated_at": true,
	}
	for field := range response.Properties {
		if !allowedResponse[field] {
			t.Errorf("safe model profile response exposes %q", field)
		}
	}
	for field := range allowedResponse {
		if _, exists := response.Properties[field]; !exists {
			t.Errorf("safe model profile response misses %q", field)
		}
	}

	update, ok := definitions["http.UpdateModelProfileRequest"]
	if !ok {
		t.Fatal("missing http.UpdateModelProfileRequest")
	}
	for _, forbidden := range []string{"task_type", "provider", "model_name", "model_version", "credential_ref", "embedding_dimensions", "endpoint", "parameters", "prompt", "raw_response", "api_key"} {
		if _, exists := update.Properties[forbidden]; exists {
			t.Errorf("model profile PATCH schema exposes immutable or sensitive %q", forbidden)
		}
	}
	for _, required := range []string{"version", "timeout_seconds", "max_attempts", "max_cost", "daily_budget", "fallback_priority", "enabled"} {
		if _, exists := update.Properties[required]; !exists {
			t.Errorf("model profile PATCH schema misses %q", required)
		}
	}

	create, ok := definitions["http.CreateModelProfileRequest"]
	if !ok {
		t.Fatal("missing http.CreateModelProfileRequest")
	}
	credential, exists := create.Properties["credential_ref"]
	if !exists {
		t.Error("model profile create schema must accept write-only credential_ref")
	} else if strings.Contains(string(credential), "example") {
		t.Error("model profile credential_ref must not have an OpenAPI example")
	}
	for _, forbidden := range []string{"endpoint", "parameters", "prompt", "raw_response", "api_key"} {
		if _, exists := create.Properties[forbidden]; exists {
			t.Errorf("model profile create schema exposes %q", forbidden)
		}
	}
}

func assertSafeCollectionOpenAPIDefinitions(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	for _, name := range []string{"http.CollectionRunResponse", "http.CollectionRunTargetResponse", "http.SourceHealthResponse"} {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing safe collection response definition %s", name)
			continue
		}
		for _, field := range []string{"source_connection_id", "query_signature", "query", "request_cursor", "next_cursor", "etag", "last_modified", "endpoint", "credential_ref", "credential_reference", "config", "health_diagnostic", "raw_secret", "secret"} {
			if _, exists := definition.Properties[field]; exists {
				t.Errorf("safe collection response definition %s exposes %q", name, field)
			}
		}
	}
	for _, field := range []string{"status", "candidate_count", "accepted_count", "rejected_count", "targets"} {
		if _, exists := definitions["http.CollectionRunResponse"].Properties[field]; !exists {
			t.Errorf("collection run response misses %q", field)
		}
	}
	for _, field := range []string{"healthy", "checked_at"} {
		if _, exists := definitions["http.SourceHealthResponse"].Properties[field]; !exists {
			t.Errorf("source health response misses %q", field)
		}
	}
}

func assertMonitorDraftDefaultsOpenAPI(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	config, ok := definitions["http.MonitorConfigRequest"]
	if !ok {
		t.Fatal("missing http.MonitorConfigRequest")
	}
	if !contains(config.Required, "event_threshold") {
		t.Error("http.MonitorConfigRequest must require event_threshold")
	}
	var threshold struct {
		Minimum *float64 `json:"minimum"`
	}
	if err := json.Unmarshal(config.Properties["event_threshold"], &threshold); err != nil {
		t.Fatalf("decode event_threshold contract: %v", err)
	}
	if threshold.Minimum == nil || *threshold.Minimum != 0 {
		t.Errorf("event_threshold minimum = %v, want explicit 0", threshold.Minimum)
	}

	for _, name := range []string{"http.MonitorRuleRequest", "http.MonitorSourceRequest", "http.AICandidateRequest"} {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		var priority struct {
			Default *int16 `json:"default"`
		}
		if err := json.Unmarshal(definition.Properties["priority"], &priority); err != nil {
			t.Errorf("decode %s priority: %v", name, err)
			continue
		}
		if priority.Default == nil || *priority.Default != 100 {
			t.Errorf("%s priority default = %v, want 100", name, priority.Default)
		}
	}
}

func assertSafeMonitorSourceOpenAPIDefinitions(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	for _, name := range []string{"http.MonitorResponse", "http.MonitorConfigResponse", "http.MonitorRuleResponse", "http.MonitorSourceResponse", "http.PreviewResponse", "http.PreviewSourceResponse"} {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing safe response definition %s", name)
			continue
		}
		for _, field := range []string{"credential_ref", "credential_reference", "endpoint", "config", "health_diagnostic", "raw_secret", "secret"} {
			if _, exists := definition.Properties[field]; exists {
				t.Errorf("safe response definition %s exposes %q", name, field)
			}
		}
	}
	monitorSource := definitions["http.MonitorSourceResponse"]
	for _, field := range []string{"name", "source_type"} {
		if _, exists := monitorSource.Properties[field]; !exists {
			t.Errorf("monitor source response misses %q", field)
		}
	}
	management, ok := definitions["http.ManagementSourceResponse"]
	if !ok {
		t.Error("missing admin source management response definition")
		return
	}
	for _, field := range []string{"credential_ref", "credential_reference", "health_diagnostic", "raw_secret", "secret"} {
		if _, exists := management.Properties[field]; exists {
			t.Errorf("management source response exposes %q", field)
		}
	}
	read, ok := definitions["http.SourceReadResponse"]
	if !ok {
		t.Error("missing role-dependent source read union definition")
	} else {
		for _, field := range []string{"credential_ref", "credential_reference", "health_diagnostic", "raw_secret", "secret"} {
			if _, exists := read.Properties[field]; exists {
				t.Errorf("source read union exposes %q", field)
			}
		}
		for _, field := range []string{"endpoint", "config"} {
			if _, exists := read.Properties[field]; !exists {
				t.Errorf("source read union misses optional admin %q", field)
			}
		}
	}
	config, ok := definitions["http.SourceConfigDTO"]
	if !ok {
		t.Error("missing allowlisted source config definition")
		return
	}
	for _, field := range []string{"credential_ref", "credential_reference", "secret", "raw_secret"} {
		if _, exists := config.Properties[field]; exists {
			t.Errorf("allowlisted source config exposes %q", field)
		}
	}
}

func assertDraftExpectedVersionOpenAPI(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	for _, name := range []string{"http.ReplaceDraftRequest", "http.AICandidateRequest", "http.ApprovalRequest", "http.PublishRequest"} {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		if !contains(definition.Required, "expected_draft_version") {
			t.Errorf("%s must require expected_draft_version", name)
		}
		raw, ok := definition.Properties["expected_draft_version"]
		if !ok {
			t.Errorf("%s misses expected_draft_version", name)
			continue
		}
		var property map[string]any
		if err := json.Unmarshal(raw, &property); err != nil {
			t.Errorf("decode %s expected draft version: %v", name, err)
			continue
		}
		if property["type"] != "integer" || property["x-nullable"] != true {
			t.Errorf("%s expected_draft_version = %#v, want required nullable integer", name, property)
		}
	}
	if lifecycle, ok := definitions["http.LifecycleRequest"]; ok {
		if _, exists := lifecycle.Properties["expected_draft_version"]; exists {
			t.Error("lifecycle request must not expose expected_draft_version")
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func usesBearerAuth(requirements []map[string][]string) bool {
	for _, requirement := range requirements {
		if _, ok := requirement["BearerAuth"]; ok {
			return true
		}
	}
	return false
}

func assertSafeIdentityOpenAPIDefinitions(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	for name, definition := range definitions {
		if name != "http.AuthenticationResponse" && name != "http.UserResponse" && name != "http.ConfirmVerificationResponse" {
			continue
		}
		for _, field := range []string{"password", "password_hash", "refresh_token", "verification_code", "code"} {
			if _, exists := definition.Properties[field]; exists {
				t.Errorf("safe response definition %s exposes %q", name, field)
			}
		}
	}
	confirm := definitions["http.ConfirmVerificationResponse"]
	if _, ok := confirm.Properties["verification_ticket"]; !ok {
		t.Error("email-verification confirmation must expose its single-use ticket")
	}
}
