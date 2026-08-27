package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

const (
	ShadowQualityTrackBaseline = "baseline"
	ShadowQualityTrackAgent    = "agent"

	ShadowQualityStatusCandidate = "candidate"

	ShadowQualityCostAvailable   = "available"
	ShadowQualityCostUnavailable = "unavailable"

	ShadowQualityFailureModelProfileInvalid = "model_profile_invalid"
	ShadowQualityFailureModelUnavailable    = "model_unavailable"
	ShadowQualityFailureBudgetExhausted     = "budget_exhausted"
	ShadowQualityFailureRateLimited         = "rate_limited"
	ShadowQualityFailureTransient           = "provider_transient"
	ShadowQualityFailureTimeout             = "provider_timeout"
	ShadowQualityFailureOutputInvalid       = "output_invalid"
	ShadowQualityFailureUnknown             = "unknown"
)

const maximumShadowQualitySamples = 500

var (
	ErrInvalidShadowQualityDataset = errors.New("Agent Shadow quality dataset is invalid")
	shadowQualityIDPattern         = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
)

// ShadowQualityDataset is a fixed, versioned Golden input set. Human reviews
// bind to output hashes so annotations cannot silently drift when a runtime
// changes. Raw input and output are deliberately absent from the report.
type ShadowQualityDataset struct {
	DatasetVersion            string
	DatasetSHA256             string
	AnnotationProtocolVersion string
	AnnotatorCount            int
	Samples                   []ShadowQualitySample
}

type ShadowQualitySample struct {
	SampleID         string
	TaskType         domain.TaskType
	SchemaVersion    string
	Input            json.RawMessage
	ExpectedEvidence []ShadowQualityEvidenceLabel
	HumanReviews     []ShadowQualityHumanReview
}

// ShadowQualityEvidenceLabel supports the two evidence shapes in approved P0
// structured contracts: a content/locator reference or an exact source quote.
type ShadowQualityEvidenceLabel struct {
	ContentID  int64
	Locator    string
	ExactQuote string
}

type ShadowQualityHumanReview struct {
	Track         string
	OutputSHA256  string
	Accepted      bool
	ReviewerCount int
}

type ShadowQualityPricing struct {
	InputUSDPerMillion  float64
	OutputUSDPerMillion float64
}

type ShadowQualityTrack struct {
	Name, RuntimeName, ModelName, ModelVersion string
	Provider                                   domain.Provider
	RuntimeDegraded                            bool
	UsageAvailable                             bool
	Pricing                                    *ShadowQualityPricing
}

type ShadowQualityEvaluatorOptions struct {
	Timeout time.Duration
}

type ShadowQualityEvaluator struct {
	schemas *SchemaRegistry
	timeout time.Duration
}

// ShadowQualitySampleResult contains only bounded comparison facts. It never
// carries a Prompt, Golden input, provider error text, or model output.
type ShadowQualitySampleResult struct {
	SampleID               string          `json:"sample_id"`
	Track                  string          `json:"track"`
	TaskType               domain.TaskType `json:"task_type"`
	SchemaVersion          string          `json:"schema_version"`
	OutputSHA256           string          `json:"output_sha256,omitempty"`
	StructureValid         bool            `json:"structure_valid"`
	FailureCategory        string          `json:"failure_category,omitempty"`
	LatencyMS              int64           `json:"latency_ms"`
	InputTokens            int64           `json:"input_tokens"`
	OutputTokens           int64           `json:"output_tokens"`
	ExpectedEvidenceCount  int             `json:"expected_evidence_count"`
	PredictedEvidenceCount int             `json:"predicted_evidence_count"`
	CorrectEvidenceCount   int             `json:"correct_evidence_count"`
	HumanAccepted          *bool           `json:"human_accepted,omitempty"`
}

