package obsidian

import "time"

// SyncStatus represents the current state of a sync config.
type SyncStatus string

const (
	SyncStatusActive       SyncStatus = "active"
	SyncStatusDisconnected SyncStatus = "disconnected"
	SyncStatusError        SyncStatus = "error"
)

// ConflictResolution defines how to handle Git conflicts.
type ConflictResolution string

const (
	ConflictResolutionSkip   ConflictResolution = "skip"
	ConflictResolutionBranch ConflictResolution = "branch"
)

// SyncConfig holds user's Git repo connection and sync preferences.
type SyncConfig struct {
	ID                 string
	UserID             string
	RepoURL            string
	Branch             string
	BaseDir            string
	AccessToken        string
	EventNoteTemplate  string
	DailyReportIndex   string
	WeeklyReportIndex  string
	ConflictResolution ConflictResolution
	Status             SyncStatus
	LastSyncAt         *time.Time
	LastError          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// SyncRecordState represents the outcome of a sync operation.
type SyncRecordState string

const (
	SyncRecordStatePending  SyncRecordState = "pending"
	SyncRecordStateSynced   SyncRecordState = "synced"
	SyncRecordStateConflict SyncRecordState = "conflict"
	SyncRecordStateFailed   SyncRecordState = "failed"
	SyncRecordStateSkipped  SyncRecordState = "skipped"
)

// SyncRecord tracks an individual sync operation for idempotency and audit.
type SyncRecord struct {
	ID          string
	ConfigID    string
	ContentType string // "event_note" | "daily_report" | "weekly_report"
	ContentID   string // source entity ID (event ID, report ID)
	FilePath    string
	IdempotencyKey string
	CommitSHA   string
	State       SyncRecordState
	Error       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
