package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	intelligenceagent "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/infrastructure/agent"
	intelligenceprovider "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/infrastructure/provider"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"go.uber.org/zap"
)

const maximumAgentQualityDatasetBytes = 8 << 20

type agentQualityCommandOptions struct {
	DatasetPath                       string
	ReviewBundlePath                  string
	BaselineRuntime                   string
	BaselineModelName                 string
	BaselineModelVersion              string
	AgentModelName                    string
	AgentModelVersion                 string
	CodexExecutable                   string
	Timeout                           time.Duration
	BaselineInputUSDPerMillionTokens  float64
	BaselineOutputUSDPerMillionTokens float64
	AgentInputUSDPerMillionTokens     float64
	AgentOutputUSDPerMillionTokens    float64
}

type agentQualityTrackBuilder func(context.Context, config.Config, agentQualityCommandOptions) (intelligenceapplication.ShadowQualityTrack, intelligenceapplication.ShadowQualityTrack, error)

type agentQualityDatasetRequest struct {
	DatasetVersion            string                      `json:"dataset_version"`
	AnnotationProtocolVersion string                      `json:"annotation_protocol_version"`
	AnnotatorCount            int                         `json:"annotator_count"`
	Samples                   []agentQualitySampleRequest `json:"samples"`
}

type agentQualitySampleRequest struct {
	SampleID         string                             `json:"sample_id"`
	TaskType         intelligencedomain.TaskType        `json:"task_type"`
	SchemaVersion    string                             `json:"schema_version"`
	Input            json.RawMessage                    `json:"input"`
	ExpectedEvidence []agentQualityEvidenceLabelRequest `json:"expected_evidence"`
	HumanReviews     []agentQualityHumanReviewRequest   `json:"human_reviews"`
}

type agentQualityEvidenceLabelRequest struct {
	ContentID  int64  `json:"content_id,omitempty"`
	Locator    string `json:"locator,omitempty"`
	ExactQuote string `json:"exact_quote,omitempty"`
}

type agentQualityHumanReviewRequest struct {
	Track         string `json:"track"`
	OutputSHA256  string `json:"output_sha256"`
	Accepted      bool   `json:"accepted"`
	ReviewerCount int    `json:"reviewer_count"`
}

type agentQualityReviewBundle struct {
	DatasetVersion            string                        `json:"dataset_version"`
	DatasetSHA256             string                        `json:"dataset_sha256"`
	AnnotationProtocolVersion string                        `json:"annotation_protocol_version"`
	Status                    string                        `json:"status"`
	ReviewerCountRequired     int                           `json:"reviewer_count_required"`
	Candidates                []agentQualityReviewCandidate `json:"candidates"`
}

type agentQualityReviewCandidate struct {
	SampleID      string                      `json:"sample_id"`
	Track         string                      `json:"track"`
	TaskType      intelligencedomain.TaskType `json:"task_type"`
	SchemaVersion string                      `json:"schema_version"`
	RuntimeName   string                      `json:"runtime_name"`
	ModelName     string                      `json:"model_name"`
	ModelVersion  string                      `json:"model_version"`
	OutputSHA256  string                      `json:"output_sha256"`
	InputRawJSON  string                      `json:"input_raw_json"`
	OutputRawJSON string                      `json:"output_raw_json"`
}

func runAgentQualityCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer) error {
	return executeAgentQualityCommand(ctx, cfg, args, output, buildAgentQualityTracks)
}

func executeAgentQualityCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer, builder agentQualityTrackBuilder) error {
	if len(args) == 0 || args[0] != "evaluate" {
		return errors.New("agent-quality command is required: expected evaluate")
	}
	flags := flag.NewFlagSet("hotkey agent-quality evaluate", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	options := agentQualityCommandOptions{}
	flags.StringVar(&options.DatasetPath, "dataset", "", "fixed versioned Golden dataset JSON")
	flags.StringVar(&options.ReviewBundlePath, "review-bundle", "", "optional absolute path for private human-review material")
	flags.StringVar(&options.BaselineRuntime, "baseline-runtime", "", "openai, deepseek, ollama, or codex")
	flags.StringVar(&options.BaselineModelName, "baseline-model-name", "", "baseline provider model name")
	flags.StringVar(&options.BaselineModelVersion, "baseline-model-version", "", "baseline provider model version")
	flags.StringVar(&options.AgentModelName, "agent-model-name", "", "Python Agent model name")
	flags.StringVar(&options.AgentModelVersion, "agent-model-version", "", "Python Agent runtime version")
	flags.StringVar(&options.CodexExecutable, "codex-executable", "", "absolute trusted Codex executable path")
	flags.DurationVar(&options.Timeout, "timeout", time.Minute, "per-track sample timeout")
	flags.Float64Var(&options.BaselineInputUSDPerMillionTokens, "baseline-input-usd-per-million", -1, "optional baseline input-token price")
	flags.Float64Var(&options.BaselineOutputUSDPerMillionTokens, "baseline-output-usd-per-million", -1, "optional baseline output-token price")
	flags.Float64Var(&options.AgentInputUSDPerMillionTokens, "agent-input-usd-per-million", -1, "optional Agent input-token price")
	flags.Float64Var(&options.AgentOutputUSDPerMillionTokens, "agent-output-usd-per-million", -1, "optional Agent output-token price")
	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("parse agent-quality evaluate flags: %w", err)
	}
	options.DatasetPath = strings.TrimSpace(options.DatasetPath)
	options.ReviewBundlePath = strings.TrimSpace(options.ReviewBundlePath)
	options.BaselineRuntime = strings.TrimSpace(options.BaselineRuntime)
	options.BaselineModelName = strings.TrimSpace(options.BaselineModelName)
	options.BaselineModelVersion = strings.TrimSpace(options.BaselineModelVersion)
	options.AgentModelName = strings.TrimSpace(options.AgentModelName)
	options.AgentModelVersion = strings.TrimSpace(options.AgentModelVersion)
	options.CodexExecutable = strings.TrimSpace(options.CodexExecutable)
	if err := validateAgentQualityCommandOptions(options, flags.NArg(), output, builder); err != nil {
		return err
	}
	dataset, err := readAgentQualityDataset(options.DatasetPath)
	if err != nil {
		return err
	}
	if err := preflightAgentQualityReviewBundle(options.ReviewBundlePath); err != nil {
		return err
	}
	baseline, agent, err := builder(ctx, cfg, options)
	if err != nil {
		return err
	}
	schemas, err := intelligenceapplication.NewSchemaRegistry()
	if err != nil {
		return errors.New("initialize Agent quality schema registry")
	}
	evaluator, err := intelligenceapplication.NewShadowQualityEvaluator(schemas, intelligenceapplication.ShadowQualityEvaluatorOptions{Timeout: options.Timeout})
	if err != nil {
		return err
	}
	var report intelligenceapplication.ShadowQualityReport
	if options.ReviewBundlePath == "" {
		report, err = evaluator.Evaluate(ctx, dataset, baseline, agent)
	} else {
		var candidates []intelligenceapplication.ShadowQualityReviewCandidate
		report, candidates, err = evaluator.EvaluateForReview(ctx, dataset, baseline, agent)
		if err == nil {
			err = writeAgentQualityReviewBundle(options.ReviewBundlePath, report, candidates)
		}
	}
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(true)
	return encoder.Encode(report)
}

func validateAgentQualityCommandOptions(options agentQualityCommandOptions, positionalArguments int, output io.Writer, builder agentQualityTrackBuilder) error {
	if positionalArguments != 0 || output == nil || builder == nil || options.DatasetPath == "" ||
		options.BaselineModelName == "" || options.BaselineModelVersion == "" || options.AgentModelName == "" || options.AgentModelVersion == "" ||
		options.Timeout <= 0 || options.Timeout > 5*time.Minute || !validAgentQualityRuntime(options.BaselineRuntime) {
		return errors.New("agent-quality evaluate requires a dataset, approved runtimes, model identities, and timeout up to 5m")
	}
	if options.BaselineRuntime == "codex" {
		if options.CodexExecutable == "" || !filepath.IsAbs(options.CodexExecutable) {
			return errors.New("agent-quality Codex baseline requires an absolute --codex-executable")
		}
	} else if options.CodexExecutable != "" {
		return errors.New("--codex-executable is valid only for the Codex baseline")
	}
	if options.ReviewBundlePath != "" && !filepath.IsAbs(options.ReviewBundlePath) {
		return errors.New("agent-quality review bundle requires an absolute path")
	}
	if !validAgentQualityPricing(options.BaselineInputUSDPerMillionTokens, options.BaselineOutputUSDPerMillionTokens) {
		return errors.New("baseline pricing must provide two finite non-negative values together")
	}
	if !validAgentQualityPricing(options.AgentInputUSDPerMillionTokens, options.AgentOutputUSDPerMillionTokens) ||
		options.AgentModelVersion == intelligenceagent.DeterministicRuntimeVersion && options.AgentInputUSDPerMillionTokens >= 0 {
		return errors.New("Agent pricing requires a non-degraded runtime and two finite non-negative values together")
	}
	return nil
}

