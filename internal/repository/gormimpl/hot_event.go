package gormimpl

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model"
	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"gorm.io/gorm"
)

type HotEventRepo struct {
	db *gorm.DB
}

func NewHotEventRepo(db *gorm.DB) *HotEventRepo {
	return &HotEventRepo{db: db}
}

func (r *HotEventRepo) Create(ctx context.Context, event *model.HotEvent) error {
	m := FromHotEvent(*event)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return err
	}
	event.ID = m.ID
	return nil
}

func (r *HotEventRepo) GetByID(ctx context.Context, id int64) (*model.HotEvent, error) {
	var m HotEvent
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	result := ToHotEvent(m)
	return &result, nil
}

func (r *HotEventRepo) List(ctx context.Context, filter repository.HotEventFilter) ([]*model.HotEvent, int64, error) {
	query := r.db.WithContext(ctx).Model(&HotEvent{})
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Platform != "" {
		query = query.Where("platform = ?", filter.Platform)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := "heat_score DESC"
	if filter.Sort == "last_seen" {
		order = "last_seen_at DESC"
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	var models []HotEvent
	if err := query.Order(order).Limit(limit).Offset(filter.Offset).Find(&models).Error; err != nil {
		return nil, 0, err
	}

	events := make([]*model.HotEvent, len(models))
	for i := range models {
		ev := ToHotEvent(models[i])
		events[i] = &ev
	}
	return events, total, nil
}

func (r *HotEventRepo) Update(ctx context.Context, event *model.HotEvent) error {
	return r.db.WithContext(ctx).Model(&HotEvent{}).Where("id = ?", event.ID).Updates(map[string]any{
		"name":         event.Name,
		"heat_score":   event.HeatScore,
		"platform":     event.Platform,
		"trend":        event.Trend,
		"last_seen_at": event.LastSeenAt,
		"peak_at":      event.PeakAt,
		"topic_ids":    event.TopicIDs,
		"post_ids":     event.PostIDs,
		"summary":      event.Summary,
		"category":     event.Category,
		"status":       event.Status,
		"updated_at":   time.Now(),
	}).Error
}

func (r *HotEventRepo) ArchiveOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Model(&HotEvent{}).
		Where("last_seen_at < ? AND status = 'active'", cutoff).
		Update("status", "archived")
	return result.RowsAffected, result.Error
}

func (r *HotEventRepo) AddPlatform(ctx context.Context, eventID int64, platform *model.EventPlatform) error {
	m := HotEventPlatform{
		HotEventID: eventID,
		Platform:   platform.Platform,
		Rank:       platform.Rank,
		Title:      platform.Title,
		URL:        platform.URL,
		Heat:       platform.Heat,
		UpdatedAt:  time.Now(),
	}
	return r.db.WithContext(ctx).Create(&m).Error
}

func (r *HotEventRepo) GetPlatforms(ctx context.Context, eventID int64) ([]*model.EventPlatform, error) {
	var models []HotEventPlatform
	if err := r.db.WithContext(ctx).Where("hot_event_id = ?", eventID).Find(&models).Error; err != nil {
		return nil, err
	}
	platforms := make([]*model.EventPlatform, len(models))
	for i := range models {
		platforms[i] = &model.EventPlatform{
			Platform:  models[i].Platform,
			Rank:      models[i].Rank,
			Title:     models[i].Title,
			URL:       models[i].URL,
			Heat:      models[i].Heat,
			UpdatedAt: models[i].UpdatedAt,
		}
	}
	return platforms, nil
}

func (r *HotEventRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("last_seen_at < ?", cutoff).Delete(&HotEvent{})
	return result.RowsAffected, result.Error
}
