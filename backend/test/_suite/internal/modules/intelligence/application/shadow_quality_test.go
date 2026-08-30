package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

type shadowQualityProviderFake struct {
	name    string
	started chan<- string
	release <-chan struct{}
	result  func(domain.StructuredRequest) (domain.StructuredResponse, error)
}

func (fake shadowQualityProviderFake) Embed(context.Context, domain.EmbeddingRequest) (domain.EmbeddingResponse, error) {
	return domain.EmbeddingResponse{}, domain.NewError(domain.CodeAIModelUnavailable)
}

func (fake shadowQualityProviderFake) GenerateStructured(_ context.Context, request domain.StructuredRequest) (domain.StructuredResponse, error) {
	if fake.started != nil {
		fake.started <- fake.name
	}
	if fake.release != nil {
		<-fake.release
	}
	return fake.result(request)
}

func TestShadowQualityEvaluatorRunsTracksConcurrentlyAndKeepsReportCandidate(t *testing.T) {
	dataset := passingShadowQualityDataset(t)
	started := make(chan string, 10)
	release := make(chan struct{})
	baseline := shadowQualityProviderFake{name: "baseline", started: started, release: release, result: func(request domain.StructuredRequest) (domain.StructuredResponse, error) {
		return shadowQualityResponse(request, "baseline-v1", false), nil
	}}
	agent := shadowQualityProviderFake{name: "agent", started: started, release: release, result: func(request domain.StructuredRequest) (domain.StructuredResponse, error) {
		return shadowQualityResponse(request, "agent-v1", true), nil
	}}
	registry, err := NewSchemaRegistry()
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := NewShadowQualityEvaluator(registry, ShadowQualityEvaluatorOptions{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan ShadowQualityReport, 1)
	failures := make(chan error, 1)
	go func() {
		report, evaluateErr := evaluator.Evaluate(t.Context(), dataset,
			ShadowQualityTrack{Name: ShadowQualityTrackBaseline, RuntimeName: "openai", ModelName: "baseline-model", ModelVersion: "baseline-v1", Provider: baseline, UsageAvailable: true, Pricing: &ShadowQualityPricing{InputUSDPerMillion: 2, OutputUSDPerMillion: 4}},
			ShadowQualityTrack{Name: ShadowQualityTrackAgent, RuntimeName: "hotkey-agent", ModelName: "agent-model", ModelVersion: "agent-v1", Provider: agent, RuntimeDegraded: true, UsageAvailable: true, Pricing: &ShadowQualityPricing{InputUSDPerMillion: 1, OutputUSDPerMillion: 2}},
		)
		if evaluateErr != nil {
			failures <- evaluateErr
			return
		}
		result <- report
	}()
	seen := map[string]bool{}
	for range 2 {
		select {
		case name := <-started:
			seen[name] = true
		case <-time.After(time.Second):
			t.Fatal("baseline and Agent did not start the same Golden sample concurrently")
		}
	}
	if !seen["baseline"] || !seen["agent"] {
		t.Fatalf("concurrent tracks = %#v", seen)
	}
	close(release)
	var report ShadowQualityReport
	select {
	case err := <-failures:
		t.Fatal(err)
	case report = <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Shadow quality report")
	}
	if report.Status != ShadowQualityStatusCandidate || report.ApprovalReady || report.DatasetVersion != dataset.DatasetVersion || report.DatasetSHA256 != dataset.DatasetSHA256 ||
		report.AnnotationProtocolVersion != dataset.AnnotationProtocolVersion || report.AnnotatorCount != dataset.AnnotatorCount || len(report.Samples) != 10 {
		t.Fatalf("quality report identity = %#v", report)
	}
	for _, reason := range []string{"quality_thresholds_not_approved", "agent_runtime_degraded"} {
		if !containsQualityString(report.ReasonCodes, reason) {
			t.Fatalf("candidate reasons = %#v, missing %q", report.ReasonCodes, reason)
		}
	}
	baselineMetric := shadowQualityMetric(t, report, ShadowQualityTrackBaseline)
	agentMetric := shadowQualityMetric(t, report, ShadowQualityTrackAgent)
	if baselineMetric.SampleCount != 5 || baselineMetric.StructureValidCount != 5 || baselineMetric.HumanReviewedCount != 5 || baselineMetric.HumanAcceptedCount != 5 || baselineMetric.EvidencePrecision == nil || *baselineMetric.EvidencePrecision != 1 || baselineMetric.EvidenceRecall == nil || *baselineMetric.EvidenceRecall != 1 || baselineMetric.CostStatus != ShadowQualityCostAvailable || baselineMetric.EstimatedCostUSD == nil {
		t.Fatalf("baseline metric = %#v", baselineMetric)
	}
	if agentMetric.SampleCount != 5 || agentMetric.StructureValidCount != 5 || agentMetric.HumanReviewedCount != 5 || agentMetric.HumanAcceptedCount != 3 || agentMetric.EvidencePrecision == nil || *agentMetric.EvidencePrecision != .5 || agentMetric.EvidenceRecall == nil || *agentMetric.EvidenceRecall != .5 || agentMetric.CostStatus != ShadowQualityCostAvailable || agentMetric.EstimatedCostUSD == nil {
		t.Fatalf("Agent metric = %#v", agentMetric)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"Monitor a launch", "HotKey launched analysis", "baseline accepted", "agent rejected"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("quality report leaked Golden input or review text %q", forbidden)
		}
	}
}

