package redisinfra

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	StatusAccepted    = "accepted"
	StatusDuplicate   = "duplicate"
	StatusRateLimited = "rate_limited"

	ModeAvailable = "available"
	ModeDegraded  = "degraded"
)

var ErrInvalidRedisInfraRequest = errors.New("invalid redis infra request")

type LockResult struct {
	Key      string    `json:"key"`
	Owner    string    `json:"owner"`
	Acquired bool      `json:"acquired"`
	Status   string    `json:"status"`
	ExpireAt time.Time `json:"expireAt"`
}

type RefreshRequest struct {
	UserID string        `json:"userId"`
	Scope  string        `json:"scope"`
	Target string        `json:"target"`
	Now    time.Time     `json:"-"`
	Limit  int           `json:"-"`
	Window time.Duration `json:"-"`
}

type RefreshResult struct {
	ID         string    `json:"id"`
	UserID     string    `json:"userId"`
	Scope      string    `json:"scope"`
	Target     string    `json:"target"`
	Accepted   bool      `json:"accepted"`
	Status     string    `json:"status"`
	EnqueuedAt time.Time `json:"enqueuedAt"`
}

type RefreshQueueItem struct {
	ID         string    `json:"id"`
	UserID     string    `json:"userId"`
	Scope      string    `json:"scope"`
	Target     string    `json:"target"`
	EnqueuedAt time.Time `json:"enqueuedAt"`
}

type DedupResult struct {
	Key      string `json:"key"`
	Accepted bool   `json:"accepted"`
	Status   string `json:"status"`
}

type HealthStatus struct {
	Available bool   `json:"available"`
	Mode      string `json:"mode"`
}

type lockRecord struct {
	owner    string
	expireAt time.Time
}

type rateRecord struct {
	timestamps []time.Time
}

type Service struct {
	mu        sync.Mutex
	available bool
	locks     map[string]lockRecord
	rates     map[string]rateRecord
	seen      map[string]time.Time
	queue     []RefreshQueueItem
	nextQueue int
}

func NewService() *Service {
	return &Service{
		available: true,
		locks:     make(map[string]lockRecord),
		rates:     make(map[string]rateRecord),
		seen:      make(map[string]time.Time),
		nextQueue: 1,
	}
}

func (s *Service) SetAvailable(available bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.available = available
}

func (s *Service) Health() HealthStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return HealthStatus{Available: false, Mode: ModeDegraded}
	}
	return HealthStatus{Available: true, Mode: ModeAvailable}
}

func (s *Service) AcquireTaskLock(key string, owner string, ttl time.Duration) (LockResult, error) {
	key = strings.TrimSpace(key)
	owner = strings.TrimSpace(owner)
	if key == "" || owner == "" || ttl <= 0 {
		return LockResult{}, ErrInvalidRedisInfraRequest
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if existing, ok := s.locks[key]; ok && existing.expireAt.After(now) {
		return LockResult{
			Key:      key,
			Owner:    existing.owner,
			Acquired: false,
			Status:   StatusDuplicate,
			ExpireAt: existing.expireAt,
		}, nil
	}

	expireAt := now.Add(ttl)
	s.locks[key] = lockRecord{owner: owner, expireAt: expireAt}
	return LockResult{
		Key:      key,
		Owner:    owner,
		Acquired: true,
		Status:   StatusAccepted,
		ExpireAt: expireAt,
	}, nil
}

func (s *Service) ReleaseTaskLock(key string, owner string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.locks[key]
	if !ok || existing.owner != owner {
		return nil
	}
	delete(s.locks, key)
	return nil
}

func (s *Service) EnqueueRefresh(req RefreshRequest) (RefreshResult, error) {
	normalized, err := normalizeRefresh(req)
	if err != nil {
		return RefreshResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rateKey := normalized.UserID + ":" + normalized.Scope + ":" + normalized.Target
	record := s.rates[rateKey]
	windowStart := normalized.Now.Add(-normalized.Window)
	active := make([]time.Time, 0, len(record.timestamps)+1)
	for _, timestamp := range record.timestamps {
		if timestamp.After(windowStart) {
			active = append(active, timestamp)
		}
	}
	if len(active) >= normalized.Limit {
		s.rates[rateKey] = rateRecord{timestamps: active}
		return RefreshResult{
			UserID:     normalized.UserID,
			Scope:      normalized.Scope,
			Target:     normalized.Target,
			Accepted:   false,
			Status:     StatusRateLimited,
			EnqueuedAt: normalized.Now,
		}, nil
	}

	active = append(active, normalized.Now)
	s.rates[rateKey] = rateRecord{timestamps: active}
	id := fmt.Sprintf("refresh_%d", s.nextQueue)
	s.nextQueue++
	item := RefreshQueueItem{
		ID:         id,
		UserID:     normalized.UserID,
		Scope:      normalized.Scope,
		Target:     normalized.Target,
		EnqueuedAt: normalized.Now,
	}
	s.queue = append(s.queue, item)
	return RefreshResult{
		ID:         id,
		UserID:     item.UserID,
		Scope:      item.Scope,
		Target:     item.Target,
		Accepted:   true,
		Status:     StatusAccepted,
		EnqueuedAt: item.EnqueuedAt,
	}, nil
}

func (s *Service) ListRefreshQueue() []RefreshQueueItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return []RefreshQueueItem{}
	}
	return append([]RefreshQueueItem(nil), s.queue...)
}

func (s *Service) MarkSeen(key string, ttl time.Duration) DedupResult {
	key = strings.TrimSpace(key)
	if key == "" || ttl <= 0 {
		return DedupResult{Key: key, Accepted: false, Status: StatusDuplicate}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if expireAt, ok := s.seen[key]; ok && expireAt.After(now) {
		return DedupResult{Key: key, Accepted: false, Status: StatusDuplicate}
	}
	s.seen[key] = now.Add(ttl)
	return DedupResult{Key: key, Accepted: true, Status: StatusAccepted}
}

func normalizeRefresh(req RefreshRequest) (RefreshRequest, error) {
	req.UserID = strings.TrimSpace(req.UserID)
	req.Scope = strings.TrimSpace(req.Scope)
	req.Target = strings.TrimSpace(req.Target)
	if req.UserID == "" || req.Scope == "" || req.Target == "" {
		return RefreshRequest{}, ErrInvalidRedisInfraRequest
	}
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	}
	if req.Limit <= 0 {
		req.Limit = 2
	}
	if req.Window <= 0 {
		req.Window = time.Hour
	}
	return req, nil
}