type ShadowQualityTrackMetric struct {
	Track               string         `json:"track"`
	RuntimeName         string         `json:"runtime_name"`
	ModelName           string         `json:"model_name"`
	ModelVersion        string         `json:"model_version"`
	RuntimeDegraded     bool           `json:"runtime_degraded"`
	SampleCount         int            `json:"sample_count"`
	SucceededCount      int            `json:"succeeded_count"`
	StructureValidCount int            `json:"structure_valid_count"`
	StructureValidRate  float64        `json:"structure_valid_rate"`
	EvidencePrecision   *float64       `json:"evidence_precision,omitempty"`
	EvidenceRecall      *float64       `json:"evidence_recall,omitempty"`
	HumanReviewedCount  int            `json:"human_reviewed_count"`
	HumanAcceptedCount  int            `json:"human_accepted_count"`
	HumanAcceptanceRate *float64       `json:"human_acceptance_rate,omitempty"`
	LatencyP50MS        int64          `json:"latency_p50_ms"`
	LatencyP95MS        int64          `json:"latency_p95_ms"`
	LatencyMaxMS        int64          `json:"latency_max_ms"`
	InputTokens         int64          `json:"input_tokens"`
	OutputTokens        int64          `json:"output_tokens"`
	UsageAvailable      bool           `json:"usage_available"`
	CostStatus          string         `json:"cost_status"`
	EstimatedCostUSD    *float64       `json:"estimated_cost_usd,omitempty"`
	FailureCategories   map[string]int `json:"failure_categories"`
}

type ShadowQualityReport struct {
	DatasetVersion            string                      `json:"dataset_version"`
	DatasetSHA256             string                      `json:"dataset_sha256"`
	AnnotationProtocolVersion string                      `json:"annotation_protocol_version"`
	AnnotatorCount            int                         `json:"annotator_count"`
	Status                    string                      `json:"status"`
	ApprovalReady             bool                        `json:"approval_ready"`
	ReasonCodes               []string                    `json:"reason_codes"`
	Tracks                    []ShadowQualityTrackMetric  `json:"tracks"`
	Samples                   []ShadowQualitySampleResult `json:"samples"`
}

func NewShadowQualityEvaluator(schemas *SchemaRegistry, options ShadowQualityEvaluatorOptions) (*ShadowQualityEvaluator, error) {
	if schemas == nil || options.Timeout <= 0 || options.Timeout > 5*time.Minute {
		return nil, fmt.Errorf("%w: schemas and a timeout between 1ns and 5m are required", ErrInvalidShadowQualityDataset)
	}
	return &ShadowQualityEvaluator{schemas: schemas, timeout: options.Timeout}, nil
}

// Evaluate runs the baseline and Agent for each fixed sample concurrently.
// It intentionally cannot return an approved report: product thresholds and a
// trusted live review remain separate G5 decisions.
func (evaluator *ShadowQualityEvaluator) Evaluate(ctx context.Context, dataset ShadowQualityDataset, baseline, agent ShadowQualityTrack) (ShadowQualityReport, error) {
	if evaluator == nil || evaluator.schemas == nil || ctx == nil ||
		validateShadowQualityTrack(baseline, ShadowQualityTrackBaseline) != nil ||
		validateShadowQualityTrack(agent, ShadowQualityTrackAgent) != nil ||
		evaluator.validateDataset(dataset) != nil {
		return ShadowQualityReport{}, ErrInvalidShadowQualityDataset
	}
	tracks := []ShadowQualityTrack{baseline, agent}
	results := make([]ShadowQualitySampleResult, 0, len(dataset.Samples)*len(tracks))
	for _, sample := range dataset.Samples {
		contract, err := evaluator.schemas.Structured(sample.TaskType, sample.SchemaVersion)
		if err != nil {
			return ShadowQualityReport{}, ErrInvalidShadowQualityDataset
		}
		pair := make([]ShadowQualitySampleResult, len(tracks))
		var wait sync.WaitGroup
		wait.Add(len(tracks))
		for index := range tracks {
			index := index
			go func() {
				defer wait.Done()
				pair[index] = evaluator.execute(ctx, sample, contract, tracks[index])
			}()
		}
		wait.Wait()
		results = append(results, pair...)
	}
	metrics := []ShadowQualityTrackMetric{
		aggregateShadowQualityTrack(baseline, results),
		aggregateShadowQualityTrack(agent, results),
	}
	reasons := []string{"quality_thresholds_not_approved"}
	if agent.RuntimeDegraded {
		reasons = append(reasons, "agent_runtime_degraded")
	}
	if shadowQualityMetricsHaveFailures(metrics) {
		reasons = append(reasons, "track_failures")
	}
	if shadowQualityMetricsHaveInvalidStructure(metrics) {
		reasons = append(reasons, "structure_invalid")
	}
	if shadowQualityMetricsNeedHumanReview(metrics) {
		reasons = append(reasons, "human_review_incomplete")
	}
	if shadowQualityMetricsNeedCost(metrics) {
		reasons = append(reasons, "cost_unavailable")
	}
	return ShadowQualityReport{
		DatasetVersion: dataset.DatasetVersion, DatasetSHA256: dataset.DatasetSHA256,
		AnnotationProtocolVersion: dataset.AnnotationProtocolVersion, AnnotatorCount: dataset.AnnotatorCount,
		Status: ShadowQualityStatusCandidate, ApprovalReady: false,
		ReasonCodes: reasons, Tracks: metrics, Samples: results,
	}, nil
}

