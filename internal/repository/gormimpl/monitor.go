package gormimpl

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model"
	"github.com/StephenQiu30/hotkey-server/internal/pkg"
	"gorm.io/gorm"
)

type MonitorRepo struct {
	db *gorm.DB
}

func NewMonitorRepo(db *gorm.DB) *MonitorRepo {
	return &MonitorRepo{db: db}
}

// Create inserts a new monitor using GORM Create.
func (r *MonitorRepo) Create(ctx context.Context, userID int64, input model.CreateMonitorInput) (model.KeywordMonitor, error) {
	m := KeywordMonitor{
		UserID:               userID,
		Name:                 input.Name,
		QueryText:            input.QueryText,
		Language:             input.Language,
		Region:               input.Region,
		PollIntervalMinutes:  input.PollIntervalMinutes,
		AlertEnabled:         input.AlertEnabled,
		AlertThresholdConfig: pkg.JSONB[map[string]any]{Data: make(map[string]any)},
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return model.KeywordMonitor{}, err
	}
	return ToKeywordMonitor(m), nil
}

func (r *MonitorRepo) GetByID(ctx context.Context, id int64) (*model.KeywordMonitor, error) {
	var m KeywordMonitor
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := ToKeywordMonitor(m)
	return &result, nil
}

func (r *MonitorRepo) ListByUser(ctx context.Context, userID int64) ([]model.KeywordMonitor, error) {
	var models []KeywordMonitor
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]model.KeywordMonitor, len(models))
	for i := range models {
		result[i] = ToKeywordMonitor(models[i])
	}
	return result, nil
}

// Update uses GORM Updates with map — eliminates the raw SQL $N concatenation.
func (r *MonitorRepo) Update(ctx context.Context, id int64, userID int64, input model.UpdateMonitorInput) (model.KeywordMonitor, error) {
	updates := make(map[string]any)
	if input.Name != nil {
		updates["name"] = *input.Name
	}
	if input.QueryText != nil {
		updates["query_text"] = *input.QueryText
	}
	if input.Language != nil {
		updates["language"] = *input.Language
	}
	if input.Region != nil {
		updates["region"] = *input.Region
	}
	if input.PollIntervalMinutes != nil {
		updates["poll_interval_minutes"] = *input.PollIntervalMinutes
	}
	if input.AlertEnabled != nil {
		updates["alert_enabled"] = *input.AlertEnabled
	}
	if input.Status != nil {
		updates["status"] = *input.Status
	}

	if len(updates) == 0 {
		m, err := r.GetByID(ctx, id)
		if err != nil {
			return model.KeywordMonitor{}, err
		}
		if m == nil {
			return model.KeywordMonitor{}, gorm.ErrRecordNotFound
		}
		return *m, nil
	}

	updates["updated_at"] = time.Now()

	var m KeywordMonitor
	if err := r.db.WithContext(ctx).Model(&KeywordMonitor{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(updates).
		Error; err != nil {
		return model.KeywordMonitor{}, err
	}
	// Re-read to return full row
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.KeywordMonitor{}, gorm.ErrRecordNotFound
		}
		return model.KeywordMonitor{}, err
	}
	return ToKeywordMonitor(m), nil
}

func (r *MonitorRepo) ListActiveIDs(ctx context.Context) ([]int64, error) {
	var ids []int64
	if err := r.db.WithContext(ctx).Model(&KeywordMonitor{}).
		Where("status = 'active'").
		Order("id").
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}
