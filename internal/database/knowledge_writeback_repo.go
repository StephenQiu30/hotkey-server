package database

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// RecordAttemptInput describes a writeback attempt to be recorded.
type RecordAttemptInput struct {
	ObjectType     string
	ObjectID       int64
	FieldName      string
	FieldValue     interface{}
	Status         string
	ConflictReason string
	SourcePath     string
}

// KnowledgeWritebackLogModel is the GORM model for knowledge_writeback_logs.
type KnowledgeWritebackLogModel struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	ObjectType     string    `gorm:"column:object_type"`
	ObjectID       int64     `gorm:"column:object_id"`
	FieldName      string    `gorm:"column:field_name"`
	FieldValue     string    `gorm:"column:field_value;type:jsonb"`
	Status         string    `gorm:"column:status"`
	ConflictReason string    `gorm:"column:conflict_reason"`
	SourcePath     string    `gorm:"column:source_path"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

// TableName overrides the default table name.
func (KnowledgeWritebackLogModel) TableName() string {
	return "knowledge_writeback_logs"
}

// KnowledgeWritebackRepo handles audit logging for writeback attempts.
type KnowledgeWritebackRepo struct {
	db *gorm.DB
}

// NewKnowledgeWritebackRepo creates a new KnowledgeWritebackRepo.
func NewKnowledgeWritebackRepo(db *gorm.DB) *KnowledgeWritebackRepo {
	return &KnowledgeWritebackRepo{db: db}
}

// RecordAttempt persists a writeback attempt in the audit log.
func (r *KnowledgeWritebackRepo) RecordAttempt(ctx context.Context, in RecordAttemptInput) error {
	model := KnowledgeWritebackLogModel{
		ObjectType:     in.ObjectType,
		ObjectID:       in.ObjectID,
		FieldName:      in.FieldName,
		FieldValue:     toJSONString(in.FieldValue),
		Status:         in.Status,
		ConflictReason: in.ConflictReason,
		SourcePath:     in.SourcePath,
	}
	return r.db.WithContext(ctx).Create(&model).Error
}

// toJSONString marshals v to a JSON string for JSONB storage.
func toJSONString(v interface{}) string {
	if v == nil {
		return "null"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return `"<marshal error>"`
	}
	return string(b)
}
