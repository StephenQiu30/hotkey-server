package architecture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/openapi"
)

type openAPIOperation struct {
	Summary    string                `json:"summary"`
	Tags       []string              `json:"tags"`
	Produces   []string              `json:"produces"`
	Security   []map[string][]string `json:"security"`
	Parameters []openAPIParameter    `json:"parameters"`
	Responses  map[string]struct {
		Schema struct {
			Ref  string `json:"$ref"`
			Type string `json:"type"`
		} `json:"schema"`
	} `json:"responses"`
}

type openAPIParameter struct {
	Name             string   `json:"name"`
	In               string   `json:"in"`
	Required         bool     `json:"required"`
	Type             string   `json:"type"`
	Enum             []string `json:"enum"`
	Minimum          *float64 `json:"minimum"`
	Maximum          *float64 `json:"maximum"`
	CollectionFormat string   `json:"collectionFormat"`
	Items            struct {
		Enum []string `json:"enum"`
	} `json:"items"`
	Schema struct {
		Ref string `json:"$ref"`
	} `json:"schema"`
}

func TestGeneratedOpenAPIRegistryMatchesCommittedArtifact(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "openapi", "swagger.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	generated := openapi.SwaggerInfo.ReadDoc()

	var committedDocument any
	if err := json.Unmarshal(contents, &committedDocument); err != nil {
		t.Fatalf("decode committed OpenAPI document: %v", err)
	}
	var generatedDocument any
	if err := json.Unmarshal([]byte(generated), &generatedDocument); err != nil {
		t.Fatalf("decode generated OpenAPI registry: %v", err)
	}
	normalizeOpenAPIDocument(committedDocument)
	normalizeOpenAPIDocument(generatedDocument)
	if !reflect.DeepEqual(generatedDocument, committedDocument) {
		t.Fatal("generated OpenAPI registry differs from docs/openapi/swagger.json")
	}
}

func normalizeOpenAPIDocument(document any) {
	object, ok := document.(map[string]any)
	if !ok {
		return
	}
	if host, exists := object["host"]; exists && host == "" {
		delete(object, "host")
	}
}

func TestOpenAPIContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "openapi", "swagger.json")
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
		"/api/v1/capabilities":                                {"get": {"200"}},
		"/api/v1/auth/email-verifications":                    {"post": {"200", "400", "429", "503"}},
		"/api/v1/auth/email-verifications/confirm":            {"post": {"200", "400", "429", "503"}},
		"/api/v1/auth/registrations":                          {"post": {"201", "400", "409", "503"}},
		"/api/v1/auth/login":                                  {"post": {"200", "400", "401", "503"}},
		"/api/v1/auth/refresh":                                {"post": {"200", "401", "403", "503"}},
		"/api/v1/auth/logout":                                 {"post": {"200", "403", "503"}},
		"/api/v1/auth/me":                                     {"get": {"200", "401"}},
		"/api/v1/auth/password":                               {"post": {"200", "400", "401", "503"}},
		"/api/v1/auth/password-resets/confirm":                {"post": {"200", "400", "503"}},
		"/api/v1/users":                                       {"get": {"200", "401", "403", "503"}},
		"/api/v1/users/{id}":                                  {"patch": {"200", "400", "401", "403", "409", "503"}, "delete": {"200", "401", "403", "409", "503"}},
		"/api/v1/users/{id}/restore":                          {"post": {"200", "401", "403", "409", "503"}},
		"/api/v1/monitors":                                    {"get": {"200", "400", "401", "503"}, "post": {"201", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}":                               {"get": {"200", "400", "401", "409", "503"}, "put": {"200", "400", "401", "403", "409", "503"}, "delete": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/versions":                      {"get": {"200", "400", "401", "409", "503"}},
		"/api/v1/monitors/{id}/draft":                         {"get": {"200", "400", "401", "403", "404", "503"}, "put": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/draft/intent":                  {"put": {"200", "201", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/monitors/{id}/draft/expansion-runs":          {"post": {"202", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/monitors/{id}/draft/expansion-runs/{run_id}": {"get": {"200", "400", "401", "403", "404", "503"}},
		"/api/v1/monitors/{id}/draft/expansion-candidates/{candidate_id}/decision": {"post": {"200", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/monitors/{id}/draft/preview-runs":                                 {"post": {"202", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/monitors/{id}/draft/preview-runs/{run_id}":                        {"get": {"200", "400", "401", "403", "404", "503"}},
		"/api/v1/monitors/{id}/draft/ai-candidates":                                {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/draft/rules/{rule_id}/approval":                     {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/preview":                                            {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/publish":                                            {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/pause":                                              {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/resume":                                             {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/archive":                                            {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/restore":                                            {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-connections":                                               {"get": {"200", "400", "401", "503"}, "post": {"201", "400", "401", "403", "409", "503"}},
		"/api/v1/source-presets":                                                   {"get": {"200", "401", "403"}},
		"/api/v1/source-connections/{id}":                                          {"get": {"200", "400", "401", "409", "503"}, "patch": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-connections/{id}/enable":                                   {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-connections/{id}/disable":                                  {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-connections/{id}/archive":                                  {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-connections/{id}/restore":                                  {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-endpoints/{id}/capabilities":                               {"get": {"200", "400", "401", "404", "503"}},
		"/api/v1/source-endpoints/{id}/rights-policies":                            {"get": {"200", "400", "401", "403", "404", "503"}, "post": {"201", "400", "401", "403", "409", "503"}},
		"/api/v1/source-endpoints/{id}/rights-decision-batches":                    {"get": {"200", "400", "401", "403", "404", "503"}, "post": {"201", "400", "401", "403", "409", "503"}},
		"/api/v1/source-endpoints/{id}/rights-decisions/{decision_id}":             {"get": {"200", "400", "401", "403", "404", "503"}},
		"/api/v1/source-endpoints/{id}/rights-evaluations":                         {"post": {"200", "400", "401", "403", "404", "503"}},
		"/api/v1/metric-capability-profiles":                                       {"post": {"201", "400", "401", "403", "409", "503"}},
		"/api/v1/metric-capability-profiles/{id}/publish":                          {"post": {"200", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/metric-capability-profiles/{id}/archive":                          {"post": {"200", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/collection-runs":                                                  {"get": {"200", "400", "401", "403", "503"}},
		"/api/v1/collection-runs/{id}/retry":                                       {"post": {"200", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/monitors/{id}/collect":                                            {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/monitors/{id}/scans":                                              {"get": {"200", "400", "401", "503"}},
		"/api/v1/search":                                                           {"post": {"200", "400", "401", "503"}},
		"/api/v1/source-connections/{id}/health":                                   {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/source-webhooks/bilibili":                                         {"post": {"200", "400", "401"}},
		"/api/v1/operations/jobs":                                                  {"get": {"200", "400", "401", "403", "503"}},
		"/api/v1/operations/jobs/{id}/cancel":                                      {"post": {"200", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/operations/jobs/{id}/retry":                                       {"post": {"200", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/operations/overview":                                              {"get": {"200", "401", "403", "503"}},
		"/api/v1/operations/usage":                                                 {"get": {"200", "401", "403", "503"}},
		"/api/v1/operations/retention-policies":                                    {"get": {"200", "401", "403", "503"}},
		"/api/v1/operations/retention-policies/{id}/preview":                       {"post": {"200", "400", "401", "403", "404", "409"}},
		"/api/v1/operations/retention-policies/{id}/run":                           {"post": {"200", "400", "401", "403", "404", "409"}},
		"/api/v1/operations/audit-logs":                                            {"get": {"200", "400", "401", "403", "503"}},
		"/api/v1/contents":                                                         {"get": {"200", "400", "401", "404", "503"}},
		"/api/v1/hotspots":                                                         {"get": {"200", "400", "401", "503"}},
		"/api/v1/contents/{id}":                                                    {"get": {"200", "400", "401", "404", "503"}},
		"/api/v1/contents/{id}/document":                                           {"get": {"200", "400", "401", "404", "503"}},
		"/api/v1/document-versions/{id}/citation":                                  {"get": {"200", "400", "401", "404", "502", "503"}},
		"/api/v1/document-versions/{id}/document":                                  {"get": {"200", "304", "400", "401", "403", "404", "409", "502", "503"}},
		"/api/v1/document-versions/{id}/text-quote-selectors":                      {"post": {"201", "400", "401", "403", "404", "409"}},
		"/api/v1/content-lineage-decisions/{id}/feedback":                          {"post": {"200", "201", "400", "401", "403", "404", "409"}},
		"/api/v1/monitors/{id}/document-matches":                                   {"get": {"200", "400", "401", "403", "503"}},
		"/api/v1/monitors/{id}/document-matches/{match_decision_id}/overrides":     {"post": {"200", "201", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/monitors/{id}/matches":                                            {"get": {"200", "400", "401", "404", "503"}},
		"/api/v1/monitors/{id}/matches/{match_id}":                                 {"get": {"200", "400", "401", "404", "503"}},
		"/api/v1/monitors/{id}/relevance-preview":                                  {"post": {"200", "400", "401", "403", "404", "503"}},
		"/api/v1/monitors/{id}/matches/{match_id}/feedback":                        {"put": {"200", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/monitors/{id}/contents/{content_id}/feedback":                     {"put": {"200", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/monitors/{id}/feedback/evaluation":                                {"get": {"200", "400", "401", "403", "404", "503"}},
		"/api/v1/monitors/{id}/feedback/suggestions/refresh":                       {"post": {"200", "400", "401", "403", "404", "503"}},
		"/api/v1/monitors/{id}/feedback/suggestions":                               {"get": {"200", "400", "401", "403", "404", "503"}},
		"/api/v1/monitors/{id}/feedback/suggestions/{suggestion_id}/review":        {"post": {"200", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/ai/model-profiles":                                                {"get": {"200", "401", "403", "503"}, "post": {"201", "400", "401", "403", "503"}},
		"/api/v1/ai/model-profiles/{id}":                                           {"get": {"200", "400", "401", "403", "503"}, "patch": {"200", "400", "401", "403", "409", "503"}, "delete": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/ai/model-profiles/{id}/restore":                                   {"post": {"200", "400", "401", "403", "409", "503"}},
		"/api/v1/ai/runs/{id}/recompute":                                           {"post": {"202", "400", "401", "403", "404", "409", "503"}},
		"/api/v1/micro-events":                                                     {"get": {"200", "400", "401", "503"}},
		"/api/v1/micro-events/{id}":                                                {"get": {"200", "400", "401", "404"}},
		"/api/v1/micro-events/{id}/evidence":                                       {"get": {"200", "400", "401", "404"}, "post": {"200", "201", "400", "401", "403", "409"}},
		"/api/v1/micro-events/{id}/evidence/{evidence_id}/feedback":                {"post": {"200", "201", "400", "401", "403", "409"}},
		"/api/v1/micro-events/{id}/feedback":                                       {"post": {"200", "400", "401", "403", "409"}},
		"/api/v1/notifications":                                                    {"get": {"200", "400", "401", "503"}},
		"/api/v1/notifications/ws":                                                 {"get": {"101", "400", "503"}},
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
			if strings.TrimSpace(operation.Summary) == "" {
				t.Errorf("%s %s is missing an annotation-generated summary", strings.ToUpper(method), route)
			}
			if len(operation.Tags) == 0 {
				t.Errorf("%s %s is missing annotation-generated tags", strings.ToUpper(method), route)
			}
			for _, status := range statuses {
				response, ok := operation.Responses[status]
				if route == "/api/v1/notifications/ws" && status == "101" {
					if !ok || response.Schema.Ref != "" || response.Schema.Type != "string" {
						t.Errorf("GET %s response 101 must describe the WebSocket upgrade", route)
					}
					continue
				}
				if route == "/api/v1/document-versions/{id}/document" && status == "304" {
					if !ok || response.Schema.Ref != "" || response.Schema.Type != "" {
						t.Errorf("GET %s response 304 must be bodyless", route)
					}
					continue
				}
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

	for _, route := range []string{"/api/v1/auth/me", "/api/v1/auth/password", "/api/v1/users", "/api/v1/users/{id}", "/api/v1/users/{id}/restore", "/api/v1/monitors", "/api/v1/monitors/{id}", "/api/v1/monitors/{id}/draft", "/api/v1/monitors/{id}/draft/ai-candidates", "/api/v1/monitors/{id}/draft/rules/{rule_id}/approval", "/api/v1/monitors/{id}/preview", "/api/v1/monitors/{id}/publish", "/api/v1/monitors/{id}/pause", "/api/v1/monitors/{id}/resume", "/api/v1/monitors/{id}/archive", "/api/v1/monitors/{id}/restore", "/api/v1/monitors/{id}/collect", "/api/v1/monitors/{id}/scans", "/api/v1/search", "/api/v1/source-connections", "/api/v1/source-connections/{id}", "/api/v1/source-connections/{id}/enable", "/api/v1/source-connections/{id}/disable", "/api/v1/source-connections/{id}/archive", "/api/v1/source-connections/{id}/restore", "/api/v1/metric-capability-profiles", "/api/v1/metric-capability-profiles/{id}/publish", "/api/v1/metric-capability-profiles/{id}/archive", "/api/v1/collection-runs", "/api/v1/collection-runs/{id}/retry", "/api/v1/source-connections/{id}/health", "/api/v1/operations/jobs", "/api/v1/operations/jobs/{id}/cancel", "/api/v1/operations/jobs/{id}/retry", "/api/v1/operations/usage", "/api/v1/operations/retention-policies", "/api/v1/operations/retention-policies/{id}/preview", "/api/v1/operations/retention-policies/{id}/run", "/api/v1/operations/audit-logs", "/api/v1/knowledge/documents", "/api/v1/knowledge/documents/{id}", "/api/v1/knowledge/proposals", "/api/v1/knowledge/proposals/{id}", "/api/v1/knowledge/proposals/{id}/approve", "/api/v1/knowledge/proposals/{id}/reject", "/api/v1/knowledge/proposals/{id}/apply", "/api/v1/knowledge/reconcile", "/api/v1/contents", "/api/v1/contents/{id}", "/api/v1/contents/{id}/document", "/api/v1/document-versions/{id}/citation", "/api/v1/document-versions/{id}/document", "/api/v1/monitors/{id}/matches", "/api/v1/monitors/{id}/matches/{match_id}", "/api/v1/monitors/{id}/relevance-preview", "/api/v1/monitors/{id}/matches/{match_id}/feedback", "/api/v1/monitors/{id}/contents/{content_id}/feedback", "/api/v1/monitors/{id}/feedback/evaluation", "/api/v1/monitors/{id}/feedback/suggestions/refresh", "/api/v1/monitors/{id}/feedback/suggestions", "/api/v1/monitors/{id}/feedback/suggestions/{suggestion_id}/review", "/api/v1/ai/model-profiles", "/api/v1/ai/model-profiles/{id}", "/api/v1/ai/model-profiles/{id}/restore", "/api/v1/events", "/api/v1/events/{id}", "/api/v1/events/{id}/contents", "/api/v1/events/{id}/heat", "/api/v1/events/{id}/intelligence", "/api/v1/events/{id}/intelligence/summary/regenerate", "/api/v1/events/{id}/contents/{content_id}/lock", "/api/v1/events/{id}/lifecycle", "/api/v1/events/{id}/merge", "/api/v1/events/{id}/split", "/api/v1/radar/events", "/api/v1/events/{id}/updates", "/api/v1/alerts", "/api/v1/alerts/{id}", "/api/v1/alerts/{id}/acknowledge", "/api/v1/alerts/{id}/resolve", "/api/v1/alerts/{id}/suppress", "/api/v1/reports", "/api/v1/reports/{id}", "/api/v1/reports/{id}/preview", "/api/v1/reports/{id}/build", "/api/v1/reports/{id}/publish", "/api/v1/report-subscriptions", "/api/v1/report-subscriptions/{id}", "/api/v1/report-subscriptions/{id}/rss-token/rotate", "/api/v1/notifications"} {
		var operations map[string]openAPIOperation
		rawPath, exists := document.Paths[route]
		if !exists {
			continue
		}
		if err := json.Unmarshal(rawPath, &operations); err != nil {
			t.Fatalf("decode protected path %s: %v", route, err)
		}
		for method, operation := range operations {
			if !usesBearerAuth(operation.Security) {
				t.Errorf("%s %s is missing BearerAuth", strings.ToUpper(method), route)
			}
		}
	}
	for _, route := range []string{
		"/api/v1/monitors/{id}/draft/intent",
		"/api/v1/monitors/{id}/draft/expansion-runs",
		"/api/v1/monitors/{id}/draft/expansion-runs/{run_id}",
		"/api/v1/monitors/{id}/draft/expansion-candidates/{candidate_id}/decision",
		"/api/v1/monitors/{id}/draft/preview-runs",
		"/api/v1/monitors/{id}/draft/preview-runs/{run_id}",
		"/api/v1/monitors/{id}/document-matches",
		"/api/v1/monitors/{id}/document-matches/{match_decision_id}/overrides",
	} {
		var operations map[string]openAPIOperation
		if err := json.Unmarshal(document.Paths[route], &operations); err != nil {
			t.Fatalf("decode protected monitor intent path %s: %v", route, err)
		}
		for method, operation := range operations {
			if !usesBearerAuth(operation.Security) {
				t.Errorf("%s %s is missing BearerAuth", strings.ToUpper(method), route)
			}
		}
	}
	for _, route := range []string{
		"/api/v1/source-endpoints/{id}/capabilities",
		"/api/v1/source-endpoints/{id}/rights-policies",
		"/api/v1/source-endpoints/{id}/rights-decision-batches",
		"/api/v1/source-endpoints/{id}/rights-decisions/{decision_id}",
		"/api/v1/source-endpoints/{id}/rights-evaluations",
		"/api/v1/source-webhooks/bilibili",
	} {
		var operations map[string]openAPIOperation
		if err := json.Unmarshal(document.Paths[route], &operations); err != nil {
			t.Fatalf("decode protected source rights path %s: %v", route, err)
		}
		if route == "/api/v1/source-webhooks/bilibili" {
			if _, ok := operations["post"]; !ok {
				t.Errorf("POST %s is missing", route)
			}
			continue
		}
		for method, operation := range operations {
			if !usesBearerAuth(operation.Security) {
				t.Errorf("%s %s is missing BearerAuth", strings.ToUpper(method), route)
			}
		}
	}

	for _, route := range []string{
		"/api/v1/events", "/api/v1/events/{id}", "/api/v1/events/{id}/contents", "/api/v1/events/{id}/contents/{content_id}/lock",
		"/api/v1/events/{id}/heat", "/api/v1/events/{id}/intelligence", "/api/v1/events/{id}/intelligence/summary/regenerate",
		"/api/v1/events/{id}/lifecycle", "/api/v1/events/{id}/merge", "/api/v1/events/{id}/split", "/api/v1/events/{id}/updates", "/api/v1/radar/events",
		"/api/v1/alerts", "/api/v1/alerts/{id}", "/api/v1/alerts/{id}/acknowledge", "/api/v1/alerts/{id}/resolve", "/api/v1/alerts/{id}/suppress",
		"/api/v1/reports", "/api/v1/reports/{id}", "/api/v1/reports/{id}/preview", "/api/v1/reports/{id}/build", "/api/v1/reports/{id}/publish",
		"/api/v1/reports/{id}/submit", "/api/v1/reports/{id}/approve", "/api/v1/reports/{id}/reject",
		"/api/v1/report-subscriptions", "/api/v1/report-subscriptions/{id}", "/api/v1/report-subscriptions/{id}/rss-token/rotate",
		"/api/v1/knowledge/documents", "/api/v1/knowledge/documents/{id}", "/api/v1/knowledge/proposals", "/api/v1/knowledge/proposals/{id}",
		"/api/v1/knowledge/proposals/{id}/approve", "/api/v1/knowledge/proposals/{id}/reject", "/api/v1/knowledge/proposals/{id}/apply", "/api/v1/knowledge/reconcile",
		"/api/v1/agent-tokens", "/api/v1/agent-tokens/{id}/revoke",
		"/api/v1/agent/monitors", "/api/v1/agent/monitors/{id}", "/api/v1/agent/monitors/{id}/collect",
		"/api/v1/agent/contents", "/api/v1/agent/contents/{id}", "/api/v1/agent/contents/{id}/document",
		"/api/v1/agent/events", "/api/v1/agent/events/{id}", "/api/v1/agent/events/{id}/contents", "/api/v1/agent/events/{id}/heat", "/api/v1/agent/events/{id}/intelligence", "/api/v1/agent/events/{id}/updates", "/api/v1/agent/radar/events",
		"/api/v1/agent/alerts", "/api/v1/agent/alerts/{id}", "/api/v1/agent/alerts/{id}/acknowledge", "/api/v1/agent/alerts/{id}/resolve",
		"/api/v1/agent/reports", "/api/v1/agent/reports/{id}",
	} {
		if _, exists := document.Paths[route]; exists {
			t.Errorf("retired product path %s remains in OpenAPI", route)
		}
	}

	for _, route := range []string{"/healthz", "/readyz", "/metrics"} {
		if _, exists := document.Paths[route]; exists {
			t.Errorf("operational path %s must not be in OpenAPI", route)
		}
	}
	assertSafeIdentityOpenAPIDefinitions(t, document.Definitions)
	assertSafeMonitorSourceOpenAPIDefinitions(t, document.Definitions)
	assertSafeMonitorIntentOpenAPI(t, document.Paths, document.Definitions)
	assertSafeDocumentMatchOpenAPI(t, document.Definitions)
	assertSafeCollectionOpenAPIDefinitions(t, document.Definitions)
	assertSafeContentOpenAPIDefinitions(t, document.Definitions)
	assertSafeVersionedCitationOpenAPIDefinitions(t, document.Definitions)
	assertSafeRelevanceOpenAPIDefinitions(t, document.Definitions)
	assertFalseNegativeContentFeedbackOpenAPI(t, document.Paths)
	assertSafeModelProfileOpenAPIDefinitions(t, document.Definitions)
	assertMetricCapabilityOpenAPI(t, document.Paths, document.Definitions)
	assertDraftExpectedVersionOpenAPI(t, document.Definitions)
	assertMonitorDraftDefaultsOpenAPI(t, document.Definitions)
	assertSimpleMonitorResponseOpenAPI(t, document.Definitions)
	assertExactResultEnvelopeDefinitions(t, document.Definitions)
}

func assertExactResultEnvelopeDefinitions(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	want := []string{"code", "data", "message"}
	found := 0
	for name, definition := range definitions {
		if !strings.HasPrefix(name, "http.") || (!strings.Contains(name, "Result-") && name != "http.Result-http_Capabilities") {
			continue
		}
		found++
		fields := make([]string, 0, len(definition.Properties))
		for field := range definition.Properties {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		if !reflect.DeepEqual(fields, want) {
			t.Errorf("%s fields = %v, want only %v", name, fields, want)
		}
	}
	if found == 0 {
		t.Fatal("published OpenAPI has no Result envelope definitions")
	}
}

func assertSimpleMonitorResponseOpenAPI(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	monitor, ok := definitions["http.MonitorResponse"]
	if !ok {
		t.Error("missing http.MonitorResponse")
		return
	}
	for _, field := range []string{"id", "version", "name", "status", "query", "collection_interval_seconds", "sources"} {
		if _, exists := monitor.Properties[field]; !exists {
			t.Errorf("simple MonitorResponse misses %q", field)
		}
	}
	for _, redundant := range []string{"published", "draft", "published_revision", "config"} {
		if _, exists := monitor.Properties[redundant]; exists {
			t.Errorf("simple MonitorResponse exposes redundant %q", redundant)
		}
	}
	source := definitions["http.MonitorSourceResponse"]
	for _, redundant := range []string{"id", "query_override", "priority"} {
		if _, exists := source.Properties[redundant]; exists {
			t.Errorf("simple MonitorSourceResponse exposes redundant %q", redundant)
		}
	}
}

func assertSafeVersionedCitationOpenAPIDefinitions(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	allowed := map[string]map[string]bool{
		"http.CitationArtifactResponseDTO": {
			"artifact_type": true, "transformer_profile_sha256": true, "mime_type": true,
			"sha256": true, "size_bytes": true, "etag": true, "anchor_map": true,
		},
		"http.CitationArtifactAnchorBlockResponseDTO": {
			"ordinal": true, "markdown_anchor": true,
		},
		"http.CitationArtifactAnchorMapResponseDTO": {
			"normalization_version": true, "anchor_map_profile_version": true,
			"anchor_map_sha256": true, "blocks": true,
		},
		"http.CitationAnchorMapResponseDTO": {
			"normalization_version": true, "anchor_map_version": true, "markdown_anchor": true,
		},
		"http.CitationPartyResponseDTO": {
			"role": true, "kind": true, "identity_namespace": true, "external_id": true,
			"display_name": true, "homepage_url": true,
		},
		"http.CitationResponseDTO": {
			"document_id": true, "document_version_id": true, "source_type": true, "source_name": true,
			"title": true, "author": true, "publisher": true, "publisher_party": true,
			"content_origin": true, "distributors": true, "publisher_availability": true,
			"publisher_unavailable_reason": true, "content_origin_availability": true,
			"content_origin_unavailable_reason": true, "source_record_url": true, "canonical_url": true,
			"discussion_url": true, "body_origin": true, "completeness": true, "language": true,
			"published_at": true, "published_utc_offset_minutes": true, "captured_at": true, "content_sha256": true, "availability": true,
			"unavailable_reason": true, "artifact": true, "locator_availability": true,
			"locator_unavailable_reason": true, "exact_quote": true, "utf8_byte_start": true,
			"utf8_byte_end": true, "anchor_map": true, "raw_evidence": true,
		},
		"http.CitationRawEvidenceResponseDTO": {
			"availability": true, "payload_sha256s": true, "retention_until": true,
			"deletion_audited": true, "exception_approved": true,
		},
		"http.VersionedDocumentResponseDTO": {"citation": true, "markdown": true, "etag": true},
	}
	for name, fields := range allowed {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		for field := range definition.Properties {
			if !fields[field] {
				t.Errorf("%s exposes non-allowlisted field %q", name, field)
			}
			for _, forbidden := range []string{"object_key", "bucket", "credential", "rights_decision", "raw_payload", "provider_payload"} {
				if strings.Contains(field, forbidden) {
					t.Errorf("%s exposes internal field %q", name, field)
				}
			}
		}
		for field := range fields {
			if _, ok := definition.Properties[field]; !ok {
				t.Errorf("%s misses %q", name, field)
			}
		}
	}
	var availability struct {
		Enum []string `json:"enum"`
	}
	if err := json.Unmarshal(definitions["http.CitationResponseDTO"].Properties["availability"], &availability); err != nil {
		t.Fatalf("decode Citation availability: %v", err)
	}
	wantAvailability := []string{
		"full_archive", "partial_archive", "summary_only", "metadata_only",
		"policy_blocked", "temporarily_unavailable", "quarantined", "tombstoned",
	}
	if !reflect.DeepEqual(availability.Enum, wantAvailability) {
		t.Errorf("Citation availability = %v, want %v", availability.Enum, wantAvailability)
	}
}

func assertSafeMonitorIntentOpenAPI(t *testing.T, paths map[string]json.RawMessage, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	allowed := map[string]map[string]bool{
		"http.IntentRunAcceptedResponseDTO": {
			"run_id": true, "kind": true, "monitor_id": true, "draft_id": true, "resource_version": true,
			"input_hash": true, "status": true, "status_url": true, "reused": true,
		},
		"http.IntentExpansionRunStatusResponseDTO": {
			"run_id": true, "kind": true, "monitor_id": true, "draft_id": true, "resource_version": true,
			"input_hash": true, "status": true, "status_url": true, "queued_at": true, "started_at": true,
			"completed_at": true, "invalidated_at": true, "failure_code": true, "candidates": true,
		},
		"http.IntentPreviewRunStatusResponseDTO": {
			"run_id": true, "kind": true, "monitor_id": true, "draft_id": true, "resource_version": true,
			"input_hash": true, "status": true, "status_url": true, "queued_at": true, "started_at": true,
			"completed_at": true, "invalidated_at": true, "failure_code": true, "preview": true,
		},
		"http.IntentExpansionCandidateResponseDTO": {
			"id": true, "value": true, "source": true, "reason": true, "model_version": true,
			"prompt_version": true, "input_hash": true, "similarity": true, "risk": true,
			"approval_status": true, "reviewer_user_id": true, "reviewed_at": true, "review_note": true,
		},
		"http.IntentPreviewRecallSignalResponseDTO": {"channel": true, "rank": true, "raw_score": true},
		"http.IntentPreviewSampleResponseDTO": {
			"document_version_id": true, "title": true, "decision": true, "recall_signals": true,
			"reasons": true, "exclusion_reasons": true,
		},
		"http.IntentPreviewResponseDTO": {
			"samples": true, "estimated_alert_count": true, "warnings": true,
		},
	}
	for name, fields := range allowed {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		for field := range definition.Properties {
			if !fields[field] {
				t.Errorf("%s exposes non-allowlisted field %q", name, field)
			}
			for _, forbidden := range []string{"body", "markdown", "raw", "objective", "provider_payload", "prompt_content", "compiled_profile"} {
				if forbidden == "raw" && field == "raw_score" {
					continue
				}
				if strings.Contains(field, forbidden) {
					t.Errorf("%s exposes forbidden field %q", name, field)
				}
			}
		}
		for field := range fields {
			if _, ok := definition.Properties[field]; !ok {
				t.Errorf("%s misses %q", name, field)
			}
		}
	}

	for _, name := range []string{"http.IntentExpansionRunStatusResponseDTO", "http.IntentPreviewRunStatusResponseDTO"} {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		for field := range definition.Properties {
			for _, forbidden := range []string{"body", "markdown", "raw", "objective", "provider_payload", "prompt_content", "compiled_profile"} {
				if forbidden == "raw" && field == "raw_score" {
					continue
				}
				if strings.Contains(field, forbidden) {
					t.Errorf("%s exposes forbidden field %q", name, field)
				}
			}
		}
	}

	operation := func(route, method string) openAPIOperation {
		var operations map[string]openAPIOperation
		if err := json.Unmarshal(paths[route], &operations); err != nil {
			t.Fatalf("decode monitor intent route %s: %v", route, err)
		}
		return operations[method]
	}
	parameter := func(value openAPIOperation, name string) openAPIParameter {
		for _, candidate := range value.Parameters {
			if candidate.Name == name {
				return candidate
			}
		}
		t.Errorf("monitor intent operation misses parameter %q", name)
		return openAPIParameter{}
	}
	put := operation("/api/v1/monitors/{id}/draft/intent", "put")
	if parameter(put, "If-Match").Required || parameter(put, "If-None-Match").Required {
		t.Error("intent PUT conditional headers must remain mutually exclusive optional OpenAPI parameters")
	}
	for _, route := range []string{
		"/api/v1/monitors/{id}/draft/expansion-runs",
		"/api/v1/monitors/{id}/draft/expansion-candidates/{candidate_id}/decision",
		"/api/v1/monitors/{id}/draft/preview-runs",
	} {
		post := operation(route, "post")
		for _, header := range []string{"If-Match", "Idempotency-Key"} {
			candidate := parameter(post, header)
			if candidate.In != "header" || !candidate.Required {
				t.Errorf("POST %s %s must be a required header", route, header)
			}
		}
	}
}

func assertSafeDocumentMatchOpenAPI(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	expected := map[string][]string{
		"http.DocumentMatchSignalResponseDTO": {
			"algorithm_version", "channel", "rank", "raw_score",
		},
		"http.DocumentMatchResponseDTO": {
			"match_decision_id", "monitor_id", "monitor_version_id", "compiled_profile_id",
			"document_version_id", "relevance_profile_id", "matching_algorithm_version",
			"reranker_version", "calibration_version", "rrf_score", "relevance_probability",
			"automatic_decision", "effective_decision", "degraded", "reason_codes", "signals",
			"resource_version", "decided_at",
		},
		"http.DocumentMatchPageResponseDTO": {
			"items", "next_cursor",
		},
		"http.OverrideDocumentMatchRequestDTO": {
			"decision", "reason_code", "note",
		},
		"http.OverrideDocumentMatchResponseDTO": {
			"override_id", "match_decision_id", "monitor_id", "monitor_version_id",
			"document_version_id", "previous_effective_decision", "decision", "reason_code",
			"note", "actor_user_id", "resource_version", "reused", "created_at",
		},
	}
	for name, fields := range expected {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		if len(definition.Properties) != len(fields) {
			t.Errorf("%s property count = %d, want %d", name, len(definition.Properties), len(fields))
		}
		for _, field := range fields {
			if _, ok := definition.Properties[field]; !ok {
				t.Errorf("%s misses %q", name, field)
			}
		}
		for field := range definition.Properties {
			for _, forbidden := range []string{
				"body", "markdown", "raw_payload", "object_key", "input_hash", "truth",
				"credibility", "verification", "confirmation", "unverified", "corroborated",
			} {
				if strings.Contains(field, forbidden) {
					t.Errorf("%s exposes forbidden field %q", name, field)
				}
			}
		}
	}
}

func assertSafeAgentTokenOpenAPIDefinitions(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	response, ok := definitions["http.TokenResponse"]
	if !ok {
		t.Fatal("missing Agent Token response definition")
	}
	for _, forbidden := range []string{"token", "token_hash", "authorization"} {
		if _, exists := response.Properties[forbidden]; exists {
			t.Errorf("safe Agent Token response exposes %q", forbidden)
		}
	}
	created, ok := definitions["http.CreatedTokenResponse"]
	if !ok {
		t.Fatal("missing one-time Agent Token response definition")
	}
	if _, exists := created.Properties["token"]; !exists {
		t.Error("created Agent Token response must expose the one-time token")
	}
}

func assertRadarAlertParameterContracts(t *testing.T, paths map[string]json.RawMessage, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	operations := func(route string) map[string]openAPIOperation {
		var result map[string]openAPIOperation
		if err := json.Unmarshal(paths[route], &result); err != nil {
			t.Fatalf("decode parameter contract %s: %v", route, err)
		}
		return result
	}
	parameter := func(operation openAPIOperation, name string) openAPIParameter {
		for _, candidate := range operation.Parameters {
			if candidate.Name == name {
				return candidate
			}
		}
		t.Fatalf("missing parameter %s", name)
		return openAPIParameter{}
	}
	assertRange := func(candidate openAPIParameter, minimum, maximum float64) {
		if candidate.Minimum == nil || candidate.Maximum == nil || *candidate.Minimum != minimum || *candidate.Maximum != maximum {
			t.Errorf("parameter %s range = %v/%v, want %v/%v", candidate.Name, candidate.Minimum, candidate.Maximum, minimum, maximum)
		}
	}
	assertEnum := func(candidate openAPIParameter, expected []string) {
		actual := candidate.Enum
		if candidate.Type == "array" {
			actual = candidate.Items.Enum
			if candidate.CollectionFormat != "csv" {
				t.Errorf("parameter %s collection format = %q, want csv", candidate.Name, candidate.CollectionFormat)
			}
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("parameter %s enum = %v, want %v", candidate.Name, actual, expected)
		}
	}

	radar := operations("/api/v1/radar/events")["get"]
	assertEnum(parameter(radar, "window"), []string{"1h", "6h", "24h", "7d"})
	assertEnum(parameter(radar, "lifecycle"), []string{"detected", "active", "cooling", "closed", "merged", "archived", "rejected"})
	assertEnum(parameter(radar, "trend"), []string{"emerging", "rising", "stable", "falling", "dormant"})
	for _, candidate := range radar.Parameters {
		if candidate.Name == "verification" {
			t.Error("Radar must not expose legacy truth/verification filters")
		}
	}
	assertEnum(parameter(radar, "sort"), []string{"momentum", "attention", "breadth", "latest", "relevance"})
	assertRange(parameter(radar, "min_heat"), 0, 100)
	assertRange(parameter(radar, "limit"), 1, 100)
	monitor := parameter(radar, "monitor_id")
	if monitor.Minimum == nil || *monitor.Minimum != 1 {
		t.Errorf("Radar monitor_id minimum = %v, want 1", monitor.Minimum)
	}

	alerts := operations("/api/v1/alerts")["get"]
	assertEnum(parameter(alerts, "state"), []string{"open", "acknowledged", "resolved", "suppressed"})
	assertEnum(parameter(alerts, "severity"), []string{"info", "warning", "critical"})
	assertRange(parameter(alerts, "limit"), 1, 100)
	alertMonitor := parameter(alerts, "monitor_id")
	if alertMonitor.Minimum == nil || *alertMonitor.Minimum != 1 {
		t.Errorf("Alert monitor_id minimum = %v, want 1", alertMonitor.Minimum)
	}

	action := definitions["http.AlertActionRequest"]
	var expectedVersion struct {
		Minimum *float64 `json:"minimum"`
	}
	if err := json.Unmarshal(action.Properties["expected_version"], &expectedVersion); err != nil || expectedVersion.Minimum == nil || *expectedVersion.Minimum != 1 {
		t.Errorf("AlertActionRequest expected_version minimum = %v/%v, want 1", expectedVersion.Minimum, err)
	}
}

func assertSafeRadarAlertOpenAPI(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	for name, required := range map[string][]string{
		"http.RadarEventResponse":      {"event_id", "event_key", "title", "attention", "momentum", "breadth", "independent_source_count", "ranking_score", "reason_codes", "last_seen_at"},
		"http.RadarPageResponse":       {"items", "as_of"},
		"http.EventUpdateResponse":     {"id", "event_id", "sequence_no", "kind", "summary", "observed_at", "reason_codes", "after_state"},
		"http.AlertThreadResponse":     {"id", "version", "monitor_id", "event_id", "trigger_type", "state", "severity", "occurrence_count", "last_triggered_at", "cooldown_until"},
		"http.AlertDetailResponse":     {"thread", "occurrences", "audits"},
		"http.AlertOccurrenceResponse": {"id", "event_update_id", "severity", "final_score", "threshold", "triggered_at"},
		"http.AlertStateAuditResponse": {"id", "actor_type", "from_state", "to_state", "reason_code", "created_at"},
		"http.AlertActionRequest":      {"expected_version", "reason_code"},
	} {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		for _, field := range required {
			if _, ok := definition.Properties[field]; !ok {
				t.Errorf("%s misses %q", name, field)
			}
		}
		for field := range definition.Properties {
			for _, forbidden := range []string{"credential", "provider", "prompt", "payload", "sql", "config", "truth", "credibility", "verification", "confirmation", "confidence", "corroborated", "unverified", "confirmed"} {
				if strings.Contains(field, forbidden) {
					t.Errorf("%s exposes forbidden field %q", name, field)
				}
			}
		}
	}
}

func assertSafeEventIntelligenceOpenAPI(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	for name, required := range map[string][]string{
		"http.EventIntelligenceResponse":    {"event_id", "entities", "claims"},
		"http.IntelligenceEntityResponse":   {"entity_id", "entity_key", "entity_type", "canonical_name", "role", "origin"},
		"http.IntelligenceClaimResponse":    {"id", "normalized_claim", "claim_hash", "evidence"},
		"http.IntelligenceEvidenceResponse": {"content_id", "locator", "excerpt", "stance"},
		"http.SummaryRegenerationResponse":  {"event_id", "status", "summary"},
		"http.EventSummaryResponse":         {"version", "title_zh", "degraded", "sentences"},
	} {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		for _, field := range required {
			if _, ok := definition.Properties[field]; !ok {
				t.Errorf("%s misses %q", name, field)
			}
		}
		for field := range definition.Properties {
			if strings.Contains(field, "provider") || strings.Contains(field, "prompt") || strings.Contains(field, "structured_result") || strings.Contains(field, "credential") || strings.Contains(field, "truth") || strings.Contains(field, "confidence") || strings.Contains(field, "confirmed") || strings.Contains(field, "verification") || strings.Contains(field, "corroborated") || strings.Contains(field, "unverified") {
				t.Errorf("%s exposes intelligence implementation field %q", name, field)
			}
		}
	}
}

func assertHeatOpenAPIDefinitions(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	for name, required := range map[string][]string{
		"http.EventResponse": {"heat_score", "trend_score", "trend_status", "window_hours", "heat_version", "reason_codes", "capability_profile_set_hash", "calculated_at"},
		"http.HeatResponse":  {"heat_score", "trend_score", "trend_status", "source_count", "content_count", "window_hours", "heat_version", "evidence_set_hash", "capability_profile_set_hash", "reason_codes", "captured_at"},
	} {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		for _, field := range required {
			if _, ok := definition.Properties[field]; !ok {
				t.Errorf("%s misses %q", name, field)
			}
		}
		for field := range definition.Properties {
			if strings.Contains(field, "weight") || strings.Contains(field, "threshold") || strings.Contains(field, "normalization") || strings.Contains(field, "independence") {
				t.Errorf("%s exposes internal heat configuration %q", name, field)
			}
		}
	}
}

func assertMetricCapabilityOpenAPI(t *testing.T, paths map[string]json.RawMessage, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	response, ok := definitions["http.MetricCapabilityProfileResponse"]
	if !ok {
		t.Fatal("missing metric capability profile response definition")
	}
	allowedResponse := map[string]bool{
		"id": true, "version": true, "source_type": true, "profile_version": true, "supports_views": true,
		"supports_likes": true, "supports_comments": true, "supports_shares": true, "independence_strategy": true,
		"normalization_window_hours": true, "max_single_item_contribution": true,
		"status": true, "published_at": true, "archived_at": true,
	}
	for field := range response.Properties {
		if !allowedResponse[field] {
			t.Errorf("metric capability response exposes %q", field)
		}
	}
	for field := range allowedResponse {
		if _, ok := response.Properties[field]; !ok {
			t.Errorf("metric capability response misses %q", field)
		}
	}

	expectBody := func(route, method, reference string) {
		t.Helper()
		var operations map[string]openAPIOperation
		if err := json.Unmarshal(paths[route], &operations); err != nil {
			t.Fatalf("decode %s: %v", route, err)
		}
		for _, parameter := range operations[method].Parameters {
			if parameter.In == "body" && parameter.Schema.Ref == reference {
				return
			}
		}
		t.Errorf("%s %s body must use %s", strings.ToUpper(method), route, reference)
	}
	expectBody("/api/v1/metric-capability-profiles", "post", "#/definitions/http.CreateMetricCapabilityProfileRequest")
	for _, route := range []string{"/api/v1/metric-capability-profiles/{id}/publish", "/api/v1/metric-capability-profiles/{id}/archive"} {
		expectBody(route, "post", "#/definitions/http.MetricCapabilityLifecycleRequest")
	}
}

func assertSafeDeliveryOpenAPIDefinitions(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	response, ok := definitions["http.SubscriptionResponse"]
	if !ok {
		t.Fatal("missing http.SubscriptionResponse")
	}
	allowed := map[string]bool{"id": true, "version": true, "monitor_id": true, "report_type": true, "channel": true, "recipient": true, "timezone": true, "schedule": true, "enabled": true}
	for field := range response.Properties {
		if !allowed[field] {
			t.Errorf("http.SubscriptionResponse exposes %q", field)
		}
	}
	secret, ok := definitions["http.SubscriptionSecretResponse"]
	if !ok || secret.Properties["rss_token"] == nil || secret.Properties["subscription"] == nil {
		t.Fatal("subscription secret response must contain subscription and one-time rss_token")
	}
}

func assertFalseNegativeContentFeedbackOpenAPI(t *testing.T, paths map[string]json.RawMessage) {
	t.Helper()
	const route = "/api/v1/monitors/{id}/contents/{content_id}/feedback"
	var operations map[string]openAPIOperation
	if err := json.Unmarshal(paths[route], &operations); err != nil {
		t.Fatalf("decode false-negative content feedback path: %v", err)
	}
	operation, ok := operations["put"]
	if !ok {
		t.Fatalf("missing PUT %s", route)
	}
	for _, parameter := range operation.Parameters {
		if parameter.In == "body" && parameter.Schema.Ref == "#/definitions/http.RelevanceFalseNegativeFeedbackRequest" {
			return
		}
	}
	t.Fatalf("%s body must use the false-negative-only request schema", route)
}

func assertSafeRelevanceOpenAPIDefinitions(t *testing.T, definitions map[string]struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}) {
	t.Helper()
	assertAllowlist := func(name string, allowed map[string]bool) {
		t.Helper()
		definition, ok := definitions[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		for field := range definition.Properties {
			if !allowed[field] {
				t.Errorf("%s exposes %q", name, field)
			}
		}
		for field := range allowed {
			if _, ok := definition.Properties[field]; !ok {
				t.Errorf("%s misses %q", name, field)
			}
		}
	}
	assertAllowlist("http.RelevanceMatchResponse", map[string]bool{
		"id": true, "version": true, "content_id": true, "monitor_config_version_id": true, "scoring_version": true,
		"recall_paths": true, "reason_codes": true, "rule_score": true, "semantic_score": true, "llm_score": true,
		"final_score": true, "decision": true, "decision_origin": true, "degraded": true, "manual_locked": true,
		"explanation": true, "created_at": true,
	})
	assertAllowlist("http.RelevanceExplanationResponse", map[string]bool{
		"matched_terms": true, "matched_entities": true, "excluded_terms": true, "recall_paths": true, "scores": true, "reason_codes": true,
	})
	assertAllowlist("http.RelevanceContentResponse", map[string]bool{
		"id": true, "title": true, "canonical_url": true, "language": true, "published_at": true,
	})
	assertAllowlist("http.RelevanceFeedbackResponse", map[string]bool{
		"id": true, "version": true, "content_id": true, "match_id": true, "feedback_type": true, "updated_at": true,
	})
	assertAllowlist("http.RelevanceFalseNegativeFeedbackRequest", map[string]bool{
		"expected_feedback_version": true,
	})
	assertAllowlist("http.RelevancePreviewCandidateResponse", map[string]bool{
		"monitor_config_version_id": true, "scoring_version": true, "recall_paths": true, "reason_codes": true,
		"matched_terms": true, "matched_entities": true, "excluded_terms": true, "factors": true, "rule_score": true,
		"decision": true, "degraded": true, "hard_veto": true,
	})
	assertAllowlist("http.RelevanceSuggestionResponse", map[string]bool{
		"id": true, "version": true, "suggestion_type": true, "value": true, "support_count": true, "status": true, "created_at": true, "updated_at": true,
	})
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
		"relevance_score": true, "match_decision": true,
		"document_version_id": true,
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
	var providerProperty struct {
		Enum []string `json:"enum"`
	}
	if err := json.Unmarshal(create.Properties["provider"], &providerProperty); err != nil {
		t.Fatalf("decode model profile provider property: %v", err)
	}
	if !reflect.DeepEqual(providerProperty.Enum, []string{"openai", "deepseek", "ollama", "onnx"}) {
		t.Errorf("model profile provider enum = %#v", providerProperty.Enum)
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
	for _, field := range []struct {
		name    string
		minimum float64
		maximum float64
	}{
		{name: "collection_interval_seconds", minimum: 300, maximum: 86400},
		{name: "relevance_threshold", minimum: 60, maximum: 100},
		{name: "event_threshold", minimum: 0, maximum: 100},
		{name: "retention_days", minimum: 1, maximum: 3650},
	} {
		var constraint struct {
			Minimum *float64 `json:"minimum"`
			Maximum *float64 `json:"maximum"`
		}
		if err := json.Unmarshal(config.Properties[field.name], &constraint); err != nil {
			t.Fatalf("decode %s contract: %v", field.name, err)
		}
		if constraint.Minimum == nil || *constraint.Minimum != field.minimum || constraint.Maximum == nil || *constraint.Maximum != field.maximum {
			t.Errorf("%s range = %v..%v, want %v..%v", field.name, constraint.Minimum, constraint.Maximum, field.minimum, field.maximum)
		}
	}

	create, ok := definitions["http.CreateMonitorRequest"]
	if !ok {
		t.Error("missing http.CreateMonitorRequest")
	} else {
		for _, field := range []string{"name", "query", "source_connection_ids"} {
			if !contains(create.Required, field) {
				t.Errorf("http.CreateMonitorRequest must require %s", field)
			}
		}
		for _, redundant := range []string{"config", "rules", "sources", "description"} {
			if _, exists := create.Properties[redundant]; exists {
				t.Errorf("simple create request exposes redundant %q", redundant)
			}
		}
		var collection struct {
			MinItems *int `json:"minItems"`
			MaxItems *int `json:"maxItems"`
		}
		if err := json.Unmarshal(create.Properties["source_connection_ids"], &collection); err != nil {
			t.Errorf("decode simple create sources: %v", err)
		} else if collection.MinItems == nil || *collection.MinItems != 1 || collection.MaxItems == nil || *collection.MaxItems != 10 {
			t.Errorf("simple create source item range = %v..%v, want 1..10", collection.MinItems, collection.MaxItems)
		}
	}
	update, ok := definitions["http.UpdateMonitorRequest"]
	if !ok {
		t.Error("missing http.UpdateMonitorRequest")
	} else {
		for _, field := range []string{"expected_monitor_version", "name", "query", "source_connection_ids", "collection_interval_seconds"} {
			if !contains(update.Required, field) {
				t.Errorf("http.UpdateMonitorRequest must require %s", field)
			}
		}
		for _, redundant := range []string{"config", "rules", "sources", "description", "expected_draft_version"} {
			if _, exists := update.Properties[redundant]; exists {
				t.Errorf("simple update request exposes redundant %q", redundant)
			}
		}
	}

	for _, name := range []string{"http.ReplaceDraftRequest"} {
		definition, ok := definitions[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		for field, wantMax := range map[string]int{"rules": 100, "sources": 10} {
			var collection struct {
				MinItems *int `json:"minItems"`
				MaxItems *int `json:"maxItems"`
			}
			if err := json.Unmarshal(definition.Properties[field], &collection); err != nil {
				t.Errorf("decode %s %s collection contract: %v", name, field, err)
				continue
			}
			if collection.MinItems == nil || *collection.MinItems != 1 || collection.MaxItems == nil || *collection.MaxItems != wantMax {
				t.Errorf("%s %s item range = %v..%v, want 1..%d", name, field, collection.MinItems, collection.MaxItems, wantMax)
			}
		}
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
