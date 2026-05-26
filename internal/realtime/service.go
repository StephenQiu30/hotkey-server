package realtime

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/event"
)

const (
	StatusAccepted = "accepted"
	StatusDegraded = "degraded"

	FallbackRateLimited = "rate_limited"
	FallbackCircuitOpen = "circuit_open"

	CircuitClosed = "closed"
	CircuitOpen   = "open"
)

var (
	ErrInvalidSource      = errors.New("invalid realtime source")
	ErrInvalidPush        = errors.New("invalid realtime push")
	ErrSourceNotFound     = errors.New("realtime source not found")
	ErrUnauthorizedSource = errors.New("unauthorized realtime source")
	ErrSourceDisabled     = errors.New("realtime source disabled")
	ErrRateLimited        = errors.New("realtime source rate limited")
	ErrCircuitOpen        = errors.New("realtime circuit open")
)

type Options struct {
	Now                func() time.Time
	RateLimitWindow    time.Duration
	MaxEventsPerWindow int
	FailureThreshold   int
}

type Source struct {
	ID            string `json:"id"`
	Token         string `json:"-"`
	Enabled       bool   `json:"enabled"`
	LowLatency    bool   `json:"lowLatency"`
	CircuitStatus string `json:"circuitStatus"`
	FailureCount  int    `json:"failureCount"`
}

type PushInput struct {
	SourceID     string    `json:"sourceId"`
	Token        string    `json:"token,omitempty"`
	SourceItemID string    `json:"sourceItemId"`
	Title        string    `json:"title"`
	ContentHash  string    `json:"contentHash"`
	ReceivedAt   time.Time `json:"receivedAt"`
	Vector       []float64 `json:"vector,omitempty"`
}

type PushResult struct {
	Status              string             `json:"status"`
	FallbackReason      string             `json:"fallbackReason,omitempty"`
	LatencyMilliseconds int64              `json:"latencyMilliseconds"`
	Match               event.ClusterMatch `json:"match,omitempty"`
}

type FallbackItem struct {
	SourceID       string    `json:"sourceId"`
	SourceItemID   string    `json:"sourceItemId"`
	Title          string    `json:"title"`
	ContentHash    string    `json:"contentHash"`
	Reason         string    `json:"reason"`
	QueuedAt       time.Time `json:"queuedAt"`
	OriginalSentAt time.Time `json:"originalSentAt"`
}

type rateRecord struct {
	timestamps []time.Time
}

type Service struct {
	mu           sync.Mutex
	options      Options
	eventService *event.Service
	sources      map[string]Source
	rates        map[string]rateRecord
	fallbacks    []FallbackItem
}

func NewService(eventService *event.Service, options Options) *Service {
	if eventService == nil {
		eventService = event.NewService(event.Options{VectorEnabled: true})
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.RateLimitWindow <= 0 {
		options.RateLimitWindow = time.Minute
	}
	if options.MaxEventsPerWindow <= 0 {
		options.MaxEventsPerWindow = 1
	}
	if options.FailureThreshold <= 0 {
		options.FailureThreshold = 3
	}
	return &Service{
		options:      options,
		eventService: eventService,
		sources:      make(map[string]Source),
		rates:        make(map[string]rateRecord),
	}
}

func (s *Service) RegisterSource(source Source) error {
	normalized, err := normalizeSource(source)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sources[normalized.ID] = normalized
	return nil
}

func (s *Service) GetSource(id string) Source {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSource(s.sources[strings.TrimSpace(id)])
}

func (s *Service) RecordFailure(sourceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.TrimSpace(sourceID)
	source := s.sources[id]
	if source.ID == "" {
		return
	}
	source.FailureCount++
	if source.FailureCount >= s.options.FailureThreshold {
		source.CircuitStatus = CircuitOpen
	}
	s.sources[id] = source
}

func (s *Service) AcceptPush(input PushInput) (PushResult, error) {
	normalized, err := normalizePush(input)
	if err != nil {
		return PushResult{}, err
	}

	now := s.options.Now().UTC()
	if normalized.ReceivedAt.IsZero() {
		normalized.ReceivedAt = now
	}

	if result, err := s.checkSourceAndRate(normalized, now); err != nil {
		return result, err
	}

	match, err := s.eventService.UpsertCandidate(event.CandidateInput{
		SourceItemID: normalized.SourceItemID,
		Title:        normalized.Title,
		ContentHash:  normalized.ContentHash,
		Vector:       normalized.Vector,
	})
	if err != nil {
		s.RecordFailure(normalized.SourceID)
		result := s.enqueueFallback(normalized, now, FallbackCircuitOpen)
		return result, err
	}

	return PushResult{
		Status:              StatusAccepted,
		LatencyMilliseconds: latencyMilliseconds(normalized.ReceivedAt, now),
		Match:               match,
	}, nil
}

func (s *Service) ListFallbacks() []FallbackItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]FallbackItem, len(s.fallbacks))
	copy(items, s.fallbacks)
	return items
}

