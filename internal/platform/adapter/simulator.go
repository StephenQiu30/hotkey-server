package adapter

import (
	"sync"
	"time"
)

// SimulatorConfig configures a Simulator adapter for testing.
type SimulatorConfig struct {
	Provider     Provider
	Name         string
	Items        []NormalizedItem
	Capabilities Capabilities
	CollectErr   error
	CollectFn    func(CollectInput) (CollectOutput, error)
}

// Simulator is a test adapter that returns pre-configured results.
type Simulator struct {
	config   SimulatorConfig
	health   HealthInfo
	mu       sync.Mutex
	lastCall *CollectInput
}

// NewSimulator creates a new Simulator adapter.
func NewSimulator(config SimulatorConfig) *Simulator {
	if config.Items == nil {
		config.Items = []NormalizedItem{}
	}
	return &Simulator{
		config: config,
		health: HealthInfo{
			Status:        HealthStatusHealthy,
			LastCheckedAt: time.Now().UTC(),
		},
	}
}

func (s *Simulator) Name() string {
	return s.config.Name
}

func (s *Simulator) Provider() Provider {
	return s.config.Provider
}

func (s *Simulator) Collect(input CollectInput) (CollectOutput, error) {
	s.mu.Lock()
	s.lastCall = &input
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

	s.mu.Lock()
	s.updateHealth(nil)
	s.mu.Unlock()
	for i := range items {
		if items[i].IdempotencyKey == "" {
			keySource := items[i].URL
			if keySource == "" {
				keySource = items[i].ExternalID
			}
			items[i].IdempotencyKey = NewIdempotencyKey(input.SourceID, keySource)
		}
	}
	return CollectOutput{
		Items: items,
	}, nil
}

func (s *Simulator) Health() HealthInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.health
}

func (s *Simulator) Capabilities() Capabilities {
	return s.config.Capabilities
}

func (s *Simulator) updateHealth(err error) {
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
