package architecture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAPIContract(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "openapi", "swagger.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var document struct {
		Swagger string                     `json:"swagger"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	if document.Swagger != "2.0" {
		t.Errorf("swagger version = %q, want 2.0", document.Swagger)
	}
	if len(document.Paths) != 1 {
		t.Fatalf("public path count = %d, want 1 (%v)", len(document.Paths), document.Paths)
	}
	capabilities, ok := document.Paths["/api/v1/capabilities"]
	if !ok {
		t.Fatal("missing /api/v1/capabilities")
	}
	var operation struct {
		Get struct {
			Responses map[string]struct {
				Schema struct {
					Ref string `json:"$ref"`
				} `json:"schema"`
			} `json:"responses"`
		} `json:"get"`
	}
	if err := json.Unmarshal(capabilities, &operation); err != nil {
		t.Fatalf("decode capabilities operation: %v", err)
	}
	response, ok := operation.Get.Responses["200"]
	if !ok || response.Schema.Ref == "" {
		t.Fatalf("capabilities 200 response lacks a concrete Result schema: %#v", operation.Get.Responses)
	}
	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		if _, exists := document.Paths[path]; exists {
			t.Errorf("operational path %s must not be in OpenAPI", path)
		}
	}
}