func (evaluator *ShadowQualityEvaluator) execute(ctx context.Context, sample ShadowQualitySample, contract StructuredContract, track ShadowQualityTrack) ShadowQualitySampleResult {
	result := ShadowQualitySampleResult{
		SampleID: sample.SampleID, Track: track.Name, TaskType: sample.TaskType, SchemaVersion: sample.SchemaVersion,
		ExpectedEvidenceCount: len(sample.ExpectedEvidence),
	}
	request := domain.StructuredRequest{
		ModelName: track.ModelName, ModelVersion: track.ModelVersion, TaskType: sample.TaskType,
		SchemaName: contract.SchemaName, SchemaVersion: contract.SchemaVersion, Instruction: contract.Instruction,
		Schema: cloneRawJSON(contract.OutputSchema), Input: cloneRawJSON(sample.Input),
	}
	started := time.Now()
	callContext, cancel := context.WithTimeout(ctx, evaluator.timeout)
	response, err := track.Provider.GenerateStructured(callContext, request)
	callContextError := callContext.Err()
	cancel()
	result.LatencyMS = time.Since(started).Milliseconds()
	if callContextError != nil {
		result.FailureCategory = shadowQualityFailureCategory(callContextError, callContext)
		return result
	}
	if err != nil {
		result.FailureCategory = shadowQualityFailureCategory(err, callContext)
		return result
	}
	result.InputTokens, result.OutputTokens = response.Usage.InputTokens, response.Usage.OutputTokens
	if response.ModelVersion != track.ModelVersion {
		result.FailureCategory = ShadowQualityFailureModelProfileInvalid
		return result
	}
	if _, err := response.Usage.TotalTokens(); err != nil {
		result.FailureCategory = ShadowQualityFailureModelProfileInvalid
		return result
	}
	if len(response.JSON) != 0 {
		result.OutputSHA256 = shadowQualityDigest(response.JSON)
	}
	if evaluator.schemas.ValidateOutput(sample.TaskType, sample.SchemaVersion, response.JSON) != nil ||
		validateStructuredOutputPolicy(sample.TaskType, sample.SchemaVersion, sample.Input, response.JSON) != nil {
		result.FailureCategory = ShadowQualityFailureOutputInvalid
		return result
	}
	predicted, err := shadowQualityOutputEvidence(sample.TaskType, sample.SchemaVersion, response.JSON)
	if err != nil {
		result.FailureCategory = ShadowQualityFailureOutputInvalid
		return result
	}
	expected := shadowQualityExpectedEvidence(sample.ExpectedEvidence)
	result.PredictedEvidenceCount = len(predicted)
	for evidence := range predicted {
		if _, found := expected[evidence]; found {
			result.CorrectEvidenceCount++
		}
	}
	result.StructureValid = true
	for _, review := range sample.HumanReviews {
		if review.Track == track.Name && review.OutputSHA256 == result.OutputSHA256 {
			accepted := review.Accepted
			result.HumanAccepted = &accepted
			break
		}
	}
	return result
}

