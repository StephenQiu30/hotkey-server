package obsidian

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrConfigNotFound  = errors.New("sync config not found")
	ErrDuplicateConfig = errors.New("sync config already exists for user")
)

// MemoryRepository is an in-memory implementation of Repository for testing.
type MemoryRepository struct {
	mu      sync.Mutex
	configs map[string]SyncConfig // keyed by ID
	records map[string]SyncRecord // keyed by ID
	byUser  map[string]string     // userID -> configID
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		configs: make(map[string]SyncConfig),
		records: make(map[string]SyncRecord),
		byUser:  make(map[string]string),
	}
}

func (r *MemoryRepository) SaveConfig(_ context.Context, config SyncConfig) (SyncConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if config.ID == "" {
		config.ID = "cfg-" + time.Now().Format("20060102150405.000000")
	}

	if existing, ok := r.byUser[config.UserID]; ok && existing != config.ID {
		return SyncConfig{}, ErrDuplicateConfig
	}

	if _, exists := r.configs[config.ID]; !exists {
		config.CreatedAt = time.Now().UTC()
	}
	config.UpdatedAt = time.Now().UTC()
	r.configs[config.ID] = config
	r.byUser[config.UserID] = config.ID
	return config, nil
}

func (r *MemoryRepository) FindConfigByUser(_ context.Context, userID string) (SyncConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	configID, ok := r.byUser[userID]
	if !ok {
		return SyncConfig{}, ErrConfigNotFound
	}
	return r.configs[configID], nil
}

func (r *MemoryRepository) FindConfigByID(_ context.Context, id string) (SyncConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	config, ok := r.configs[id]
	if !ok {
		return SyncConfig{}, ErrConfigNotFound
	}
	return config, nil
}

func (r *MemoryRepository) DeleteConfig(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	config, ok := r.configs[id]
	if !ok {
		return ErrConfigNotFound
	}
	delete(r.configs, id)
	delete(r.byUser, config.UserID)
	return nil
}

func (r *MemoryRepository) SaveRecord(_ context.Context, record SyncRecord) (SyncRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if record.ID == "" {
		record.ID = "rec-" + time.Now().Format("20060102150405.000000")
	}
	if _, exists := r.records[record.ID]; !exists {
		record.CreatedAt = time.Now().UTC()
	}
	record.UpdatedAt = time.Now().UTC()
	r.records[record.ID] = record
	return record, nil
}

func (r *MemoryRepository) FindRecordByIdempotencyKey(_ context.Context, configID, idempotencyKey string) (SyncRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, rec := range r.records {
		if rec.ConfigID == configID && rec.IdempotencyKey == idempotencyKey {
			return rec, nil
		}
	}
	return SyncRecord{}, ErrConfigNotFound // reuse error; service layer checks by convention
}

func (r *MemoryRepository) ListRecordsByConfig(_ context.Context, configID string, limit int) ([]SyncRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []SyncRecord
	for _, rec := range r.records {
		if rec.ConfigID == configID {
			result = append(result, rec)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