func preflightAgentQualityReviewBundle(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Lstat(path); err == nil {
		return errors.New("Agent quality review bundle already exists")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect Agent quality review bundle: %w", err)
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil || !parent.IsDir() {
		return errors.New("Agent quality review bundle parent directory is unavailable")
	}
	return nil
}

func writeAgentQualityReviewBundle(path string, report intelligenceapplication.ShadowQualityReport, candidates []intelligenceapplication.ShadowQualityReviewCandidate) error {
	bundle := agentQualityReviewBundle{
		DatasetVersion: report.DatasetVersion, DatasetSHA256: report.DatasetSHA256,
		AnnotationProtocolVersion: report.AnnotationProtocolVersion,
		Status:                    "pending_independent_review", ReviewerCountRequired: 2,
		Candidates: make([]agentQualityReviewCandidate, len(candidates)),
	}
	for index, candidate := range candidates {
		bundle.Candidates[index] = agentQualityReviewCandidate{
			SampleID: candidate.SampleID, Track: candidate.Track, TaskType: candidate.TaskType, SchemaVersion: candidate.SchemaVersion,
			RuntimeName: candidate.RuntimeName, ModelName: candidate.ModelName, ModelVersion: candidate.ModelVersion,
			OutputSHA256: candidate.OutputSHA256, InputRawJSON: string(candidate.Input), OutputRawJSON: string(candidate.Output),
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return errors.New("Agent quality review bundle already exists")
		}
		return fmt.Errorf("create Agent quality review bundle: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(true)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(bundle); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return errors.New("encode Agent quality review bundle")
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return errors.New("close Agent quality review bundle")
	}
	return nil
}

func validAgentQualityPricing(input, output float64) bool {
	if input == -1 && output == -1 {
		return true
	}
	return input >= 0 && output >= 0 && input <= 1_000_000 && output <= 1_000_000 &&
		!math.IsNaN(input) && !math.IsInf(input, 0) && !math.IsNaN(output) && !math.IsInf(output, 0)
}

func validAgentQualityRuntime(runtime string) bool {
	return runtime == string(intelligencedomain.ProviderOpenAI) || runtime == string(intelligencedomain.ProviderDeepSeek) ||
		runtime == string(intelligencedomain.ProviderOllama) || runtime == "codex"
}

func readAgentQualityDataset(path string) (intelligenceapplication.ShadowQualityDataset, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return intelligenceapplication.ShadowQualityDataset{}, fmt.Errorf("open Agent quality dataset: %w", err)
	}
	if len(encoded) > maximumAgentQualityDatasetBytes {
		return intelligenceapplication.ShadowQualityDataset{}, errors.New("Agent quality dataset exceeds 8 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	var request agentQualityDatasetRequest
	if err := decoder.Decode(&request); err != nil {
		return intelligenceapplication.ShadowQualityDataset{}, fmt.Errorf("decode Agent quality dataset: %w", err)
	}
	if err := ensureRelevanceDatasetEOF(decoder); err != nil {
		return intelligenceapplication.ShadowQualityDataset{}, err
	}
	digest := sha256.Sum256(encoded)
	dataset := intelligenceapplication.ShadowQualityDataset{
		DatasetVersion: request.DatasetVersion, DatasetSHA256: hex.EncodeToString(digest[:]),
		AnnotationProtocolVersion: request.AnnotationProtocolVersion, AnnotatorCount: request.AnnotatorCount,
		Samples: make([]intelligenceapplication.ShadowQualitySample, len(request.Samples)),
	}
	for sampleIndex, sample := range request.Samples {
		mapped := intelligenceapplication.ShadowQualitySample{
			SampleID: sample.SampleID, TaskType: sample.TaskType, SchemaVersion: sample.SchemaVersion,
			Input:            append(json.RawMessage(nil), sample.Input...),
			ExpectedEvidence: make([]intelligenceapplication.ShadowQualityEvidenceLabel, len(sample.ExpectedEvidence)),
			HumanReviews:     make([]intelligenceapplication.ShadowQualityHumanReview, len(sample.HumanReviews)),
		}
		for evidenceIndex, evidence := range sample.ExpectedEvidence {
			mapped.ExpectedEvidence[evidenceIndex] = intelligenceapplication.ShadowQualityEvidenceLabel{
				ContentID: evidence.ContentID, Locator: evidence.Locator, ExactQuote: evidence.ExactQuote,
			}
		}
		for reviewIndex, review := range sample.HumanReviews {
			mapped.HumanReviews[reviewIndex] = intelligenceapplication.ShadowQualityHumanReview{
				Track: review.Track, OutputSHA256: review.OutputSHA256, Accepted: review.Accepted, ReviewerCount: review.ReviewerCount,
			}
		}
		dataset.Samples[sampleIndex] = mapped
	}
	return dataset, nil
}

func buildAgentQualityTracks(_ context.Context, cfg config.Config, options agentQualityCommandOptions) (intelligenceapplication.ShadowQualityTrack, intelligenceapplication.ShadowQualityTrack, error) {
	var baselineProvider intelligencedomain.Provider
	if options.BaselineRuntime == "codex" {
		adapter, err := intelligenceprovider.NewCodexCLIAdapterWithOptions(intelligenceprovider.CodexCLIAdapterOptions{
			Executable: options.CodexExecutable, WorkspaceRoot: os.TempDir(), Timeout: options.Timeout,
			AuthFile: cfg.AI.CodexAuthFile, MaxOutputBytes: 1 << 20, MaxConcurrent: 1,
		})
		if err != nil {
			return intelligenceapplication.ShadowQualityTrack{}, intelligenceapplication.ShadowQualityTrack{}, errors.New("configure trusted Codex baseline")
		}
		baselineProvider, err = intelligenceprovider.NewCodexCLIProvider(adapter)
		if err != nil {
			return intelligenceapplication.ShadowQualityTrack{}, intelligenceapplication.ShadowQualityTrack{}, errors.New("configure trusted Codex baseline")
		}
	} else {
		registry := newAIProviderRegistry(cfg, zap.NewNop())
		var found bool
		baselineProvider, found = registry.Resolve(intelligencedomain.ProviderName(options.BaselineRuntime))
		if !found {
			return intelligenceapplication.ShadowQualityTrack{}, intelligenceapplication.ShadowQualityTrack{}, errors.New("configured baseline provider is unavailable")
		}
	}
	agentProvider, err := intelligenceagent.NewClient(intelligenceagent.Options{
		BaseURL: cfg.Agent.URL, AuthToken: cfg.Agent.AuthToken, MaxResponseBytes: cfg.Agent.MaxResponseBytes,
	})
	if err != nil {
		return intelligenceapplication.ShadowQualityTrack{}, intelligenceapplication.ShadowQualityTrack{}, errors.New("configured Python Agent is unavailable")
	}
	baseline := intelligenceapplication.ShadowQualityTrack{
		Name: intelligenceapplication.ShadowQualityTrackBaseline, RuntimeName: options.BaselineRuntime,
		ModelName: options.BaselineModelName, ModelVersion: options.BaselineModelVersion,
		Provider: baselineProvider, UsageAvailable: true,
	}
	if options.BaselineInputUSDPerMillionTokens >= 0 {
		baseline.Pricing = &intelligenceapplication.ShadowQualityPricing{
			InputUSDPerMillion: options.BaselineInputUSDPerMillionTokens, OutputUSDPerMillion: options.BaselineOutputUSDPerMillionTokens,
		}
	}
	agent := intelligenceapplication.ShadowQualityTrack{
		Name: intelligenceapplication.ShadowQualityTrackAgent, RuntimeName: "hotkey-agent",
		ModelName: options.AgentModelName, ModelVersion: options.AgentModelVersion,
		Provider: agentProvider, RuntimeDegraded: options.AgentModelVersion == intelligenceagent.DeterministicRuntimeVersion,
		UsageAvailable: options.AgentModelVersion != intelligenceagent.DeterministicRuntimeVersion,
	}
	if options.AgentInputUSDPerMillionTokens >= 0 {
		agent.Pricing = &intelligenceapplication.ShadowQualityPricing{
			InputUSDPerMillion:  options.AgentInputUSDPerMillionTokens,
			OutputUSDPerMillion: options.AgentOutputUSDPerMillionTokens,
		}
	}
	return baseline, agent, nil
}
