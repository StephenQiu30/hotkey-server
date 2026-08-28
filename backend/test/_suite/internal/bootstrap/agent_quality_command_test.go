package bootstrap

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
)

type agentQualityProviderFake struct {
	version string
}

func (fake agentQualityProviderFake) Embed(context.Context, intelligencedomain.EmbeddingRequest) (intelligencedomain.EmbeddingResponse, error) {
	return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
}

func (fake agentQualityProviderFake) GenerateStructured(_ context.Context, request intelligencedomain.StructuredRequest) (intelligencedomain.StructuredResponse, error) {
	return intelligencedomain.StructuredResponse{ModelVersion: fake.version, JSON: agentQualityFixtureOutput(request.TaskType), Usage: intelligencedomain.Usage{InputTokens: 5, OutputTokens: 2}}, nil
}

func TestAgentQualityCommandEvaluatesFixedFiveSkillDatasetAsCandidate(t *testing.T) {
	datasetPath := filepath.Join("..", "..", "test", "fixtures", "agent-shadow", "v1", "golden-dataset.json")
	var captured agentQualityCommandOptions
	builder := func(_ context.Context, _ config.Config, options agentQualityCommandOptions) (intelligenceapplication.ShadowQualityTrack, intelligenceapplication.ShadowQualityTrack, error) {
		captured = options
		return intelligenceapplication.ShadowQualityTrack{
				Name: intelligenceapplication.ShadowQualityTrackBaseline, RuntimeName: options.BaselineRuntime,
				ModelName: options.BaselineModelName, ModelVersion: options.BaselineModelVersion,
				Provider: agentQualityProviderFake{version: options.BaselineModelVersion}, UsageAvailable: true,
			}, intelligenceapplication.ShadowQualityTrack{
				Name: intelligenceapplication.ShadowQualityTrackAgent, RuntimeName: "hotkey-agent",
				ModelName: options.AgentModelName, ModelVersion: options.AgentModelVersion,
				Provider: agentQualityProviderFake{version: options.AgentModelVersion}, RuntimeDegraded: true,
			}, nil
	}
	var output strings.Builder
	err := executeAgentQualityCommand(t.Context(), config.Default(), []string{
		"evaluate", "--dataset", datasetPath,
		"--baseline-runtime", "openai", "--baseline-model-name", "baseline-model", "--baseline-model-version", "baseline-v1",
		"--agent-model-name", "hotkey-agent", "--agent-model-version", "deterministic.v1", "--timeout", "2s",
	}, &output, builder)
	if err != nil {
		t.Fatalf("executeAgentQualityCommand() error = %v", err)
	}
	if captured.Timeout != 2*time.Second || captured.DatasetPath != datasetPath || captured.BaselineRuntime != "openai" {
		t.Fatalf("parsed options = %#v", captured)
	}
	var report intelligenceapplication.ShadowQualityReport
	if err := json.Unmarshal([]byte(output.String()), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Status != intelligenceapplication.ShadowQualityStatusCandidate || report.ApprovalReady || report.DatasetVersion != "agent-shadow-golden-v1" ||
		report.AnnotationProtocolVersion != "pending-dual-review-v1" || report.AnnotatorCount != 0 || len(report.Tracks) != 2 || len(report.Samples) != 10 ||
		report.Tracks[0].Track != intelligenceapplication.ShadowQualityTrackBaseline || report.Tracks[0].SampleCount != 5 ||
		report.Tracks[1].Track != intelligenceapplication.ShadowQualityTrackAgent || report.Tracks[1].SampleCount != 5 {
		t.Fatalf("quality report = %#v", report)
	}
	for _, reason := range []string{"quality_thresholds_not_approved", "agent_runtime_degraded", "human_review_incomplete", "cost_unavailable"} {
		if !containsAgentQualityReason(report.ReasonCodes, reason) {
			t.Fatalf("candidate reasons = %#v, missing %q", report.ReasonCodes, reason)
		}
	}
	for _, forbidden := range []string{"Synthetic launch evidence", "Track the synthetic launch"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("command report leaked Golden input %q", forbidden)
		}
	}
}

func TestAgentQualityDatasetRejectsUnknownFieldsAndTrailingDocuments(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "test", "fixtures", "agent-shadow", "v1", "golden-dataset.json")
	encoded, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutated := range []string{
		strings.Replace(string(encoded), `"sample_id": "monitor-compile-001",`, `"sample_id": "monitor-compile-001", "prompt": "forbidden",`, 1),
		string(encoded) + `{}`,
	} {
		path := filepath.Join(t.TempDir(), "dataset.json")
		if err := os.WriteFile(path, []byte(mutated), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readAgentQualityDataset(path); err == nil {
			t.Fatal("invalid Agent quality dataset was accepted")
		}
	}
}

