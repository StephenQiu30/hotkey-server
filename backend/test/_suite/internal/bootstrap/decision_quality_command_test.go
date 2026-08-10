package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
)

func TestDecisionQualityCommandEvaluatesVersionedTimeIsolatedFixture(t *testing.T) {
	datasetPath := filepath.Join("..", "..", "test", "fixtures", "quality", "time-isolated", "acceptance-dataset.json")
	var output strings.Builder
	if err := runDecisionQualityCommand(t.Context(), config.Config{}, []string{"evaluate", "--dataset", datasetPath}, &output); err != nil {
		t.Fatalf("runDecisionQualityCommand() error = %v", err)
	}
	var result struct {
		Evaluation struct {
			DatasetVersion         string `json:"dataset_version"`
			DatasetSHA256          string `json:"dataset_sha256"`
			AllRequiredGatesPassed bool   `json:"all_required_gates_passed"`
			Metrics                []struct {
				Module                   string `json:"module"`
				ProfileVersion           string `json:"profile_version"`
				Passed                   bool   `json:"passed"`
				AutomaticDecisionAllowed bool   `json:"automatic_decision_allowed"`
			} `json:"metrics"`
		} `json:"evaluation"`
	}
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatalf("decode quality command output: %v", err)
	}
	if result.Evaluation.DatasetVersion != "decision-quality-time-isolated-v1" || len(result.Evaluation.DatasetSHA256) != 64 ||
		!result.Evaluation.AllRequiredGatesPassed || len(result.Evaluation.Metrics) != 5 {
		t.Fatalf("quality command result = %#v", result)
	}
	for _, metric := range result.Evaluation.Metrics {
		if metric.Module == "" || metric.ProfileVersion == "" || !metric.Passed || !metric.AutomaticDecisionAllowed {
			t.Fatalf("quality metric = %#v", metric)
		}
	}
}

func TestDecisionQualityDatasetRejectsUnknownNestedFields(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "test", "fixtures", "quality", "time-isolated", "acceptance-dataset.json")
	encoded, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(encoded), `"sample_id": "duplicate-000",`, `"sample_id": "duplicate-000", "body": "forbidden",`, 1)
	path := filepath.Join(t.TempDir(), "unknown-field.json")
	if err := os.WriteFile(path, []byte(mutated), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readDecisionQualityDataset(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("nested unknown field error = %v", err)
	}
}

func TestDecisionQualityCommandRequiresSemanticSubcommandAndDataset(t *testing.T) {
	for _, arguments := range [][]string{nil, {"preview"}, {"evaluate"}, {"evaluate", "--dataset", ""}, {"evaluate", "--dataset", "fixture.json", "--activate"}} {
		if err := runDecisionQualityCommand(t.Context(), config.Config{}, arguments, &strings.Builder{}); err == nil {
			t.Fatalf("arguments %#v were accepted", arguments)
		}
	}
}
