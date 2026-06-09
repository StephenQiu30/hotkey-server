package eventsummary

import (
	"encoding/json"
	"fmt"
	"strings"
)

// llmResponse mirrors the JSON structure returned by the LLM.
type llmResponse struct {
	Title      string          `json:"title"`
	Summary    string          `json:"summary"`
	Timeline   []TimelineEntry `json:"timeline"`
	KeySignals []string        `json:"key_signals"`
	RiskAlerts []string        `json:"risk_alerts"`
	FollowUp   []string        `json:"follow_up"`
}

// ParseLLMResponse parses raw JSON bytes from the LLM into an llmResponse.
// It strips markdown code fences if present.
func ParseLLMResponse(data []byte) (llmResponse, error) {
	s := strings.TrimSpace(string(data))
	s = stripMarkdownFence(s)

	var resp llmResponse
	if err := json.Unmarshal([]byte(s), &resp); err != nil {
		return llmResponse{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if resp.Title == "" {
		return llmResponse{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	return resp, nil
}

// stripMarkdownFence removes leading/trailing ```json ... ``` wrappers.
func stripMarkdownFence(s string) string {
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
