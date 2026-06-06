package adapter

import (
	"sync"
	"time"
)

// SimulatorConfig configures a Simulator adapter for testing.
type SimulatorConfig struct {
	Provider    Provider
	Name        string
	Items       []NormalizedItem
	Capabilities Capabilities
	CollectErr  error
	CollectFn   func(CollectInput) (CollectOutput, error)
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
	defer s.mu.Unlock()

	s.lastCall = &input

	if s.config.CollectFn != nil {
		output, err := s.config.CollectFn(input)
		s.updateHealth(err)
		return output, err
	}

	if s.config.CollectErr != nil {
		s.updateHealth(s.config.CollectErr)
		return CollectOutput{}, s.config.CollectErr
	}

	s.updateHealth(nil)
	return CollectOutput{
		Items: s.config.Items,
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