func (evaluator *ShadowQualityEvaluator) validateDataset(dataset ShadowQualityDataset) error {
	if !shadowQualityIDPattern.MatchString(dataset.DatasetVersion) || !shadowQualitySHA256(dataset.DatasetSHA256) ||
		!shadowQualityIDPattern.MatchString(dataset.AnnotationProtocolVersion) || dataset.AnnotatorCount < 0 || dataset.AnnotatorCount > 32 ||
		len(dataset.Samples) < 5 || len(dataset.Samples) > maximumShadowQualitySamples {
		return ErrInvalidShadowQualityDataset
	}
	required := map[string]bool{
		string(domain.TaskTypeTermExpansion) + "\x00v1":         false,
		string(domain.TaskTypeRelevanceReview) + "\x00v1":       false,
		string(domain.TaskTypeEventCluster) + "\x00v1":          false,
		string(domain.TaskTypeEventSummary) + "\x00v1":          false,
		string(domain.TaskTypeEntityClaimExtraction) + "\x00v2": false,
	}
	seen := make(map[string]struct{}, len(dataset.Samples))
	hasHumanReviews := false
	for _, sample := range dataset.Samples {
		if !shadowQualityIDPattern.MatchString(sample.SampleID) {
			return ErrInvalidShadowQualityDataset
		}
		if _, duplicate := seen[sample.SampleID]; duplicate {
			return ErrInvalidShadowQualityDataset
		}
		seen[sample.SampleID] = struct{}{}
		key := string(sample.TaskType) + "\x00" + sample.SchemaVersion
		if _, approved := required[key]; !approved || evaluator.schemas.ValidateInput(sample.TaskType, sample.SchemaVersion, sample.Input) != nil ||
			evaluator.schemas.ValidateOutput(sample.TaskType, sample.SchemaVersion, []byte(shadowQualityMinimalOutput(sample.TaskType, sample.SchemaVersion))) != nil ||
			validateShadowQualityExpectedEvidence(sample) != nil || validateShadowQualityHumanReviews(sample, dataset.AnnotatorCount) != nil {
			return ErrInvalidShadowQualityDataset
		}
		if len(sample.HumanReviews) > 0 {
			hasHumanReviews = true
		}
		required[key] = true
	}
	if hasHumanReviews && dataset.AnnotatorCount < 2 {
		return ErrInvalidShadowQualityDataset
	}
	for _, covered := range required {
		if !covered {
			return ErrInvalidShadowQualityDataset
		}
	}
	return nil
}

func validateShadowQualityTrack(track ShadowQualityTrack, expectedName string) error {
	if track.Name != expectedName || track.Provider == nil || !shadowQualityIDPattern.MatchString(track.RuntimeName) ||
		strings.TrimSpace(track.ModelName) == "" || len(track.ModelName) > 128 || strings.TrimSpace(track.ModelVersion) == "" || len(track.ModelVersion) > 128 {
		return ErrInvalidShadowQualityDataset
	}
	if track.Pricing == nil {
		return nil
	}
	if !track.UsageAvailable || math.IsNaN(track.Pricing.InputUSDPerMillion) || math.IsInf(track.Pricing.InputUSDPerMillion, 0) ||
		math.IsNaN(track.Pricing.OutputUSDPerMillion) || math.IsInf(track.Pricing.OutputUSDPerMillion, 0) ||
		track.Pricing.InputUSDPerMillion < 0 || track.Pricing.OutputUSDPerMillion < 0 ||
		track.Pricing.InputUSDPerMillion > 1_000_000 || track.Pricing.OutputUSDPerMillion > 1_000_000 {
		return ErrInvalidShadowQualityDataset
	}
	return nil
}

