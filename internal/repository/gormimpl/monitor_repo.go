package gormimpl

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/pkg"
	"gorm.io/gorm"
)

// MonitorRepo implements monitor.Repository via GORM.
type MonitorRepo struct {
	db *gorm.DB
}

func NewMonitorRepo(db *gorm.DB) *MonitorRepo {
	return &MonitorRepo{db: db}
}

func (r *MonitorRepo) Create(ctx context.Context, userID int64, input monitor.CreateMonitorInput) (monitor.Monitor, error) {
	m := entity.KeywordMonitor{
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
		return monitor.Monitor{}, err
	}
	return toDomainMonitor(m), nil
}

func (r *MonitorRepo) GetByID(ctx context.Context, id int64) (*monitor.Monitor, error) {
	var m entity.KeywordMonitor
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	result := toDomainMonitor(m)
	return &result, nil
}

func (r *MonitorRepo) ListByUser(ctx context.Context, userID int64) ([]monitor.Monitor, error) {
	var models []entity.KeywordMonitor
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]monitor.Monitor, len(models))
	for i := range models {
		result[i] = toDomainMonitor(models[i])
	}
	return result, nil
}

func (r *MonitorRepo) ListActive(ctx context.Context) ([]monitor.Monitor, error) {
	var models []entity.KeywordMonitor
	if err := r.db.WithContext(ctx).Where("status = ?", "active").Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]monitor.Monitor, len(models))
	for i, model := range models {
		out[i] = toDomainMonitor(model)
	}
	return out, nil
}

func (r *MonitorRepo) Update(ctx context.Context, id int64, userID int64, input monitor.UpdateMonitorInput) (monitor.Monitor, error) {
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
		got, err := r.GetByID(ctx, id)
		if err != nil || got == nil {
			return monitor.Monitor{}, err
		}
		return *got, nil
	}
	updates["updated_at"] = "now()"
	if err := r.db.WithContext(ctx).Model(&entity.KeywordMonitor{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(updates).Error; err != nil {
		return monitor.Monitor{}, err
	}
	got, err := r.GetByID(ctx, id)
	if err != nil || got == nil {
		return monitor.Monitor{}, err
	}
	return *got, nil
}

// SetQueryEmbedding stores the embedding vector for a monitor's query text.
func (r *MonitorRepo) SetQueryEmbedding(ctx context.Context, id int64, emb pkg.Vector384) error {
	return r.db.WithContext(ctx).Model(&entity.KeywordMonitor{}).
		Where("id = ?", id).
		Update("query_embedding", emb).Error
}

func toDomainMonitor(m entity.KeywordMonitor) monitor.Monitor {
	return monitor.Monitor{
		ID:                   m.ID,
		UserID:               m.UserID,
		Name:                 m.Name,
		QueryText:            m.QueryText,
		Language:             m.Language,
		Region:               m.Region,
		Status:               m.Status,
		PollIntervalMinutes:  m.PollIntervalMinutes,
		AlertEnabled:         m.AlertEnabled,
		AlertThresholdConfig: m.AlertThresholdConfig.Data,
		LastPolledAt:         m.LastPolledAt,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}
