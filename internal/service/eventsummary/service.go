package eventsummary

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const defaultMaxRetries = 1

// Service orchestrates AI event summary generation.
type Service struct {
	repo       SummaryRepository
	llm        QwenClient
	now        func() time.Time
	maxRetries int
}

// NewService creates a new summary Service.
func NewService(repo SummaryRepository, llm QwenClient) *Service {
	return &Service{
		repo:       repo,
		llm:        llm,
		now:        time.Now,
		maxRetries: defaultMaxRetries,
	}
}

// SetMaxRetries overrides the default retry count.
func (s *Service) SetMaxRetries(n int) {
	s.maxRetries = n
}

// GetSummary retrieves an existing event summary by event ID.
func (s *Service) GetSummary(ctx context.Context, eventID string) (EventSummary, error) {
	if eventID == "" {
		return EventSummary{}, fmt.Errorf("%w: eventID is required", ErrInvalidInput)
	}
	return s.repo.FindByEventID(ctx, eventID)
}

// GenerateSummary generates or refreshes an AI summary for the given event.

// GenerateSummary generates or refreshes an AI summary for the given event.
func (s *Service) GenerateSummary(ctx context.Context, input GenerateSummaryInput) (EventSummary, error) {
	if input.EventID == "" {
		return EventSummary{}, fmt.Errorf("%w: eventID is required", ErrInvalidInput)
	}

	// Check for existing summary (idempotent refresh).
	existing, err := s.repo.FindByEventID(ctx, input.EventID)
	isRefresh := err == nil

	// Build prompt and call LLM with retries.
	prompt := buildPrompt(input)
	llmResp, llmErr := s.callLLMWithRetry(ctx, prompt)

	now := s.now()
	sourceCount := len(input.Items)

	var summary EventSummary

	if llmErr != nil {
		// Model failure: create/update with failed status.
		summary = s.buildFailedSummary(input, existing, isRefresh, llmErr, now)
	} else {
		// Success: build summary from LLM response.
		summary = s.buildSuccessSummary(input, existing, isRefresh, llmResp, sourceCount, now)
	}

	return s.repo.Save(ctx, summary)
}

func (s *Service) callLLMWithRetry(ctx context.Context, prompt string) (string, error) {
	if s.llm == nil {
		return "", fmt.Errorf("LLM client is not configured")
	}
	var lastErr error
	for i := 0; i < s.maxRetries; i++ {
		resp, err := s.llm.GenerateReport(ctx, prompt)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func (s *Service) buildFailedSummary(input GenerateSummaryInput, existing EventSummary, isRefresh bool, llmErr error, now time.Time) EventSummary {
	id := newID("es")
	version := 1
	createdAt := now
	if isRefresh {
		id = existing.ID
		version = existing.Version + 1
		createdAt = existing.CreatedAt
	}
	return EventSummary{
		ID:            id,
		EventID:       input.EventID,
		PromptVersion: PromptVersion,
		Title:         input.Title,
		ModelStatus:   ModelStatusFailed,
		LastError:     llmErr.Error(),
		Version:       version,
		SourceRefs:    buildSourceRefs(input.Items),
		CreatedAt:     createdAt,
		UpdatedAt:     now,
	}
}

func (s *Service) buildSuccessSummary(input GenerateSummaryInput, existing EventSummary, isRefresh bool, llmRespRaw string, sourceCount int, now time.Time) EventSummary {
	resp, err := ParseLLMResponse([]byte(llmRespRaw))
	if err != nil {
		// Parse failure treated as model failure.
		return s.buildFailedSummary(input, existing, isRefresh, err, now)
	}

	lowEvidence := IsLowEvidence(sourceCount)
	maxConf := MaxConfidence(sourceCount)
	status := ModelStatusSucceeded
	if lowEvidence {
		status = ModelStatusDegraded
	}

	id := newID("es")
	version := 1
	createdAt := now
	if isRefresh {
		id = existing.ID
		version = existing.Version + 1
		createdAt = existing.CreatedAt
	}

	return EventSummary{
		ID:            id,
		EventID:       input.EventID,
		PromptVersion: PromptVersion,
		Title:         resp.Title,
		Summary:       resp.Summary,
		Timeline:      resp.Timeline,
		KeySignals:    resp.KeySignals,
		SourceRefs:    buildSourceRefs(input.Items),
		RiskAlerts:    resp.RiskAlerts,
		FollowUp:      resp.FollowUp,
		Confidence:    maxConf,
		ModelStatus:   status,
		Version:       version,
		LowEvidence:   lowEvidence,
		CreatedAt:     createdAt,
		UpdatedAt:     now,
	}
}

func buildSourceRefs(items []ItemInfo) []SourceRef {
	refs := make([]SourceRef, len(items))
	for i, item := range items {
		refs[i] = SourceRef{
			SourceID: item.SourceID,
			ItemID:   item.ID,
			Title:    item.Title,
			URL:      item.URL,
		}
	}
	return refs
}

func buildPrompt(input GenerateSummaryInput) string {
	p := fmt.Sprintf("请为以下事件生成摘要，事件标题：%s\n\n来源内容：\n", input.Title)
	for i, item := range input.Items {
		p += fmt.Sprintf("%d. %s: %s\n", i+1, item.Title, item.Snippet)
	}
	p += "\n请以 JSON 格式输出，包含 title、summary、timeline、key_signals、risk_alerts、follow_up 字段。"
	return p
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}
