package bootstrap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestCommittedRelevanceAcceptanceDatasetPassesQualityGates(t *testing.T) {
	datasetPath := filepath.Join("..", "..", "test", "fixtures", "relevance", "time-isolated", "acceptance-dataset.json")
	guidePath := filepath.Join("..", "..", "test", "fixtures", "relevance", "time-isolated", "annotation-guide.md")
	command, err := readRelevanceEvaluationDataset(datasetPath, 7, true)
	if err != nil {
		t.Fatalf("read committed relevance dataset: %v", err)
	}
	guide, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("read annotation guide: %v", err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(guide)); got != command.AnnotationGuidelineSHA256 {
		t.Fatalf("annotation guide digest = %s, dataset has %s", got, command.AnnotationGuidelineSHA256)
	}
	repository := &committedRelevanceEvaluationRepository{}
	service, err := ingestionapplication.NewRelevanceCalibrationService(
		ingestionapplication.NewRankSignalDocumentMatchReranker(), repository,
		committedRelevanceEvaluationClock{value: time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC)},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Evaluate(t.Context(), command)
	if err != nil {
		t.Fatalf("evaluate committed relevance dataset: %v", err)
	}
	if !result.Activated || result.Profile.Status != "active" || result.Metrics.SampleCount != 560 ||
		result.Metrics.PositiveCount != 280 || result.Metrics.NegativeCount != 280 ||
		result.Metrics.RecallAt100 < .95 || result.Metrics.Precision < .90 || result.Metrics.Recall < .80 ||
		result.Metrics.ECE > .05 || result.Metrics.PrecisionWilsonLower < .87 || !result.Metrics.HardNegativePassed {
		t.Fatalf("committed relevance metrics = %#v", result)
	}
	if len(result.Slices) != 9 {
		t.Fatalf("committed relevance slices = %d, want 9", len(result.Slices))
	}
}

type committedRelevanceEvaluationClock struct{ value time.Time }

func (clock committedRelevanceEvaluationClock) Now() time.Time { return clock.value }

type committedRelevanceEvaluationRepository struct{}

func (*committedRelevanceEvaluationRepository) PersistRelevanceCalibration(_ context.Context, command ingestionapplication.PersistRelevanceCalibrationCommand) (ingestionapplication.RelevanceCalibrationProfileDTO, error) {
	return ingestionapplication.RelevanceCalibrationProfileDTO{
		ID: 1, Version: 1, EvaluationRunID: 1,
		MatchingAlgorithmVersion: command.MatchingAlgorithmVersion,
		RerankerVersion:          command.RerankerVersion,
		CalibrationVersion:       command.CalibrationVersion,
		Status:                   command.Status,
		RejectThreshold:          command.RejectThreshold,
		AcceptThreshold:          command.AcceptThreshold,
		CalibrationSlope:         command.CalibrationSlope,
		CalibrationIntercept:     command.CalibrationIntercept,
		EvaluationSampleCount:    command.Metrics.SampleCount,
	}, nil
}

func TestReadRelevanceEvaluationDatasetMapsSafeVersionedSignals(t *testing.T) {
	path := writeRelevanceDatasetFixture(t, `{
  "profile_name":"production relevance",
  "dataset_version":"time-split-2026-08-v1",
  "family_isolation_hash":"`+strings.Repeat("a", 64)+`",
  "annotation_protocol_version":"dual-review-relevance-v1",
  "annotation_guideline_sha256":"`+strings.Repeat("c", 64)+`",
  "split_strategy_version":"time-family-event-isolated-v1",
  "annotator_count":2,
  "agreement_metric":"cohen_kappa",
  "agreement_score":0.96,
  "time_boundary":"2026-07-01T00:00:00Z",
  "required_slices":[{"dimension":"language","value":"zh"}],
  "reject_threshold":0.4,
  "accept_threshold":0.8,
  "samples":[{
    "sample_id":"sample-1",
    "content_family_hash":"`+strings.Repeat("b", 64)+`",
    "observed_at":"2026-08-01T00:00:00Z",
    "language":"zh",
    "source_type":"rss",
    "relevant":true,
    "retrieved_at_100":true,
    "hard_negative":false,
    "candidate":{"document_version_id":17,"rrf_score":0.02,"signals":[{"channel":"lexical","rank":1,"raw_score":0.9,"algorithm_version":"fts-trgm-dice-v1"}]}
  }]
}`)
	command, err := readRelevanceEvaluationDataset(path, 9, true)
	if err != nil {
		t.Fatalf("readRelevanceEvaluationDataset(): %v", err)
	}
	if command.ActorUserID != 9 || !command.Activate || command.ProfileName != "production relevance" || len(command.Samples) != 1 ||
		command.Samples[0].Candidate.DocumentVersionID != 17 || len(command.Samples[0].Candidate.Signals) != 1 ||
		command.Samples[0].Candidate.Signals[0].AlgorithmVersion != "fts-trgm-dice-v1" {
		t.Fatalf("command = %#v", command)
	}
}

