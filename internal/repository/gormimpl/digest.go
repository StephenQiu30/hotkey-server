package gormimpl

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DigestRepo struct {
	db *gorm.DB
}

func NewDigestRepo(db *gorm.DB) *DigestRepo {
	return &DigestRepo{db: db}
}

func (r *DigestRepo) Upsert(ctx context.Context, e model.TopicDailyExport) (model.TopicDailyExport, error) {
	m := TopicDailyExport{
		MonitorID:    e.MonitorID,
		TopicID:      e.TopicID,
		ExportDate:   e.ExportDate,
		SummaryText:  e.SummaryText,
		MarkdownPath: e.MarkdownPath,
		Status:       e.Status,
		ErrorMessage: e.ErrorMessage,
		PublishedAt:  e.PublishedAt,
	}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "monitor_id"}, {Name: "topic_id"}, {Name: "export_date"}},
			DoUpdates: clause.AssignmentColumns([]string{"summary_text", "markdown_path", "status", "error_message", "published_at"}),
		}).
		Create(&m).Error; err != nil {
		return model.TopicDailyExport{}, err
	}
	e.ID = m.ID
	e.CreatedAt = m.CreatedAt
	return e, nil
}

func (r *DigestRepo) GetByTopicDate(ctx context.Context, topicID int64, exportDate string) (*model.TopicDailyExport, error) {
	var m TopicDailyExport
	if err := r.db.WithContext(ctx).
		Where("topic_id = ? AND export_date = ?", topicID, exportDate).
		First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	result := ToTopicDailyExport(m)
	return &result, nil
}
