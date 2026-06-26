package http_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
)

var expectedOperations = []string{
	"register",
	"login",
	"list-monitors",
	"create-monitor",
	"get-monitor",
	"update-monitor",
	"list-posts",
	"list-topics",
	"get-monitor-trends",
	"get-topic-trends",
	"list-notifications",
	"mark-notification-read",
	"health-check",
}

var expectedAPIv1Paths = []string{
	"/api/v1/auth/login",
	"/api/v1/auth/register",
	"/api/v1/monitors",
	"/api/v1/monitors/{id}",
	"/api/v1/monitors/{id}/posts",
	"/api/v1/monitors/{id}/topics",
	"/api/v1/monitors/{id}/trends",
	"/api/v1/notifications",
	"/api/v1/notifications/{id}/read",
	"/api/v1/topics/{id}/trends",
}

type openapiSpec struct {
	OpenAPI string                    `json:"openapi"`
	Info    map[string]interface{}    `json:"info"`
	Paths   map[string]map[string]any `json:"paths"`
}

func TestOpenAPICoverage(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "docs", "openapi.json")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("cannot read docs/openapi.json: %v (run `make openapi` first)", err)
	}

	var spec openapiSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("docs/openapi.json is not valid JSON: %v", err)
	}

	gotOps := collectOperationIDsFromPathItems(spec.Paths)

	for _, want := range expectedOperations {
		if !gotOps[want] {
			t.Errorf("missing operationId %q in docs/openapi.json", want)
		}
	}

	for _, want := range expectedAPIv1Paths {
		if _, ok := spec.Paths[want]; !ok {
			t.Errorf("missing path %q in docs/openapi.json", want)
		}
	}
}

func TestOpenAPIVersion(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "docs", "openapi.json")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("cannot read docs/openapi.json: %v", err)
	}

	var spec openapiSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("docs/openapi.json is not valid JSON: %v", err)
	}

	if spec.OpenAPI != "3.1.0" {
		t.Errorf("expected openapi version 3.1.0, got %q", spec.OpenAPI)
	}
}

func TestOpenAPISecurityScheme(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "docs", "openapi.json")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("cannot read docs/openapi.json: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("docs/openapi.json is not valid JSON: %v", err)
	}

	components, _ := raw["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	if _, ok := schemes["bearer"]; !ok {
		t.Error("missing securityScheme 'bearer' in docs/openapi.json")
	}
}

func TestOpenAPIErrorBodyIncludesRequestID(t *testing.T) {
	spec := platformhttp.BuildOpenAPISpec()
	components, _ := spec["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	errorBody, _ := schemas["ErrorBody"].(map[string]any)
	properties, _ := errorBody["properties"].(map[string]any)

	if _, ok := properties["request_id"]; !ok {
		t.Fatal("static ErrorBody schema missing request_id")
	}
}

func TestOpenAPIPathCount(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "docs", "openapi.json")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("cannot read docs/openapi.json: %v", err)
	}

	var spec openapiSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("docs/openapi.json is not valid JSON: %v", err)
	}

	minPaths := len(expectedAPIv1Paths) + 1
	if len(spec.Paths) < minPaths {
		t.Errorf("expected at least %d paths, got %d", minPaths, len(spec.Paths))
	}
}

func TestOpenAPIFromStaticSpec(t *testing.T) {
	spec := platformhttp.BuildOpenAPISpec()
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("expected paths in static OpenAPI spec")
	}

	gotOps := collectOperationIDs(paths)
	for _, want := range expectedOperations {
		if !gotOps[want] {
			t.Errorf("static spec missing operationId %q", want)
		}
	}
}

func collectOperationIDsFromPathItems(paths map[string]map[string]any) map[string]bool {
	ops := make(map[string]bool)
	httpMethods := map[string]bool{
		"get": true, "post": true, "put": true,
		"patch": true, "delete": true, "head": true, "options": true,
	}
	for _, methods := range paths {
		for method, val := range methods {
			if !httpMethods[method] {
				continue
			}
			opMap, ok := val.(map[string]any)
			if !ok {
				continue
			}
			if id, ok := opMap["operationId"].(string); ok {
				ops[id] = true
			}
		}
	}
	return ops
}

func collectOperationIDs(paths map[string]any) map[string]bool {
	ops := make(map[string]bool)
	httpMethods := map[string]bool{
		"get": true, "post": true, "put": true,
		"patch": true, "delete": true, "head": true, "options": true,
	}
	for _, methodsVal := range paths {
		methods, ok := methodsVal.(map[string]any)
		if !ok {
			continue
		}
		for method, val := range methods {
			if !httpMethods[method] {
				continue
			}
			opMap, ok := val.(map[string]any)
			if !ok {
				continue
			}
			if id, ok := opMap["operationId"].(string); ok {
				ops[id] = true
			}
		}
	}
	return ops
}
