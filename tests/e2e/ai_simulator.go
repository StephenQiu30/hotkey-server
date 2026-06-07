package e2e_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
	"sync"
)

// aiSimulator implements AIProviderSimulator with five configurable behaviors.
type aiSimulator struct {
	mu       sync.Mutex
	behavior ProviderBehavior
}

// newAISimulatorImpl creates a new instance of aiSimulator with default behavior.
func newAISimulatorImpl() *aiSimulator {
	return &aiSimulator{behavior: BehaviorNormal}
}

// SetBehavior updates the current simulator behavior.
func (s *aiSimulator) SetBehavior(b ProviderBehavior) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.behavior = b
}

// Reset restores the simulator to its default BehaviorNormal.
func (s *aiSimulator) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.behavior = BehaviorNormal
}

// behaviorValue returns the current behavior in a thread-safe manner.
func (s *aiSimulator) behaviorValue() ProviderBehavior {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.behavior
}

// Embed simulates generating a vector embedding for the given text.
func (s *aiSimulator) Embed(ctx context.Context, text string) ([]float64, error) {
	b := s.behaviorValue()
	switch b {
	case BehaviorNormal:
		return s.normalEmbed(text), nil
	case BehaviorRateLimit:
		return nil, NewSimulatorError("rate_limit", "simulated rate limit: too many requests")
	case BehaviorAuthInvalid:
		return nil, NewSimulatorError("auth_invalid", "simulated auth failure: invalid API key")
	case BehaviorSchemaChange:
		return nil, NewSimulatorError("schema_change", "simulated schema change: response format unexpected")
	case BehaviorEmptyResult:
		return nil, NewSimulatorError("empty_result", "simulated empty result: no embeddings returned")
	default:
		return nil, fmt.Errorf("unknown behavior: %s", b)
	}
}

// Chat simulates an AI chat interaction for generating responses or reports.
func (s *aiSimulator) Chat(ctx context.Context, prompt string) (string, error) {
	b := s.behaviorValue()
	switch b {
	case BehaviorNormal:
		return s.normalChat(prompt), nil
	case BehaviorRateLimit:
		return "", NewSimulatorError("rate_limit", "simulated rate limit: too many requests")
	case BehaviorAuthInvalid:
		return "", NewSimulatorError("auth_invalid", "simulated auth failure: invalid API key")
	case BehaviorSchemaChange:
		return "", NewSimulatorError("schema_change", "simulated schema change: response format unexpected")
	case BehaviorEmptyResult:
		return "", NewSimulatorError("empty_result", "simulated empty result: no choices returned")
	default:
		return "", fmt.Errorf("unknown behavior: %s", b)
	}
}

const EmbeddingDimension = 1536

// normalEmbed generates a deterministic 1536-dim vector from text hash.
func (s *aiSimulator) normalEmbed(text string) []float64 {
	hash := deterministicHash(text)
	vec := make([]float64, EmbeddingDimension)
	for i := range vec {
		// Cycle over hash bytes to fill the larger vector
		vec[i] = float64(hash[i%len(hash)]) / 255.0
	}
	// Normalize to unit vector
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}

// normalChat generates a deterministic Chinese response.
func (s *aiSimulator) normalChat(prompt string) string {
	if strings.Contains(prompt, "日报") || strings.Contains(prompt, "report") {
		return "今日热点摘要：AI 技术持续发展，多个行业加速智能化转型。"
	}
	return "模拟 AI 响应：已处理请求。"
}

// deterministicHash returns a 32-byte hash for simulation (uses crypto/sha256).
func deterministicHash(text string) []byte {
	h := sha256.Sum256([]byte(text))
	return h[:]
}
