package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	"golang.org/x/text/unicode/norm"
)

const (
	IntentExpansionPromptVersion    = "monitor-intent-term-expansion-v1"
	IntentExpansionTargetType       = "monitor_intent_expansion_run"
	intentExpansionInputSchema      = "v1"
	intentExpansionOutputSchema     = "v1"
	maximumIntentExpansionTerms     = 32
	maximumIntentExpansionTermRunes = 120
)

var (
	ErrIntentExpansionProcessorUnavailable = errors.New("monitor intent expansion processor is unavailable")
	ErrIntentExpansionOutputInvalid        = errors.New("monitor intent expansion output is invalid")
	ErrIntentPreviewProcessorUnavailable   = errors.New("monitor intent preview processor is unavailable")

	probabilityOrFactReasonPattern = regexp.MustCompile(`(?i)(^|[^[:alnum:]_])(probability|probable|likely|likelihood|confidence|confident|verified|confirmed|credible|credibility|true|false|fact|factual)([^[:alnum:]_]|$)`)
)

type intentExpansionPreparer interface {
	PrepareIntentExpansion(context.Context, monitorapplication.PrepareIntentExpansionQuery) (monitorapplication.PrepareIntentExpansionResult, error)
}

type intentExpansionStructuredRunner interface {
	ExecuteStructured(context.Context, intelligenceapplication.StructuredExecutionInput) (intelligenceapplication.StructuredExecutionResult, error)
}

type intentPreviewProcessor interface {
	EvaluatePreview(context.Context, monitorapplication.IntentAnalysisTaskDTO) (monitorapplication.IntentPreviewDTO, error)
}

// IntentAnalysisCompositeProcessor intentionally implements only production
// expansion. Preview remains unavailable until a real immutable-document
// evaluator exists; it is never represented by an empty or fixed result.
type IntentAnalysisCompositeProcessor struct {
	preparer intentExpansionPreparer
	runner   intentExpansionStructuredRunner
	preview  intentPreviewProcessor
}

func NewIntentAnalysisCompositeProcessor(intents *monitorapplication.IntentService, runs *intelligenceapplication.RunService, previews ...*IntentPreviewEvaluator) (*IntentAnalysisCompositeProcessor, error) {
	processors := make([]intentPreviewProcessor, 0, len(previews))
	for _, preview := range previews {
		if preview != nil {
			processors = append(processors, preview)
		}
	}
	return newIntentAnalysisCompositeProcessor(intents, runs, processors...)
}

func newIntentAnalysisCompositeProcessor(preparer intentExpansionPreparer, runner intentExpansionStructuredRunner, previews ...intentPreviewProcessor) (*IntentAnalysisCompositeProcessor, error) {
	if preparer == nil || runner == nil {
		return nil, ErrIntentExpansionProcessorUnavailable
	}
	if len(previews) > 1 {
		return nil, ErrIntentPreviewProcessorUnavailable
	}
	processor := &IntentAnalysisCompositeProcessor{preparer: preparer, runner: runner}
	if len(previews) == 1 {
		processor.preview = previews[0]
	}
	return processor, nil
}

func (processor *IntentAnalysisCompositeProcessor) Available(kind string) bool {
	if processor == nil {
		return false
	}
	switch kind {
	case "expansion":
		return processor.preparer != nil && processor.runner != nil
	case "preview":
		return processor.preview != nil
	default:
		return false
	}
}

func (processor *IntentAnalysisCompositeProcessor) EvaluatePreview(ctx context.Context, task monitorapplication.IntentAnalysisTaskDTO) (monitorapplication.IntentPreviewDTO, error) {
	if processor == nil || processor.preview == nil {
		return monitorapplication.IntentPreviewDTO{}, ErrIntentPreviewProcessorUnavailable
	}
	return processor.preview.EvaluatePreview(ctx, task)
}

func (processor *IntentAnalysisCompositeProcessor) GenerateExpansion(ctx context.Context, task monitorapplication.IntentAnalysisTaskDTO) ([]monitorapplication.ExpansionCandidateDTO, error) {
	if !processor.Available("expansion") {
		return nil, ErrIntentExpansionProcessorUnavailable
	}
	prepared, err := processor.preparer.PrepareIntentExpansion(ctx, monitorapplication.PrepareIntentExpansionQuery{Task: task})
	if err != nil {
		return nil, err
	}
	if prepared.Expansion.Task != task {
		return nil, ErrIntentExpansionOutputInvalid
	}
	input, err := intentExpansionModelInput(prepared.Expansion.Draft)
	if err != nil {
		return nil, err
	}
	encodedInput, err := json.Marshal(input)
	if err != nil {
		return nil, ErrIntentExpansionOutputInvalid
	}
	evidenceHash := sha256.Sum256(encodedInput)
	executed, err := processor.runner.ExecuteStructured(ctx, intelligenceapplication.StructuredExecutionInput{
		TaskType:   intelligencedomain.TaskTypeTermExpansion,
		TargetType: IntentExpansionTargetType, TargetID: task.Run.RunID,
		PromptVersion: IntentExpansionPromptVersion, InputSchemaVersion: intentExpansionInputSchema,
		SchemaVersion: intentExpansionOutputSchema, ParametersVersion: task.AnalysisProfile,
		InputHash: task.Run.InputHash, EvidenceSetHash: hex.EncodeToString(evidenceHash[:]),
		Input: encodedInput,
	})
	if err != nil {
		return nil, err
	}
	if executed.Status != "succeeded" || executed.Run.ID <= 0 || executed.Run.TaskType != intelligencedomain.TaskTypeTermExpansion ||
		!validIntentExpansionVersion(executed.Run.ModelVersion) || len(executed.Result) == 0 {
		return nil, ErrIntentExpansionProcessorUnavailable
	}
	output, err := decodeIntentExpansionOutput(executed.Result)
	if err != nil {
		return nil, err
	}
	return mapIntentExpansionCandidates(task, prepared.Expansion.Draft, executed.Run.ModelVersion, output)
}

