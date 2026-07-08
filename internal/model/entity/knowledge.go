package entity

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

type KnowledgeRun struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	RunKey       string     `gorm:"column:run_key"`
	RunType      string     `gorm:"column:run_type"`
	TargetDate   *time.Time `gorm:"column:target_date"`
	Status       string     `gorm:"column:status"`
	ErrorMessage string     `gorm:"column:error_message"`
	StartedAt    *time.Time `gorm:"column:started_at"`
	FinishedAt   *time.Time `gorm:"column:finished_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
}

func (KnowledgeRun) TableName() string { return "knowledge_runs" }

type KnowledgeWritebackLog struct {
	ID             int64             `gorm:"column:id;primaryKey"`
	ObjectType     string            `gorm:"column:object_type"`
	ObjectID       int64             `gorm:"column:object_id"`
	FieldName      string            `gorm:"column:field_name"`
	FieldValue     pkg.JSONB[string] `gorm:"column:field_value;type:jsonb"`
	Status         string            `gorm:"column:status"`
	ConflictReason string            `gorm:"column:conflict_reason"`
	SourcePath     string            `gorm:"column:source_path"`
	CreatedAt      time.Time         `gorm:"column:created_at"`
}

func (KnowledgeWritebackLog) TableName() string { return "knowledge_writeback_logs" }

type KnowledgeObjectRevision struct {
	ID         int64     `gorm:"column:id;primaryKey"`
	ObjectType string    `gorm:"column:object_type"`
	ObjectID   int64     `gorm:"column:object_id"`
	Revision   string    `gorm:"column:revision"`
	SourcePath string    `gorm:"column:source_path"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (KnowledgeObjectRevision) TableName() string { return "knowledge_object_revisions" }
