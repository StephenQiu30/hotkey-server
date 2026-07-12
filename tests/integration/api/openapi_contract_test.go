package api_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenAPIResponseContract(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "swagger.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skip("swagger.json not found — run 'make openapi' first")
	}
	var doc struct {
		Definitions map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for name, schema := range doc.Definitions {
		if name != "internal_controller.LoginResponse" && name != "github_com_StephenQiu30_hotkey-server_internal_platform_http.ErrorBody" {
			continue
		}
		for _, key := range []string{"code", "message", "data"} {
			if _, ok := schema.Properties[key]; !ok {
				t.Fatalf("%s missing %s", name, key)
			}
		}
		for _, forbidden := range []string{"error_code", "request_id"} {
			if _, ok := schema.Properties[forbidden]; ok {
				t.Fatalf("%s exposes %s", name, forbidden)
			}
		}
	}
}