func (s *Service) checkSourceAndRate(input PushInput, now time.Time) (PushResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	source, ok := s.sources[input.SourceID]
	if !ok {
		return PushResult{}, ErrSourceNotFound
	}
	if !source.Enabled {
		return PushResult{}, ErrSourceDisabled
	}
	if source.Token != input.Token {
		return PushResult{}, ErrUnauthorizedSource
	}
	if source.CircuitStatus == CircuitOpen {
		result := s.enqueueFallbackLocked(input, now, FallbackCircuitOpen)
		return result, ErrCircuitOpen
	}
	if !s.allowLocked(input.SourceID, now) {
		result := s.enqueueFallbackLocked(input, now, FallbackRateLimited)
		return result, ErrRateLimited
	}
	return PushResult{}, nil
}

func (s *Service) allowLocked(sourceID string, now time.Time) bool {
	record := s.rates[sourceID]
	cutoff := now.Add(-s.options.RateLimitWindow)
	active := record.timestamps[:0]
	for _, ts := range record.timestamps {
		if ts.After(cutoff) {
			active = append(active, ts)
		}
	}
	if len(active) >= s.options.MaxEventsPerWindow {
		s.rates[sourceID] = rateRecord{timestamps: active}
		return false
	}
	active = append(active, now)
	s.rates[sourceID] = rateRecord{timestamps: active}
	return true
}

func (s *Service) enqueueFallback(input PushInput, now time.Time, reason string) PushResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enqueueFallbackLocked(input, now, reason)
}

func (s *Service) enqueueFallbackLocked(input PushInput, now time.Time, reason string) PushResult {
	item := FallbackItem{
		SourceID:       input.SourceID,
		SourceItemID:   input.SourceItemID,
		Title:          input.Title,
		ContentHash:    input.ContentHash,
		Reason:         reason,
		QueuedAt:       now,
		OriginalSentAt: input.ReceivedAt,
	}
	s.fallbacks = append(s.fallbacks, item)
	return PushResult{
		Status:              StatusDegraded,
		FallbackReason:      reason,
		LatencyMilliseconds: latencyMilliseconds(input.ReceivedAt, now),
	}
}

func normalizeSource(source Source) (Source, error) {
	source.ID = strings.TrimSpace(source.ID)
	source.Token = strings.TrimSpace(source.Token)
	if source.ID == "" || source.Token == "" {
		return Source{}, ErrInvalidSource
	}
	if source.CircuitStatus == "" {
		source.CircuitStatus = CircuitClosed
	}
	if source.CircuitStatus != CircuitClosed && source.CircuitStatus != CircuitOpen {
		return Source{}, ErrInvalidSource
	}
	if !source.Enabled {
		source.Enabled = true
	}
	return source, nil
}

func normalizePush(input PushInput) (PushInput, error) {
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.Token = strings.TrimSpace(input.Token)
	input.SourceItemID = strings.TrimSpace(input.SourceItemID)
	input.Title = strings.Join(strings.Fields(input.Title), " ")
	input.ContentHash = strings.TrimSpace(input.ContentHash)
	input.Vector = append([]float64(nil), input.Vector...)
	if input.SourceID == "" || input.Token == "" || input.SourceItemID == "" || input.Title == "" || input.ContentHash == "" {
		return PushInput{}, ErrInvalidPush
	}
	return input, nil
}

func latencyMilliseconds(receivedAt time.Time, now time.Time) int64 {
	if receivedAt.IsZero() {
		return 0
	}
	latency := now.Sub(receivedAt.UTC()).Milliseconds()
	if latency < 0 {
		return 0
	}
	return latency
}

func cloneSource(source Source) Source {
	return source
}
