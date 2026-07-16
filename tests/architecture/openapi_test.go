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
		"/api/v1/capabilities":                     {"get": {"200"}},
		"/api/v1/auth/email-verifications":         {"post": {"200", "400", "429", "503"}},
		"/api/v1/auth/email-verifications/confirm": {"post": {"200", "400", "429", "503"}},
		"/api/v1/auth/registrations":               {"post": {"201", "400", "409", "503"}},
		"/api/v1/auth/login":                       {"post": {"200", "400", "401", "503"}},
		"/api/v1/auth/refresh":                     {"post": {"200", "401", "403", "503"}},
		"/api/v1/auth/logout":                      {"post": {"200", "403", "503"}},
		"/api/v1/auth/me":                          {"get": {"200", "401"}},
		"/api/v1/auth/password":                    {"post": {"200", "400", "401", "503"}},
		"/api/v1/auth/password-resets/confirm":     {"post": {"200", "400", "503"}},
		"/api/v1/users":                            {"get": {"200", "401", "403", "503"}},
		"/api/v1/users/{id}":                       {"patch": {"200", "400", "401", "403", "409", "503"}, "delete": {"200", "401", "403", "409", "503"}},
		"/api/v1/users/{id}/restore":               {"post": {"200", "401", "403", "409", "503"}},
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

	for _, route := range []string{"/api/v1/auth/me", "/api/v1/auth/password", "/api/v1/users", "/api/v1/users/{id}", "/api/v1/users/{id}/restore"} {
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