func TestAgentQualityCommandRequiresExplicitTrustedRuntimeInputs(t *testing.T) {
	builderCalled := false
	builder := func(context.Context, config.Config, agentQualityCommandOptions) (intelligenceapplication.ShadowQualityTrack, intelligenceapplication.ShadowQualityTrack, error) {
		builderCalled = true
		return intelligenceapplication.ShadowQualityTrack{}, intelligenceapplication.ShadowQualityTrack{}, nil
	}
	for _, arguments := range [][]string{
		nil,
		{"preview"},
		{"evaluate"},
		{"evaluate", "--dataset", "fixture.json", "--baseline-runtime", "openai"},
		{"evaluate", "--dataset", "fixture.json", "--baseline-runtime", "unknown", "--baseline-model-name", "x", "--baseline-model-version", "v1", "--agent-model-name", "agent", "--agent-model-version", "v1"},
		{"evaluate", "--dataset", "fixture.json", "--baseline-runtime", "codex", "--baseline-model-name", "x", "--baseline-model-version", "v1", "--agent-model-name", "agent", "--agent-model-version", "v1"},
	} {
		if err := executeAgentQualityCommand(t.Context(), config.Default(), arguments, &strings.Builder{}, builder); err == nil {
			t.Fatalf("arguments %#v were accepted", arguments)
		}
	}
	if builderCalled {
		t.Fatal("provider builder ran before command validation")
	}
}

func TestAgentQualityCommandRequiresPairedFinitePricingForBothTracks(t *testing.T) {
	valid := agentQualityCommandOptions{
		DatasetPath: "fixture.json", BaselineRuntime: "openai", BaselineModelName: "baseline",
		BaselineModelVersion: "baseline-v1", AgentModelName: "agent", AgentModelVersion: "agent-v1",
		Timeout: time.Minute, BaselineInputUSDPerMillionTokens: 1, BaselineOutputUSDPerMillionTokens: 2,
		AgentInputUSDPerMillionTokens: 3, AgentOutputUSDPerMillionTokens: 4,
	}
	if err := validateAgentQualityCommandOptions(valid, 0, &strings.Builder{}, func(context.Context, config.Config, agentQualityCommandOptions) (intelligenceapplication.ShadowQualityTrack, intelligenceapplication.ShadowQualityTrack, error) {
		return intelligenceapplication.ShadowQualityTrack{}, intelligenceapplication.ShadowQualityTrack{}, nil
	}); err != nil {
		t.Fatalf("valid paired pricing rejected: %v", err)
	}
	for _, mutate := range []func(*agentQualityCommandOptions){
		func(value *agentQualityCommandOptions) { value.AgentOutputUSDPerMillionTokens = -1 },
		func(value *agentQualityCommandOptions) { value.AgentInputUSDPerMillionTokens = math.NaN() },
		func(value *agentQualityCommandOptions) {
			value.AgentModelVersion = "deterministic.v1"
		},
	} {
		options := valid
		mutate(&options)
		if err := validateAgentQualityCommandOptions(options, 0, &strings.Builder{}, func(context.Context, config.Config, agentQualityCommandOptions) (intelligenceapplication.ShadowQualityTrack, intelligenceapplication.ShadowQualityTrack, error) {
			return intelligenceapplication.ShadowQualityTrack{}, intelligenceapplication.ShadowQualityTrack{}, nil
		}); err == nil {
			t.Fatalf("invalid Agent pricing was accepted: %#v", options)
		}
	}
}

func agentQualityFixtureOutput(taskType intelligencedomain.TaskType) json.RawMessage {
	switch taskType {
	case intelligencedomain.TaskTypeTermExpansion:
		return json.RawMessage(`{"terms":[]}`)
	case intelligencedomain.TaskTypeRelevanceReview:
		return json.RawMessage(`{"decision":"review","score":0,"reason_codes":["insufficient_evidence"]}`)
	case intelligencedomain.TaskTypeEventCluster:
		return json.RawMessage(`{"action":"create","confidence":0,"reason_codes":["no_candidate"]}`)
	case intelligencedomain.TaskTypeEventSummary:
		return json.RawMessage(`{"title_zh":"待分析","sentences":[]}`)
	case intelligencedomain.TaskTypeEntityClaimExtraction:
		return json.RawMessage(`{"claims":[{"subject":"pending","predicate":"pending","object":"pending","relation":"unknown","exact_quote":"Synthetic launch evidence.","relation_score":0,"qualifiers":[]}]}`)
	default:
		return json.RawMessage(`{}`)
	}
}

func containsAgentQualityReason(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

var _ intelligencedomain.Provider = agentQualityProviderFake{}