type intentExpansionInputRecord struct {
	Objective          string                         `json:"objective"`
	Clauses            []intentExpansionClauseRecord  `json:"clauses"`
	Entities           []intentExpansionEntityRecord  `json:"entities"`
	Examples           []intentExpansionExampleRecord `json:"examples"`
	ExistingCandidates []string                       `json:"existing_candidates"`
	OutputLanguages    []string                       `json:"output_languages"`
}

type intentExpansionClauseRecord struct {
	Operator string `json:"operator"`
	Field    string `json:"field"`
	Value    string `json:"value"`
}

type intentExpansionEntityRecord struct {
	CanonicalID   string   `json:"canonical_id"`
	DisplayName   string   `json:"display_name"`
	Aliases       []string `json:"aliases"`
	AmbiguityNote string   `json:"ambiguity_note"`
}

type intentExpansionExampleRecord struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

type intentExpansionOutputRecord struct {
	Terms []intentExpansionSuggestionRecord `json:"terms"`
}

type intentExpansionSuggestionRecord struct {
	Term       string  `json:"term"`
	Language   string  `json:"language"`
	Reason     string  `json:"reason"`
	Similarity float64 `json:"similarity"`
	Risk       string  `json:"risk"`
}

func intentExpansionModelInput(draft monitorapplication.IntentDraftDTO) (intentExpansionInputRecord, error) {
	if draft.MonitorID <= 0 || draft.DraftID <= 0 || draft.ResourceVersion <= 0 {
		return intentExpansionInputRecord{}, ErrIntentExpansionOutputInvalid
	}
	input := intentExpansionInputRecord{
		Objective:          draft.Objective,
		Clauses:            make([]intentExpansionClauseRecord, 0, len(draft.Clauses)),
		Entities:           make([]intentExpansionEntityRecord, 0, len(draft.Entities)),
		Examples:           make([]intentExpansionExampleRecord, 0, len(draft.Examples)),
		ExistingCandidates: make([]string, 0, len(draft.Candidates)),
	}
	languages := make(map[string]struct{}, 3)
	for _, clause := range draft.Clauses {
		input.Clauses = append(input.Clauses, intentExpansionClauseRecord{Operator: clause.Operator, Field: clause.Field, Value: clause.Value})
		if clause.Field == "language" && clause.Operator != "must_not" {
			languages[intentExpansionPrimaryLanguage(clause.Value)] = struct{}{}
		}
	}
	for _, entity := range draft.Entities {
		input.Entities = append(input.Entities, intentExpansionEntityRecord{
			CanonicalID: entity.CanonicalID, DisplayName: entity.DisplayName,
			Aliases: append([]string{}, entity.Aliases...), AmbiguityNote: entity.AmbiguityNote,
		})
	}
	for _, example := range draft.Examples {
		input.Examples = append(input.Examples, intentExpansionExampleRecord{Label: example.Label, Text: example.Text})
	}
	for _, candidate := range draft.Candidates {
		input.ExistingCandidates = append(input.ExistingCandidates, candidate.Value)
	}
	if len(languages) == 0 {
		languages["und"] = struct{}{}
	}
	input.OutputLanguages = make([]string, 0, len(languages))
	for language := range languages {
		input.OutputLanguages = append(input.OutputLanguages, language)
	}
	sort.Strings(input.OutputLanguages)
	return input, nil
}

func intentExpansionPrimaryLanguage(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch {
	case normalized == "zh" || strings.HasPrefix(normalized, "zh-"):
		return "zh"
	case normalized == "en" || strings.HasPrefix(normalized, "en-"):
		return "en"
	default:
		return "und"
	}
}

