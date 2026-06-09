package eventsummary

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// --- Schema Validation Tests ---

func TestEventSummarySchemaValid(t *testing.T) {
	summary := EventSummary{
		ID:            "es-1",
		EventID:       "evt-1",
		PromptVersion: PromptVersion,
		Title:         "AI 创作工具重大更新",
		Summary:       "多家公司发布新一代 AI 创作工具，创作者工作流面临重大变革。",
		Timeline: []TimelineEntry{
			{Date: "2026-06-05", Description: "公司 A 发布新工具"},
			{Date: "2026-06-06", Description: "创作者社区开始试用"},
		},
		KeySignals:  []string{"工具更新", "工作流变革"},
		SourceRefs:  []SourceRef{{SourceID: "src-1", ItemID: "item-1", Title: "来源一", URL: "https://example.test/1"}},
		RiskAlerts:  []string{"技术壁垒上升"},
		FollowUp:    []string{"关注用户反馈"},
		Confidence:  0.85,
		ModelStatus: ModelStatusSucceeded,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := ValidateSummary(summary); err != nil {
		t.Fatalf("expected valid summary, got error: %v", err)
	}
}

func TestEventSummarySchemaMissingTitle(t *testing.T) {
	summary := EventSummary{
		ID:          "es-1",
		EventID:     "evt-1",
		Summary:     "摘要内容",
		ModelStatus: ModelStatusSucceeded,
		Confidence:  0.8,
	}
	if err := ValidateSummary(summary); err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestEventSummarySchemaMissingSummary(t *testing.T) {
	summary := EventSummary{
		ID:          "es-1",
		EventID:     "evt-1",
		Title:       "标题",
		ModelStatus: ModelStatusSucceeded,
		Confidence:  0.8,
	}
	if err := ValidateSummary(summary); err == nil {
		t.Fatal("expected error for missing summary")
	}
}

func TestEventSummarySchemaConfidenceRange(t *testing.T) {
	summary := EventSummary{
		ID:          "es-1",
		EventID:     "evt-1",
		Title:       "标题",
		Summary:     "摘要",
		Confidence:  1.5,
		ModelStatus: ModelStatusSucceeded,
	}
	if err := ValidateSummary(summary); err == nil {
		t.Fatal("expected error for confidence > 1.0")
	}

	summary.Confidence = -0.1
	if err := ValidateSummary(summary); err == nil {
		t.Fatal("expected error for confidence < 0")
	}
}

// --- LLM Response Parsing Tests ---

func TestParseLLMResponseValidJSON(t *testing.T) {
	input := `{
		"title": "AI 创作工具重大更新",
		"summary": "多家公司发布新一代 AI 创作工具。",
		"timeline": [
			{"date": "2026-06-05", "description": "公司 A 发布新工具"}
		],
		"key_signals": ["工具更新", "工作流变革"],
		"risk_alerts": ["技术壁垒上升"],
		"follow_up": ["关注用户反馈"]
	}`
	result, err := ParseLLMResponse([]byte(input))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Title != "AI 创作工具重大更新" {
		t.Fatalf("expected title 'AI 创作工具重大更新', got: %s", result.Title)
	}
	if len(result.Timeline) != 1 {
		t.Fatalf("expected 1 timeline entry, got: %d", len(result.Timeline))
	}
	if len(result.KeySignals) != 2 {
		t.Fatalf("expected 2 key signals, got: %d", len(result.KeySignals))
	}
}

func TestParseLLMResponseInvalidJSON(t *testing.T) {
	input := `not json at all`
	_, err := ParseLLMResponse([]byte(input))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseLLMResponseMissingRequiredFields(t *testing.T) {
	input := `{"summary": "只有摘要"}`
	_, err := ParseLLMResponse([]byte(input))
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestParseLLMResponseWithMarkdownWrapper(t *testing.T) {
	input := "```json\n{\"title\": \"标题\", \"summary\": \"摘要\", \"timeline\": [], \"key_signals\": [], \"risk_alerts\": [], \"follow_up\": []}\n```"
	result, err := ParseLLMResponse([]byte(input))
	if err != nil {
		t.Fatalf("expected no error for markdown-wrapped JSON, got: %v", err)
	}
	if result.Title != "标题" {
		t.Fatalf("expected title '标题', got: %s", result.Title)
	}
}

// --- Low Evidence Tests ---

func TestLowEvidenceFlagging(t *testing.T) {
	tests := []struct {
		name        string
		sourceCount int
		wantLow     bool
	}{
		{"single source is low evidence", 1, true},
		{"two sources is low evidence", 2, true},
		{"three sources is sufficient", 3, false},
		{"five sources is sufficient", 5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsLowEvidence(tt.sourceCount)
			if got != tt.wantLow {
				t.Fatalf("IsLowEvidence(%d) = %v, want %v", tt.sourceCount, got, tt.wantLow)
			}
		})
	}
}

func TestLowEvidenceConfidenceCap(t *testing.T) {
	tests := []struct {
		name        string
		sourceCount int
		wantMaxConf float64
	}{
		{"single source caps at 0.3", 1, 0.3},
		{"two sources caps at 0.5", 2, 0.5},
		{"three sources no cap", 3, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaxConfidence(tt.sourceCount)
			if got != tt.wantMaxConf {
				t.Fatalf("MaxConfidence(%d) = %f, want %f", tt.sourceCount, got, tt.wantMaxConf)
			}
		})
	}
}

// --- Model Failure and Retry Tests ---

func TestModelStatusConstants(t *testing.T) {
	if ModelStatusPending != "pending" {
		t.Fatalf("expected pending, got %s", ModelStatusPending)
	}
	if ModelStatusSucceeded != "succeeded" {
		t.Fatalf("expected succeeded, got %s", ModelStatusSucceeded)
	}
	if ModelStatusFailed != "failed" {
		t.Fatalf("expected failed, got %s", ModelStatusFailed)
	}
	if ModelStatusDegraded != "degraded" {
		t.Fatalf("expected degraded, got %s", ModelStatusDegraded)
	}
}

func TestGenerateSummaryWithMockLLM(t *testing.T) {
	llm := &mockLLM{
		response: `{
			"title": "AI 创作工具重大更新",
			"summary": "多家公司发布新一代 AI 创作工具。",
			"timeline": [{"date": "2026-06-05", "description": "公司 A 发布新工具"}],
			"key_signals": ["工具更新"],
			"risk_alerts": ["技术壁垒"],
			"follow_up": ["关注反馈"]
		}`,
	}
	repo := NewMemoryRepository()
	svc := NewService(repo, llm)

	input := GenerateSummaryInput{
		EventID: "evt-1",
		Title:   "AI 工具更新",
		Items: []ItemInfo{
			{ID: "item-1", SourceID: "src-1", Title: "来源一", Snippet: "内容一", URL: "https://example.test/1"},
			{ID: "item-2", SourceID: "src-2", Title: "来源二", Snippet: "内容二", URL: "https://example.test/2"},
			{ID: "item-3", SourceID: "src-3", Title: "来源三", Snippet: "内容三", URL: "https://example.test/3"},
		},
	}

	summary, err := svc.GenerateSummary(context.Background(), input)
	if err != nil {
		t.Fatalf("GenerateSummary returned error: %v", err)
	}
	if summary.ModelStatus != ModelStatusSucceeded {
		t.Fatalf("expected succeeded, got %s", summary.ModelStatus)
	}
	if summary.Title != "AI 创作工具重大更新" {
		t.Fatalf("expected title from LLM, got: %s", summary.Title)
	}
	if len(summary.Timeline) != 1 {
		t.Fatalf("expected 1 timeline entry, got: %d", len(summary.Timeline))
	}
	if len(summary.SourceRefs) != 3 {
		t.Fatalf("expected 3 source refs, got: %d", len(summary.SourceRefs))
	}
	if summary.Confidence <= 0 || summary.Confidence > 1 {
		t.Fatalf("expected confidence in (0,1], got: %f", summary.Confidence)
	}
}

func TestGenerateSummaryLLMFailureSetsFailedStatus(t *testing.T) {
	llm := &mockLLM{err: errors.New("model timeout")}
	repo := NewMemoryRepository()
	svc := NewService(repo, llm)

	input := GenerateSummaryInput{
		EventID: "evt-1",
		Title:   "AI 工具更新",
		Items: []ItemInfo{
			{ID: "item-1", SourceID: "src-1", Title: "来源一", Snippet: "内容一", URL: "https://example.test/1"},
			{ID: "item-2", SourceID: "src-2", Title: "来源二", Snippet: "内容二", URL: "https://example.test/2"},
			{ID: "item-3", SourceID: "src-3", Title: "来源三", Snippet: "内容三", URL: "https://example.test/3"},
		},
	}

	summary, err := svc.GenerateSummary(context.Background(), input)
	if err != nil {
		t.Fatalf("GenerateSummary should not return error on model failure, got: %v", err)
	}
	if summary.ModelStatus != ModelStatusFailed {
		t.Fatalf("expected failed status, got %s", summary.ModelStatus)
	}
	if summary.LastError == "" {
		t.Fatal("expected LastError to be set")
	}
}

func TestGenerateSummaryLLMFailureRetries(t *testing.T) {
	callCount := 0
	llm := &mockLLM{
		responseFn: func(ctx context.Context, prompt string) (string, error) {
			callCount++
			if callCount < 3 {
				return "", errors.New("transient error")
			}
			return `{"title": "标题", "summary": "摘要", "timeline": [], "key_signals": [], "risk_alerts": [], "follow_up": []}`, nil
		},
	}
	repo := NewMemoryRepository()
	svc := NewService(repo, llm)
	svc.SetMaxRetries(3)

	input := GenerateSummaryInput{
		EventID: "evt-1",
		Title:   "AI 工具更新",
		Items: []ItemInfo{
			{ID: "item-1", SourceID: "src-1", Title: "来源一", Snippet: "内容一", URL: "https://example.test/1"},
			{ID: "item-2", SourceID: "src-2", Title: "来源二", Snippet: "内容二", URL: "https://example.test/2"},
			{ID: "item-3", SourceID: "src-3", Title: "来源三", Snippet: "内容三", URL: "https://example.test/3"},
		},
	}

	summary, err := svc.GenerateSummary(context.Background(), input)
	if err != nil {
		t.Fatalf("GenerateSummary returned error: %v", err)
	}
	if summary.ModelStatus != ModelStatusSucceeded {
		t.Fatalf("expected succeeded after retry, got %s", summary.ModelStatus)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls, got %d", callCount)
	}
}

func TestGenerateSummaryLowEvidenceSetsDegradedStatus(t *testing.T) {
	llm := &mockLLM{
		response: `{"title": "标题", "summary": "摘要", "timeline": [], "key_signals": [], "risk_alerts": [], "follow_up": []}`,
	}
	repo := NewMemoryRepository()
	svc := NewService(repo, llm)

	input := GenerateSummaryInput{
		EventID: "evt-1",
		Title:   "AI 工具更新",
		Items: []ItemInfo{
			{ID: "item-1", SourceID: "src-1", Title: "来源一", Snippet: "内容一", URL: "https://example.test/1"},
		},
	}

	summary, err := svc.GenerateSummary(context.Background(), input)
	if err != nil {
		t.Fatalf("GenerateSummary returned error: %v", err)
	}
	if summary.ModelStatus != ModelStatusDegraded {
		t.Fatalf("expected degraded for low evidence, got %s", summary.ModelStatus)
	}
	if summary.Confidence > 0.3 {
		t.Fatalf("expected confidence <= 0.3 for single source, got %f", summary.Confidence)
	}
}

// --- Idempotency / Refresh Tests ---

func TestGenerateSummaryIdempotentRefresh(t *testing.T) {
	llm := &mockLLM{
		response: `{"title": "标题", "summary": "摘要", "timeline": [], "key_signals": [], "risk_alerts": [], "follow_up": []}`,
	}
	repo := NewMemoryRepository()
	svc := NewService(repo, llm)

	input := GenerateSummaryInput{
		EventID: "evt-1",
		Title:   "AI 工具更新",
		Items: []ItemInfo{
			{ID: "item-1", SourceID: "src-1", Title: "来源一", Snippet: "内容一", URL: "https://example.test/1"},
			{ID: "item-2", SourceID: "src-2", Title: "来源二", Snippet: "内容二", URL: "https://example.test/2"},
			{ID: "item-3", SourceID: "src-3", Title: "来源三", Snippet: "内容三", URL: "https://example.test/3"},
		},
	}

	summary1, err := svc.GenerateSummary(context.Background(), input)
	if err != nil {
		t.Fatalf("first GenerateSummary error: %v", err)
	}

	summary2, err := svc.GenerateSummary(context.Background(), input)
	if err != nil {
		t.Fatalf("second GenerateSummary error: %v", err)
	}

	if summary1.ID != summary2.ID {
		t.Fatalf("expected same ID for refresh, got %s vs %s", summary1.ID, summary2.ID)
	}
	if summary2.Version != summary1.Version+1 {
		t.Fatalf("expected version increment, got %d vs %d", summary1.Version, summary2.Version)
	}
}

// --- Source Reference Tests ---

func TestSourceRefsPopulatedFromItems(t *testing.T) {
	llm := &mockLLM{
		response: `{"title": "标题", "summary": "摘要", "timeline": [], "key_signals": [], "risk_alerts": [], "follow_up": []}`,
	}
	repo := NewMemoryRepository()
	svc := NewService(repo, llm)

	input := GenerateSummaryInput{
		EventID: "evt-1",
		Title:   "AI 工具更新",
		Items: []ItemInfo{
			{ID: "item-1", SourceID: "src-1", Title: "来源一", Snippet: "内容一", URL: "https://example.test/1"},
			{ID: "item-2", SourceID: "src-2", Title: "来源二", Snippet: "内容二", URL: "https://example.test/2"},
			{ID: "item-3", SourceID: "src-3", Title: "来源三", Snippet: "内容三", URL: "https://example.test/3"},
		},
	}

	summary, err := svc.GenerateSummary(context.Background(), input)
	if err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}
	if len(summary.SourceRefs) != 3 {
		t.Fatalf("expected 3 source refs, got %d", len(summary.SourceRefs))
	}
	for i, ref := range summary.SourceRefs {
		if ref.SourceID == "" || ref.ItemID == "" || ref.URL == "" {
			t.Fatalf("source ref %d missing fields: %+v", i, ref)
		}
	}
}

// --- Prompt Construction Tests ---

func TestPromptIncludesEventTitleAndItems(t *testing.T) {
	llm := &mockLLM{
		response: `{"title": "标题", "summary": "摘要", "timeline": [], "key_signals": [], "risk_alerts": [], "follow_up": []}`,
	}
	repo := NewMemoryRepository()
	svc := NewService(repo, llm)

	input := GenerateSummaryInput{
		EventID: "evt-1",
		Title:   "AI 创作工具更新",
		Items: []ItemInfo{
			{ID: "item-1", SourceID: "src-1", Title: "来源一", Snippet: "内容一", URL: "https://example.test/1"},
			{ID: "item-2", SourceID: "src-2", Title: "来源二", Snippet: "内容二", URL: "https://example.test/2"},
			{ID: "item-3", SourceID: "src-3", Title: "来源三", Snippet: "内容三", URL: "https://example.test/3"},
		},
	}

	_, _ = svc.GenerateSummary(context.Background(), input)

	prompt := llm.lastPrompt
	if !strings.Contains(prompt, "AI 创作工具更新") {
		t.Fatal("prompt should contain event title")
	}
	if !strings.Contains(prompt, "来源一") {
		t.Fatal("prompt should contain item titles")
	}
	if !strings.Contains(prompt, "JSON") {
		t.Fatal("prompt should request JSON output")
	}
}

// --- JSON serialization roundtrip ---

func TestEventSummaryJSONRoundtrip(t *testing.T) {
	original := EventSummary{
		ID:            "es-1",
		EventID:       "evt-1",
		PromptVersion: PromptVersion,
		Title:         "AI 创作工具重大更新",
		Summary:       "多家公司发布新一代 AI 创作工具。",
		Timeline: []TimelineEntry{
			{Date: "2026-06-05", Description: "公司 A 发布新工具"},
		},
		KeySignals:  []string{"工具更新"},
		SourceRefs:  []SourceRef{{SourceID: "src-1", ItemID: "item-1", Title: "来源一", URL: "https://example.test/1"}},
		RiskAlerts:  []string{"技术壁垒"},
		FollowUp:    []string{"关注反馈"},
		Confidence:  0.85,
		ModelStatus: ModelStatusSucceeded,
		Version:     1,
		LowEvidence: false,
		CreatedAt:   time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded EventSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != original.ID {
		t.Fatalf("ID mismatch: %s vs %s", decoded.ID, original.ID)
	}
	if decoded.Title != original.Title {
		t.Fatalf("Title mismatch: %s vs %s", decoded.Title, original.Title)
	}
	if len(decoded.Timeline) != len(original.Timeline) {
		t.Fatalf("Timeline length mismatch: %d vs %d", len(decoded.Timeline), len(original.Timeline))
	}
	if decoded.Confidence != original.Confidence {
		t.Fatalf("Confidence mismatch: %f vs %f", decoded.Confidence, original.Confidence)
	}
}

// --- Mock ---

type mockLLM struct {
	response   string
	responseFn func(ctx context.Context, prompt string) (string, error)
	err        error
	lastPrompt string
}

func (m *mockLLM) GenerateReport(ctx context.Context, prompt string) (string, error) {
	m.lastPrompt = prompt
	if m.responseFn != nil {
		return m.responseFn(ctx, prompt)
	}
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}