func validateShadowQualityHumanReviews(sample ShadowQualitySample, annotatorCount int) error {
	seen := make(map[string]struct{}, 2)
	for _, review := range sample.HumanReviews {
		if review.Track != ShadowQualityTrackBaseline && review.Track != ShadowQualityTrackAgent || !shadowQualitySHA256(review.OutputSHA256) ||
			review.ReviewerCount < 2 || review.ReviewerCount > annotatorCount {
			return ErrInvalidShadowQualityDataset
		}
		if _, duplicate := seen[review.Track]; duplicate {
			return ErrInvalidShadowQualityDataset
		}
		seen[review.Track] = struct{}{}
	}
	return nil
}

func validateShadowQualityExpectedEvidence(sample ShadowQualitySample) error {
	if len(sample.ExpectedEvidence) > 64 {
		return ErrInvalidShadowQualityDataset
	}
	switch {
	case sample.TaskType == domain.TaskTypeEventSummary && sample.SchemaVersion == "v1",
		sample.TaskType == domain.TaskTypeEntityClaimExtraction && sample.SchemaVersion == "v1":
		var input struct {
			Evidence []structuredEvidenceReference `json:"evidence"`
		}
		if json.Unmarshal(sample.Input, &input) != nil {
			return ErrInvalidShadowQualityDataset
		}
		allowed := make(map[string]struct{}, len(input.Evidence))
		for _, evidence := range input.Evidence {
			allowed[shadowQualityContentEvidenceKey(evidence.ContentID, evidence.Locator)] = struct{}{}
		}
		seen := make(map[string]struct{}, len(sample.ExpectedEvidence))
		for _, label := range sample.ExpectedEvidence {
			key := shadowQualityContentEvidenceKey(label.ContentID, label.Locator)
			if label.ContentID <= 0 || strings.TrimSpace(label.Locator) == "" || label.ExactQuote != "" {
				return ErrInvalidShadowQualityDataset
			}
			if _, exists := allowed[key]; !exists {
				return ErrInvalidShadowQualityDataset
			}
			if _, duplicate := seen[key]; duplicate {
				return ErrInvalidShadowQualityDataset
			}
			seen[key] = struct{}{}
		}
	case sample.TaskType == domain.TaskTypeEntityClaimExtraction && sample.SchemaVersion == "v2":
		var input struct {
			Body string `json:"body"`
		}
		if json.Unmarshal(sample.Input, &input) != nil || input.Body == "" {
			return ErrInvalidShadowQualityDataset
		}
		seen := make(map[string]struct{}, len(sample.ExpectedEvidence))
		for _, label := range sample.ExpectedEvidence {
			if label.ContentID != 0 || label.Locator != "" || label.ExactQuote == "" || len(label.ExactQuote) > 4096 || !strings.Contains(input.Body, label.ExactQuote) {
				return ErrInvalidShadowQualityDataset
			}
			key := shadowQualityQuoteEvidenceKey(label.ExactQuote)
			if _, duplicate := seen[key]; duplicate {
				return ErrInvalidShadowQualityDataset
			}
			seen[key] = struct{}{}
		}
	default:
		if len(sample.ExpectedEvidence) != 0 {
			return ErrInvalidShadowQualityDataset
		}
	}
	return nil
}

