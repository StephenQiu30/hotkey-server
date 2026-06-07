package adapter

import (
	"strings"
	"sync"
	"time"
)

const defaultBiliBiliName = "bilibili-simulator"

// BiliBiliSimulatorConfig configures a BiliBiliSimulator adapter for testing.
type BiliBiliSimulatorConfig struct {
	Name         string
	Items        []NormalizedItem
	Capabilities Capabilities
	CollectErr   error
	CollectFn    func(CollectInput) (CollectOutput, error)
}

// BiliBiliSimulator is a test adapter that returns pre-configured Bilibili content
// with BVID deduplication, empty-title filtering, and subtitle-fallback simulation.
type BiliBiliSimulator struct {
	config BiliBiliSimulatorConfig
	health HealthInfo
	mu     sync.Mutex
}

// NewBiliBiliSimulator creates a new BiliBiliSimulator adapter.
func NewBiliBiliSimulator(config BiliBiliSimulatorConfig) *BiliBiliSimulator {
	if config.Name == "" {
		config.Name = defaultBiliBiliName
	}
	if config.Items == nil {
		config.Items = []NormalizedItem{}
	}
	return &BiliBiliSimulator{
		config: config,
		health: HealthInfo{
			Status:        HealthStatusHealthy,
			LastCheckedAt: time.Now().UTC(),
		},
	}
}

func (s *BiliBiliSimulator) Name() string {
	return s.config.Name
}

func (s *BiliBiliSimulator) Provider() Provider {
	return ProviderBiliBili
}

func (s *BiliBiliSimulator) Collect(input CollectInput) (CollectOutput, error) {
	s.mu.Lock()
	collectFn := s.config.CollectFn
	collectErr := s.config.CollectErr
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

	// Deduplicate by ExternalID (BVID for videos, dynamic ID for dynamics)
	seen := make(map[string]struct{})
	deduped := make([]NormalizedItem, 0, len(items))
	for _, item := range items {
		// Filter empty titles
		if strings.TrimSpace(item.Title) == "" {
			continue
		}

		dedupKey := item.ExternalID
		if dedupKey == "" {
			dedupKey = item.URL
		}
		if _, exists := seen[dedupKey]; exists {
			continue
		}
		seen[dedupKey] = struct{}{}

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

func (s *BiliBiliSimulator) Health() HealthInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.health
}

func (s *BiliBiliSimulator) Capabilities() Capabilities {
	return s.config.Capabilities
}

func (s *BiliBiliSimulator) updateHealth(err error) {
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
