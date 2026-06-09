package adapter

import (
	"strings"
	"sync"
	"time"
)

const (
	defaultZhihuName            = "zhihu-simulator"
	defaultZhihuMaxSnippetChars = 500
)

// ZhihuSimulatorConfig configures a ZhihuSimulator adapter for testing.
type ZhihuSimulatorConfig struct {
	Name            string
	Items           []NormalizedItem
	Capabilities    Capabilities
	CollectErr      error
	CollectFn       func(CollectInput) (CollectOutput, error)
	MaxSnippetChars int
}

// ZhihuSimulator is a test adapter that returns pre-configured Zhihu content
// with long-text truncation and metadata enrichment.
type ZhihuSimulator struct {
	config ZhihuSimulatorConfig
	health HealthInfo
	mu     sync.Mutex
}

// NewZhihuSimulator creates a new ZhihuSimulator adapter.
func NewZhihuSimulator(config ZhihuSimulatorConfig) *ZhihuSimulator {
	if config.Name == "" {
		config.Name = defaultZhihuName
	}
	if config.Items == nil {
		config.Items = []NormalizedItem{}
	}
	if config.MaxSnippetChars <= 0 {
		config.MaxSnippetChars = defaultZhihuMaxSnippetChars
	}
	return &ZhihuSimulator{
		config: config,
		health: HealthInfo{
			Status:        HealthStatusHealthy,
			LastCheckedAt: time.Now().UTC(),
		},
	}
}

func (s *ZhihuSimulator) Name() string {
	return s.config.Name
}

func (s *ZhihuSimulator) Provider() Provider {
	return ProviderZhihu
}

func (s *ZhihuSimulator) Collect(input CollectInput) (CollectOutput, error) {
	s.mu.Lock()
	collectFn := s.config.CollectFn
	collectErr := s.config.CollectErr
	maxChars := s.config.MaxSnippetChars
	items := make([]NormalizedItem, len(s.config.Items))
	copy(items, s.config.Items)
	s.mu.Unlock()

	if collectFn != nil {
		output, err := collectFn(input)
		s.mu.Lock()
		s.updateHealth(err)
		s.mu.Unlock()
		return output, err
	}

	if collectErr != nil {
		s.mu.Lock()
		s.updateHealth(collectErr)
		s.mu.Unlock()
		return CollectOutput{}, collectErr
	}

	// Deduplicate by URL
	seen := make(map[string]struct{})
	deduped := make([]NormalizedItem, 0, len(items))
	for _, item := range items {
		if _, exists := seen[item.URL]; exists {
			continue
		}
		seen[item.URL] = struct{}{}

		// Truncate long text
		if len(item.Snippet) > maxChars {
			item.Snippet = item.Snippet[:maxChars] + "..."
			if item.Metadata == nil {
				item.Metadata = make(map[string]string)
			}
			item.Metadata["needs_summary"] = "true"
		}

		// Generate idempotency key if not set
		if item.IdempotencyKey == "" {
			keySource := item.URL
			if keySource == "" {
				keySource = item.ExternalID
			}
			item.IdempotencyKey = NewIdempotencyKey(input.SourceID, keySource)
		}

		// Normalize snippet whitespace
		item.Snippet = strings.TrimSpace(item.Snippet)

		deduped = append(deduped, item)
	}

	s.mu.Lock()
	s.updateHealth(nil)
	s.mu.Unlock()

	return CollectOutput{
		Items: deduped,
	}, nil
}

func (s *ZhihuSimulator) Health() HealthInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.health
}

func (s *ZhihuSimulator) Capabilities() Capabilities {
	return s.config.Capabilities
}

func (s *ZhihuSimulator) updateHealth(err error) {
	if err != nil {
		s.health = HealthInfo{
			Status:        HealthStatusDegraded,
			LastError:     err.Error(),
			LastCheckedAt: time.Now().UTC(),
		}
	} else {
		s.health = HealthInfo{
			Status:        HealthStatusHealthy,
			LastCheckedAt: time.Now().UTC(),
		}
	}
}