func TestShadowQualityEvaluatorExportsValidatedReviewCandidatesOnlyWhenRequested(t *testing.T) {
	dataset := passingShadowQualityDataset(t)
	registry, err := NewSchemaRegistry()
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := NewShadowQualityEvaluator(registry, ShadowQualityEvaluatorOptions{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	baseline := shadowQualityProviderFake{result: func(request domain.StructuredRequest) (domain.StructuredResponse, error) {
		return shadowQualityResponse(request, "baseline-v1", false), nil
	}}
	agent := shadowQualityProviderFake{result: func(request domain.StructuredRequest) (domain.StructuredResponse, error) {
		return shadowQualityResponse(request, "agent-v1", true), nil
	}}
	report, candidates, err := evaluator.EvaluateForReview(t.Context(), dataset,
		ShadowQualityTrack{Name: ShadowQualityTrackBaseline, RuntimeName: "codex", ModelName: "baseline-model", ModelVersion: "baseline-v1", Provider: baseline},
		ShadowQualityTrack{Name: ShadowQualityTrackAgent, RuntimeName: "hotkey-agent", ModelName: "agent-model", ModelVersion: "agent-v1", Provider: agent},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 10 || len(report.Samples) != 10 {
		t.Fatalf("review candidates = %d, report samples = %d", len(candidates), len(report.Samples))
	}
	first := candidates[0]
	if first.SampleID != dataset.Samples[0].SampleID || first.Track != ShadowQualityTrackBaseline || first.RuntimeName != "codex" ||
		first.ModelName != "baseline-model" || first.ModelVersion != "baseline-v1" ||
		!json.Valid(first.Input) || !json.Valid(first.Output) || first.OutputSHA256 != qualityTestDigest(first.Output) {
		t.Fatalf("first review candidate = %#v", first)
	}
	first.Input[0] = '['
	first.Output[0] = '['
	if dataset.Samples[0].Input[0] != '{' {
		t.Fatal("review candidate input aliases the Golden dataset")
	}
	encodedReport, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedReport), "Monitor a launch") || strings.Contains(string(encodedReport), "HotKey事件") {
		t.Fatal("review-only input or output leaked into the sanitized report")
	}
}

func TestShadowQualityEvaluatorRejectsIncompleteOrAmbiguousGoldenDatasets(t *testing.T) {
	registry, err := NewSchemaRegistry()
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := NewShadowQualityEvaluator(registry, ShadowQualityEvaluatorOptions{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	provider := shadowQualityProviderFake{result: func(request domain.StructuredRequest) (domain.StructuredResponse, error) {
		return shadowQualityResponse(request, "runtime-v1", false), nil
	}}
	track := ShadowQualityTrack{Name: ShadowQualityTrackBaseline, RuntimeName: "fixture", ModelName: "fixture", ModelVersion: "runtime-v1", Provider: provider}
	for _, mutate := range []func(*ShadowQualityDataset){
		func(value *ShadowQualityDataset) { value.Samples[1].SampleID = value.Samples[0].SampleID },
		func(value *ShadowQualityDataset) { value.Samples = value.Samples[:4] },
		func(value *ShadowQualityDataset) {
			value.Samples[0].Input = json.RawMessage(`{"objective":"missing fields"}`)
		},
		func(value *ShadowQualityDataset) { value.Samples[0].HumanReviews[0].OutputSHA256 = "not-a-digest" },
		func(value *ShadowQualityDataset) { value.Samples[3].ExpectedEvidence[0].ContentID = 999 },
	} {
		dataset := passingShadowQualityDataset(t)
		mutate(&dataset)
		if _, err := evaluator.Evaluate(t.Context(), dataset, track, ShadowQualityTrack{Name: ShadowQualityTrackAgent, RuntimeName: "fixture", ModelName: "fixture", ModelVersion: "runtime-v1", Provider: provider}); !errors.Is(err, ErrInvalidShadowQualityDataset) {
			t.Fatalf("invalid dataset error = %v", err)
		}
	}
}

func TestShadowQualityEvaluatorMapsFailuresWithoutPreservingProviderText(t *testing.T) {
	dataset := passingShadowQualityDataset(t)
	registry, err := NewSchemaRegistry()
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := NewShadowQualityEvaluator(registry, ShadowQualityEvaluatorOptions{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	baseline := shadowQualityProviderFake{result: func(domain.StructuredRequest) (domain.StructuredResponse, error) {
		return domain.StructuredResponse{}, errors.New("provider token=do-not-store prompt=private")
	}}
	agent := shadowQualityProviderFake{result: func(request domain.StructuredRequest) (domain.StructuredResponse, error) {
		return domain.StructuredResponse{ModelVersion: "agent-v1", JSON: json.RawMessage(`{"forbidden":true}`)}, nil
	}}
	report, candidates, err := evaluator.EvaluateForReview(t.Context(), dataset,
		ShadowQualityTrack{Name: ShadowQualityTrackBaseline, RuntimeName: "openai", ModelName: "baseline", ModelVersion: "baseline-v1", Provider: baseline},
		ShadowQualityTrack{Name: ShadowQualityTrackAgent, RuntimeName: "agent", ModelName: "agent", ModelVersion: "agent-v1", Provider: agent},
	)
	if err != nil {
		t.Fatal(err)
	}
	if shadowQualityMetric(t, report, ShadowQualityTrackBaseline).FailureCategories[ShadowQualityFailureTransient] != 5 || shadowQualityMetric(t, report, ShadowQualityTrackAgent).FailureCategories[ShadowQualityFailureOutputInvalid] != 5 || !containsQualityString(report.ReasonCodes, "track_failures") {
		t.Fatalf("failure report = %#v", report)
	}
	if len(candidates) != 0 {
		t.Fatalf("failed or invalid outputs entered review bundle: %#v", candidates)
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "do-not-store") || strings.Contains(string(encoded), "private") {
		t.Fatal("provider error text leaked into quality report")
	}
}

func passingShadowQualityDataset(t *testing.T) ShadowQualityDataset {
	t.Helper()
	samples := []ShadowQualitySample{
		{SampleID: "monitor-compile-001", TaskType: domain.TaskTypeTermExpansion, SchemaVersion: "v1", Input: json.RawMessage(`{"objective":"Monitor a launch","clauses":[{"operator":"must","field":"term","value":"HotKey"}],"entities":[],"examples":[],"existing_candidates":[],"output_languages":["en"]}`)},
		{SampleID: "relevance-001", TaskType: domain.TaskTypeRelevanceReview, SchemaVersion: "v1", Input: json.RawMessage(`{"content_excerpt":"HotKey launched analysis.","content_language":"en","monitor_intent":"Track HotKey launches","scoring_version":"relevance-v1","scores":{"semantic":90,"lexical":90,"entity":90,"title":80,"preference":50},"recall_paths":["lexical"],"reason_codes":["lexical_candidate"],"evidence_terms":["HotKey"]}`)},
		{SampleID: "event-cluster-001", TaskType: domain.TaskTypeEventCluster, SchemaVersion: "v1", Input: json.RawMessage(`{"content_family_id":7,"family_version":1,"subject_keys":["hotkey"],"action_keys":["launch"],"location_keys":[],"identifier_keys":["release-1"],"event_started_at":"2026-08-01T00:00:00Z","candidates":[{"micro_event_id":41,"event_version":1,"same_event_score":0.95,"hard_conflict_reasons":[]}]}`)},
		{SampleID: "event-summary-001", TaskType: domain.TaskTypeEventSummary, SchemaVersion: "v1", Input: json.RawMessage(`{"event_id":41,"event_key":"evt-41","evidence":[{"content_id":11,"locator":"body:0","excerpt":"HotKey launched analysis."},{"content_id":12,"locator":"body:1","excerpt":"Secondary observation."}]}`), ExpectedEvidence: []ShadowQualityEvidenceLabel{{ContentID: 11, Locator: "body:0"}}},
		{SampleID: "claim-evidence-001", TaskType: domain.TaskTypeEntityClaimExtraction, SchemaVersion: "v2", Input: json.RawMessage(`{"event_id":41,"event_version":1,"event_key":"evt-41","document_version_id":71,"plaintext_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","body":"HotKey launched analysis.","body_truncated":false}`), ExpectedEvidence: []ShadowQualityEvidenceLabel{{ExactQuote: "HotKey launched analysis."}}},
	}
	for index := range samples {
		baseline := shadowQualityResponse(domain.StructuredRequest{TaskType: samples[index].TaskType}, "baseline-v1", false)
		agent := shadowQualityResponse(domain.StructuredRequest{TaskType: samples[index].TaskType}, "agent-v1", true)
		samples[index].HumanReviews = []ShadowQualityHumanReview{
			{Track: ShadowQualityTrackBaseline, OutputSHA256: qualityTestDigest(baseline.JSON), Accepted: true, ReviewerCount: 2},
			{Track: ShadowQualityTrackAgent, OutputSHA256: qualityTestDigest(agent.JSON), Accepted: index < 3, ReviewerCount: 2},
		}
	}
	return ShadowQualityDataset{
		DatasetVersion: "agent-shadow-golden-v1", DatasetSHA256: strings.Repeat("a", 64),
		AnnotationProtocolVersion: "dual-review-v1", AnnotatorCount: 2, Samples: samples,
	}
}

func shadowQualityResponse(request domain.StructuredRequest, version string, agent bool) domain.StructuredResponse {
	var output string
	switch request.TaskType {
	case domain.TaskTypeTermExpansion:
		output = `{"terms":[]}`
	case domain.TaskTypeRelevanceReview:
		output = `{"decision":"accepted","score":90,"reason_codes":["relevant_evidence"]}`
	case domain.TaskTypeEventCluster:
		output = `{"action":"attach","candidate_micro_event_id":41,"confidence":0.95,"reason_codes":["matching_identifier"]}`
	case domain.TaskTypeEventSummary:
		if agent {
			output = `{"title_zh":"HotKey事件","sentences":[{"text":"Secondary observation.","evidence":[{"content_id":12,"locator":"body:1","excerpt":"Secondary observation."}]}]}`
		} else {
			output = `{"title_zh":"HotKey事件","sentences":[{"text":"HotKey launched analysis.","evidence":[{"content_id":11,"locator":"body:0","excerpt":"HotKey launched analysis."}]}]}`
		}
	case domain.TaskTypeEntityClaimExtraction:
		output = `{"claims":[{"subject":"HotKey","predicate":"launched","object":"analysis","relation":"asserts","exact_quote":"HotKey launched analysis.","relation_score":1,"qualifiers":[]}]}`
	default:
		output = `{}`
	}
	return domain.StructuredResponse{ModelVersion: version, JSON: json.RawMessage(output), Usage: domain.Usage{InputTokens: 10, OutputTokens: 5}}
}

func qualityTestDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func shadowQualityMetric(t *testing.T, report ShadowQualityReport, track string) ShadowQualityTrackMetric {
	t.Helper()
	for _, metric := range report.Tracks {
		if metric.Track == track {
			return metric
		}
	}
	t.Fatalf("missing metric for track %q", track)
	return ShadowQualityTrackMetric{}
}

func containsQualityString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

var _ domain.Provider = shadowQualityProviderFake{}
