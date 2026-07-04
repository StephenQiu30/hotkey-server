package http_test

import (
	"encoding/json"
	"testing"

	swaggerdocs "github.com/StephenQiu30/hotkey-server/docs"
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

type swaggerSpec struct {
	Swagger string                    `json:"swagger"`
	Paths   map[string]map[string]any `json:"paths"`
}

func TestSwaggerCoverage(t *testing.T) {
	spec := loadSwaggerSpec(t)
	gotOps := collectOperationIDs(spec.Paths)

	for _, want := range expectedOperations {
		if !gotOps[want] {
			t.Errorf("missing operationId %q in generated swagger doc", want)
		}
	}

	for _, want := range expectedAPIv1Paths {
		if _, ok := spec.Paths[want]; !ok {
			t.Errorf("missing path %q in generated swagger doc", want)
		}
	}
}

func TestSwaggerVersion(t *testing.T) {
	spec := loadSwaggerSpec(t)
	if spec.Swagger != "2.0" {
		t.Fatalf("expected swagger version 2.0, got %q", spec.Swagger)
	}
}

func TestSwaggerSecurityDefinition(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(swaggerdocs.SwaggerInfo.ReadDoc()), &raw); err != nil {
		t.Fatalf("generated swagger doc is not valid JSON: %v", err)
	}

	securityDefinitions, _ := raw["securityDefinitions"].(map[string]any)
	if _, ok := securityDefinitions["BearerAuth"]; !ok {
		t.Fatal("missing BearerAuth security definition in generated swagger doc")
	}
}

func TestSwaggerPathCount(t *testing.T) {
	spec := loadSwaggerSpec(t)
	minPaths := len(expectedAPIv1Paths) + 1
	if len(spec.Paths) < minPaths {
		t.Fatalf("expected at least %d paths, got %d", minPaths, len(spec.Paths))
	}
}

func loadSwaggerSpec(t *testing.T) swaggerSpec {
	t.Helper()

	var spec swaggerSpec
	if err := json.Unmarshal([]byte(swaggerdocs.SwaggerInfo.ReadDoc()), &spec); err != nil {
		t.Fatalf("generated swagger doc is not valid JSON: %v", err)
	}
	return spec
}

func collectOperationIDs(paths map[string]map[string]any) map[string]bool {
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