func decodeIntentExpansionOutput(payload []byte) (intentExpansionOutputRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var output intentExpansionOutputRecord
	if err := decoder.Decode(&output); err != nil {
		return intentExpansionOutputRecord{}, ErrIntentExpansionOutputInvalid
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF || len(output.Terms) == 0 || len(output.Terms) > maximumIntentExpansionTerms {
		return intentExpansionOutputRecord{}, ErrIntentExpansionOutputInvalid
	}
	return output, nil
}

func mapIntentExpansionCandidates(task monitorapplication.IntentAnalysisTaskDTO, draft monitorapplication.IntentDraftDTO, modelVersion string, output intentExpansionOutputRecord) ([]monitorapplication.ExpansionCandidateDTO, error) {
	existing, excluded := intentExpansionTermSets(draft)
	seen := make(map[string]struct{}, len(output.Terms))
	candidates := make([]monitorapplication.ExpansionCandidateDTO, 0, len(output.Terms))
	for _, suggestion := range output.Terms {
		term, key, err := normalizeIntentExpansionTerm(suggestion.Term)
		if err != nil || !validIntentExpansionLanguage(suggestion.Language) || !validIntentExpansionAssessment(suggestion.Similarity, suggestion.Risk) {
			return nil, ErrIntentExpansionOutputInvalid
		}
		reason, err := normalizeIntentExpansionReason(suggestion.Reason)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		if _, duplicate := existing[key]; duplicate || intentExpansionConflicts(key, excluded) {
			continue
		}
		candidates = append(candidates, monitorapplication.ExpansionCandidateDTO{
			ID: deterministicIntentExpansionCandidateID(task.Run.RunID, term), Value: term,
			Source: "llm", Reason: reason, ModelVersion: modelVersion,
			PromptVersion: IntentExpansionPromptVersion, InputHash: task.Run.InputHash,
			Similarity: suggestion.Similarity, Risk: suggestion.Risk, ApprovalStatus: "pending",
		})
	}
	if len(candidates) == 0 {
		return nil, ErrIntentExpansionOutputInvalid
	}
	return candidates, nil
}

func intentExpansionTermSets(draft monitorapplication.IntentDraftDTO) (map[string]struct{}, []string) {
	existing := make(map[string]struct{}, len(draft.Clauses)+len(draft.Entities)+len(draft.Candidates))
	excluded := make([]string, 0)
	add := func(value string) string {
		_, key, err := normalizeIntentExpansionTerm(value)
		if err == nil {
			existing[key] = struct{}{}
			return key
		}
		return ""
	}
	for _, clause := range draft.Clauses {
		key := add(clause.Value)
		if clause.Operator == "must_not" && key != "" {
			excluded = append(excluded, key)
		}
	}
	for _, entity := range draft.Entities {
		add(entity.DisplayName)
		for _, alias := range entity.Aliases {
			add(alias)
		}
	}
	for _, candidate := range draft.Candidates {
		add(candidate.Value)
	}
	sort.Strings(excluded)
	return existing, excluded
}

func intentExpansionConflicts(candidate string, excluded []string) bool {
	for _, term := range excluded {
		if candidate == term || strings.Contains(candidate, term) || strings.Contains(term, candidate) {
			return true
		}
	}
	return false
}

func normalizeIntentExpansionTerm(value string) (string, string, error) {
	normalized := norm.NFC.String(strings.TrimSpace(value))
	for _, character := range normalized {
		if unicode.IsControl(character) {
			return "", "", ErrIntentExpansionOutputInvalid
		}
	}
	normalized = strings.Join(strings.Fields(normalized), " ")
	if normalized == "" || utf8.RuneCountInString(normalized) > maximumIntentExpansionTermRunes {
		return "", "", ErrIntentExpansionOutputInvalid
	}
	return normalized, strings.ToLower(normalized), nil
}

func normalizeIntentExpansionReason(value string) (string, error) {
	normalized := norm.NFC.String(strings.TrimSpace(value))
	if normalized == "" || utf8.RuneCountInString(normalized) > 1000 || strings.Contains(normalized, "%") || probabilityOrFactReasonPattern.MatchString(normalized) {
		return "", ErrIntentExpansionOutputInvalid
	}
	for _, forbidden := range []string{"概率", "可能性", "置信度", "可信", "不可信", "已证实", "已确认", "事实", "真假", "真伪"} {
		if strings.Contains(normalized, forbidden) {
			return "", ErrIntentExpansionOutputInvalid
		}
	}
	for _, character := range normalized {
		if unicode.IsControl(character) {
			return "", ErrIntentExpansionOutputInvalid
		}
	}
	return normalized, nil
}

func validIntentExpansionLanguage(value string) bool {
	return value == "zh" || value == "en" || value == "und"
}

func validIntentExpansionAssessment(similarity float64, risk string) bool {
	return !math.IsNaN(similarity) && !math.IsInf(similarity, 0) && similarity >= 0 && similarity <= 1 &&
		(risk == "low" || risk == "medium" || risk == "high")
}

func validIntentExpansionVersion(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len([]byte(value)) <= 128 && !strings.ContainsAny(value, "\x00\r\n")
}

func deterministicIntentExpansionCandidateID(runID int64, normalizedTerm string) string {
	_, key, err := normalizeIntentExpansionTerm(normalizedTerm)
	if runID <= 0 || err != nil {
		return ""
	}
	digest := sha256.New()
	for _, part := range []string{"monitor-intent-expansion-candidate-v1", strconv.FormatInt(runID, 10), key} {
		_, _ = fmt.Fprintf(digest, "%d:%s\n", len([]byte(part)), part)
	}
	return "expansion-" + hex.EncodeToString(digest.Sum(nil))
}