func TestReadRelevanceEvaluationDatasetRejectsBodyUnknownFieldsAndTrailingValues(t *testing.T) {
	for _, fixture := range []struct {
		name, payload string
	}{
		{name: "body", payload: `{"body":"must never enter an evaluation dataset"}`},
		{name: "unknown", payload: `{"unexpected":"value"}`},
		{name: "trailing", payload: `{}` + "\n{}"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, err := readRelevanceEvaluationDataset(writeRelevanceDatasetFixture(t, fixture.payload), 9, false)
			if err == nil {
				t.Fatal("unsafe or ambiguous relevance dataset was accepted")
			}
		})
	}
}

func TestRunRelevanceCommandRequiresExplicitDatasetActorAndSubcommand(t *testing.T) {
	for _, arguments := range [][]string{nil, {"preview"}, {"evaluate"}, {"evaluate", "--dataset", "fixture.json"}} {
		err := runRelevanceCommand(t.Context(), testConfig(), arguments, &strings.Builder{})
		if err == nil {
			t.Fatalf("arguments %v were accepted", arguments)
		}
	}
	if err := runRelevanceCommand(t.Context(), testConfig(), []string{"evaluate", "--dataset", "fixture.json", "--actor-user-id", "0"}, &strings.Builder{}); err == nil {
		t.Fatal("non-positive actor was accepted")
	}
}

func TestRunRelevanceCommandPersistsOnlyEvaluationMetricsAndActivatesPassingProfile(t *testing.T) {
	dsn := initializedBootstrapDatabase(t)
	runtime, err := database.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	var actorUserID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO users (email,password_hash,display_name,role)
VALUES ('relevance-command-admin@example.test','fixture','relevance command admin','admin') RETURNING id`).Scan(&actorUserID); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	_ = runtime.Close()

	boundary := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	request := relevanceEvaluationDatasetRequest{
		ProfileName: "command relevance profile", DatasetVersion: "command-time-split-v1",
		FamilyIsolationHash: strings.Repeat("f", 64), TimeBoundary: boundary,
		AnnotationProtocolVersion: "dual-review-relevance-v1", AnnotationGuidelineSHA256: strings.Repeat("e", 64),
		SplitStrategyVersion: "time-family-event-isolated-v1", AnnotatorCount: 2,
		AgreementMetric: "cohen_kappa", AgreementScore: .96,
		RejectThreshold: .4, AcceptThreshold: .8,
		RequiredSlices: []relevanceEvaluationSliceRequest{{Dimension: "language", Value: "en"}, {Dimension: "language", Value: "zh"}, {Dimension: "source", Value: "rss"}},
		Samples:        make([]relevanceEvaluationSampleRequest, 400),
	}
	for index := range request.Samples {
		positive := index < 200
		language := "en"
		if index%2 == 1 {
			language = "zh"
		}
		signals := []relevanceEvaluationRecallSignalRequest{{Channel: "lexical", Rank: 100, RawScore: .01, AlgorithmVersion: ingestionapplication.LexicalRecallAlgorithmVersion}}
		if positive {
			signals = []relevanceEvaluationRecallSignalRequest{
				{Channel: "lexical", Rank: 1, RawScore: .9, AlgorithmVersion: ingestionapplication.LexicalRecallAlgorithmVersion},
				{Channel: "semantic", Rank: 1, RawScore: .9, AlgorithmVersion: ingestionapplication.SemanticRecallAlgorithmVersion},
				{Channel: "structured", Rank: 1, RawScore: 3, AlgorithmVersion: ingestionapplication.StructuredRecallAlgorithmVersion},
			}
		}
		request.Samples[index] = relevanceEvaluationSampleRequest{
			SampleID: fmt.Sprintf("sample-%04d", index+1), ContentFamilyHash: fmt.Sprintf("%064x", index+1),
			ObservedAt: boundary.Add(time.Duration(index+1) * time.Minute), Language: language, SourceType: "rss",
			Relevant: positive, RetrievedAt100: true, HardNegative: !positive,
			Candidate: relevanceEvaluationCandidateRequest{DocumentVersionID: int64(index + 1), RRFScore: .04, Signals: signals},
		}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	path := writeRelevanceDatasetFixture(t, string(payload))
	cfg := testConfig()
	cfg.DatabaseURL = dsn
	var output bytes.Buffer
	if err := runRelevanceCommand(t.Context(), cfg, []string{"evaluate", "--dataset", path, "--actor-user-id", fmt.Sprint(actorUserID), "--activate"}, &output); err != nil {
		t.Fatalf("runRelevanceCommand(): %v", err)
	}
	var response relevanceEvaluationCommandResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode command response: %v", err)
	}
	if response.ProfileID <= 0 || response.EvaluationRunID <= 0 || !response.Activated || response.ProfileStatus != "active" ||
		response.SampleCount != 400 || response.PositiveCount != 200 || response.NegativeCount != 200 || !response.HardNegativePassed {
		t.Fatalf("response = %#v", response)
	}
	if strings.Contains(output.String(), "content_family_hash") || strings.Contains(output.String(), "sample-") {
		t.Fatalf("command output disclosed dataset records: %s", output.String())
	}
}

func writeRelevanceDatasetFixture(t *testing.T, value string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "relevance-evaluation.json")
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testConfig() config.Config {
	value := config.Default()
	value.DatabaseURL = "postgres://unused"
	return value
}