func shadowQualityOutputEvidence(taskType domain.TaskType, schemaVersion string, output json.RawMessage) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	switch {
	case schemaVersion == "v1" && (taskType == domain.TaskTypeEventSummary || taskType == domain.TaskTypeEntityClaimExtraction):
		var decoded struct {
			Sentences []struct {
				Evidence []structuredEvidenceReference `json:"evidence"`
			} `json:"sentences"`
			Claims []struct {
				Evidence []structuredEvidenceReference `json:"evidence"`
			} `json:"claims"`
		}
		if json.Unmarshal(output, &decoded) != nil {
			return nil, ErrInvalidShadowQualityDataset
		}
		if taskType == domain.TaskTypeEventSummary {
			for _, sentence := range decoded.Sentences {
				for _, evidence := range sentence.Evidence {
					result[shadowQualityContentEvidenceKey(evidence.ContentID, evidence.Locator)] = struct{}{}
				}
			}
		} else {
			for _, claim := range decoded.Claims {
				for _, evidence := range claim.Evidence {
					result[shadowQualityContentEvidenceKey(evidence.ContentID, evidence.Locator)] = struct{}{}
				}
			}
		}
	case schemaVersion == "v2" && taskType == domain.TaskTypeEntityClaimExtraction:
		var decoded struct {
			Claims []struct {
				ExactQuote string `json:"exact_quote"`
			} `json:"claims"`
		}
		if json.Unmarshal(output, &decoded) != nil {
			return nil, ErrInvalidShadowQualityDataset
		}
		for _, claim := range decoded.Claims {
			result[shadowQualityQuoteEvidenceKey(claim.ExactQuote)] = struct{}{}
		}
	}
	return result, nil
}

func shadowQualityExpectedEvidence(labels []ShadowQualityEvidenceLabel) map[string]struct{} {
	result := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		if label.ExactQuote != "" {
			result[shadowQualityQuoteEvidenceKey(label.ExactQuote)] = struct{}{}
		} else {
			result[shadowQualityContentEvidenceKey(label.ContentID, label.Locator)] = struct{}{}
		}
	}
	return result
}

func shadowQualityContentEvidenceKey(contentID int64, locator string) string {
	return fmt.Sprintf("content:%d:%s", contentID, shadowQualityDigest([]byte(locator)))
}

func shadowQualityQuoteEvidenceKey(quote string) string {
	return "quote:" + shadowQualityDigest([]byte(quote))
}

func shadowQualityDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func shadowQualitySHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func shadowQualityFailureCategory(err error, ctx context.Context) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return ShadowQualityFailureTimeout
	}
	code, ok := domain.CodeOf(err)
	if !ok {
		return ShadowQualityFailureTransient
	}
	switch code {
	case domain.CodeAIModelProfileInvalid:
		return ShadowQualityFailureModelProfileInvalid
	case domain.CodeAIModelUnavailable:
		return ShadowQualityFailureModelUnavailable
	case domain.CodeAIBudgetExhausted:
		return ShadowQualityFailureBudgetExhausted
	case domain.CodeAIProviderRateLimited:
		return ShadowQualityFailureRateLimited
	case domain.CodeAIProviderTransient:
		return ShadowQualityFailureTransient
	case domain.CodeAIProviderTimeout:
		return ShadowQualityFailureTimeout
	case domain.CodeAIOutputInvalid:
		return ShadowQualityFailureOutputInvalid
	default:
		return ShadowQualityFailureUnknown
	}
}

