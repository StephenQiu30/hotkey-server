package obsidian

import "context"

// Repository defines the persistence interface for sync configs and records.
type Repository interface {
	SaveConfig(ctx context.Context, config SyncConfig) (SyncConfig, error)
	FindConfigByUser(ctx context.Context, userID string) (SyncConfig, error)
	FindConfigByID(ctx context.Context, id string) (SyncConfig, error)
	DeleteConfig(ctx context.Context, id string) error

	SaveRecord(ctx context.Context, record SyncRecord) (SyncRecord, error)
	FindRecordByIdempotencyKey(ctx context.Context, configID, idempotencyKey string) (SyncRecord, error)
	ListRecordsByConfig(ctx context.Context, configID string, limit int) ([]SyncRecord, error)
}
