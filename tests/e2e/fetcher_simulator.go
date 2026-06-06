package e2e_test

import (
	"context"
	"fmt"
	"sync"
)

// fetcherSimulator implements FetcherSimulator with five configurable behaviors.
type fetcherSimulator struct {
	mu       sync.Mutex
	behavior ProviderBehavior
}

func newFetcherSimulatorImpl() *fetcherSimulator {
	return &fetcherSimulator{behavior: BehaviorNormal}
}

func (s *fetcherSimulator) SetBehavior(b ProviderBehavior) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.behavior = b
}

func (s *fetcherSimulator) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.behavior = BehaviorNormal
}

func (s *fetcherSimulator) behaviorValue() ProviderBehavior {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.behavior
}

func (s *fetcherSimulator) Fetch(ctx context.Context, sourceURL string) ([]map[string]string, error) {
	b := s.behaviorValue()
	switch b {
	case BehaviorNormal:
		return s.normalFetch(sourceURL), nil
	case BehaviorRateLimit:
		return nil, NewSimulatorError("rate_limit", "simulated rate limit: too many requests")
	case BehaviorAuthInvalid:
		return nil, NewSimulatorError("auth_invalid", "simulated auth failure: invalid API key")
	case BehaviorSchemaChange:
		return nil, NewSimulatorError("schema_change", "simulated schema change: feed format unexpected")
	case BehaviorEmptyResult:
		return nil, NewSimulatorError("empty_result", "simulated empty result: no items in feed")
	default:
		return nil, fmt.Errorf("unknown behavior: %s", b)
	}
}

// normalFetch returns deterministic RSS-like feed items.
func (s *fetcherSimulator) normalFetch(sourceURL string) []map[string]string {
	return []map[string]string{
		{
			"title":       "AI 技术突破：新一代大模型性能提升 50%",
			"link":        "https://example.com/article/1",
			"description": "多家科技公司发布最新 AI 模型，性能大幅提升。",
			"pubDate":     "2026-06-07T08:00:00Z",
		},
		{
			"title":       "全球经济展望：数字化转型加速",
			"link":        "https://example.com/article/2",
			"description": "报告显示全球企业数字化转型投入同比增长 30%。",
			"pubDate":     "2026-06-07T09:00:00Z",
		},
		{
			"title":       "开源社区年度报告发布",
			"link":        "https://example.com/article/3",
			"description": "GitHub 年度报告揭示开源项目增长趋势。",
			"pubDate":     "2026-06-07T10:00:00Z",
		},
	}
}