func aggregateShadowQualityTrack(track ShadowQualityTrack, results []ShadowQualitySampleResult) ShadowQualityTrackMetric {
	metric := ShadowQualityTrackMetric{
		Track: track.Name, RuntimeName: track.RuntimeName, ModelName: track.ModelName, ModelVersion: track.ModelVersion,
		RuntimeDegraded: track.RuntimeDegraded, UsageAvailable: track.UsageAvailable,
		CostStatus: ShadowQualityCostUnavailable, FailureCategories: make(map[string]int),
	}
	latencies := make([]int64, 0)
	expectedEvidence, predictedEvidence, correctEvidence := 0, 0, 0
	for _, result := range results {
		if result.Track != track.Name {
			continue
		}
		metric.SampleCount++
		latencies = append(latencies, result.LatencyMS)
		if result.FailureCategory == "" {
			metric.SucceededCount++
		} else {
			metric.FailureCategories[result.FailureCategory]++
		}
		if result.StructureValid {
			metric.StructureValidCount++
		}
		if result.HumanAccepted != nil {
			metric.HumanReviewedCount++
			if *result.HumanAccepted {
				metric.HumanAcceptedCount++
			}
		}
		expectedEvidence += result.ExpectedEvidenceCount
		predictedEvidence += result.PredictedEvidenceCount
		correctEvidence += result.CorrectEvidenceCount
		if track.UsageAvailable {
			metric.InputTokens += result.InputTokens
			metric.OutputTokens += result.OutputTokens
		}
	}
	if metric.SampleCount > 0 {
		metric.StructureValidRate = float64(metric.StructureValidCount) / float64(metric.SampleCount)
	}
	if expectedEvidence > 0 {
		precision := 0.0
		if predictedEvidence > 0 {
			precision = float64(correctEvidence) / float64(predictedEvidence)
		}
		recall := float64(correctEvidence) / float64(expectedEvidence)
		metric.EvidencePrecision, metric.EvidenceRecall = &precision, &recall
	}
	if metric.HumanReviewedCount > 0 {
		acceptance := float64(metric.HumanAcceptedCount) / float64(metric.HumanReviewedCount)
		metric.HumanAcceptanceRate = &acceptance
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	metric.LatencyP50MS = shadowQualityPercentile(latencies, .50)
	metric.LatencyP95MS = shadowQualityPercentile(latencies, .95)
	if len(latencies) > 0 {
		metric.LatencyMaxMS = latencies[len(latencies)-1]
	}
	if track.UsageAvailable && track.Pricing != nil {
		cost := (float64(metric.InputTokens)*track.Pricing.InputUSDPerMillion + float64(metric.OutputTokens)*track.Pricing.OutputUSDPerMillion) / 1_000_000
		metric.CostStatus, metric.EstimatedCostUSD = ShadowQualityCostAvailable, &cost
	}
	return metric
}

func shadowQualityPercentile(sorted []int64, percentile float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	return sorted[index]
}

func shadowQualityMetricsHaveFailures(metrics []ShadowQualityTrackMetric) bool {
	for _, metric := range metrics {
		if len(metric.FailureCategories) != 0 {
			return true
		}
	}
	return false
}

func shadowQualityMetricsHaveInvalidStructure(metrics []ShadowQualityTrackMetric) bool {
	for _, metric := range metrics {
		if metric.StructureValidCount != metric.SampleCount {
			return true
		}
	}
	return false
}

func shadowQualityMetricsNeedHumanReview(metrics []ShadowQualityTrackMetric) bool {
	for _, metric := range metrics {
		if metric.HumanReviewedCount != metric.SampleCount {
			return true
		}
	}
	return false
}

func shadowQualityMetricsNeedCost(metrics []ShadowQualityTrackMetric) bool {
	for _, metric := range metrics {
		if metric.CostStatus != ShadowQualityCostAvailable {
			return true
		}
	}
	return false
}

// The validator only needs one schema-shaped value per approved contract to
// ensure the registry contains the expected version. It is not a quality
// label and never becomes a report result.
func shadowQualityMinimalOutput(taskType domain.TaskType, schemaVersion string) string {
	switch {
	case taskType == domain.TaskTypeTermExpansion && schemaVersion == "v1":
		return `{"terms":[]}`
	case taskType == domain.TaskTypeRelevanceReview && schemaVersion == "v1":
		return `{"decision":"review","score":0,"reason_codes":["insufficient_evidence"]}`
	case taskType == domain.TaskTypeEventCluster && schemaVersion == "v1":
		return `{"action":"create","confidence":0,"reason_codes":["no_candidate"]}`
	case taskType == domain.TaskTypeEventSummary && schemaVersion == "v1":
		return `{"title_zh":"待分析","sentences":[]}`
	case taskType == domain.TaskTypeEntityClaimExtraction && schemaVersion == "v2":
		return `{"claims":[{"subject":"pending","predicate":"pending","object":"pending","relation":"unknown","exact_quote":"pending","relation_score":0,"qualifiers":[]}]}`
	default:
		return `{}`
	}
}
