package gormimpl

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/hotevent"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/pkg"
	"gorm.io/gorm"
)

// HotEventFilter defines filtering and pagination for List queries.
type HotEventFilter struct {
	Status   string
	Platform string
	Sort     string // "heat_score" (default) or "last_seen"
	Limit    int
	Offset   int
}

// HotEventRepo implements hotevent.Repository via GORM.
type HotEventRepo struct {
	db *gorm.DB
}

func NewHotEventRepo(db *gorm.DB) *HotEventRepo {
	return &HotEventRepo{db: db}
}

func (r *HotEventRepo) Create(ctx context.Context, event *hotevent.HotEvent) error {
	m := entity.HotEvent{
		Name:        event.Name,
		HeatScore:   event.HeatScore,
		Platform:    event.Platform,
		Trend:       event.Trend,
		FirstSeenAt: event.FirstSeenAt,
		LastSeenAt:  event.LastSeenAt,
		PeakAt:      event.PeakAt,
		TopicIDs:    toInt64Array(event.TopicIDs),
		PostIDs:     toInt64Array(event.PostIDs),
		Summary:     event.Summary,
		Category:    event.Category,
		Status:      event.Status,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return err
	}
	event.ID = m.ID
	event.CreatedAt = m.CreatedAt
	event.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *HotEventRepo) GetByID(ctx context.Context, id int64) (*hotevent.HotEvent, error) {
	var m entity.HotEvent
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, hotevent.ErrNotFound
		}
		return nil, err
	}
	return toHotEvent(m), nil
}

func (r *HotEventRepo) List(ctx context.Context, filter hotevent.ListFilter) ([]*hotevent.HotEvent, int64, error) {
	query := r.db.WithContext(ctx).Model(&entity.HotEvent{})
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

	var models []entity.HotEvent
	if err := query.Order(order).Limit(limit).Offset(filter.Offset).Find(&models).Error; err != nil {
		return nil, 0, err
	}

	events := make([]*hotevent.HotEvent, len(models))
	for i := range models {
		events[i] = toHotEvent(models[i])
	}
	return events, total, nil
}

func (r *HotEventRepo) Update(ctx context.Context, event *hotevent.HotEvent) error {
	return r.db.WithContext(ctx).Model(&entity.HotEvent{}).Where("id = ?", event.ID).Updates(map[string]any{
		"name":         event.Name,
		"heat_score":   event.HeatScore,
		"platform":     event.Platform,
		"trend":        event.Trend,
		"last_seen_at": event.LastSeenAt,
		"peak_at":      event.PeakAt,
		"topic_ids":    toInt64Array(event.TopicIDs),
		"post_ids":     toInt64Array(event.PostIDs),
		"summary":      event.Summary,
		"category":     event.Category,
		"status":       event.Status,
		"updated_at":   time.Now(),
	}).Error
}

func (r *HotEventRepo) ArchiveOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Model(&entity.HotEvent{}).
		Where("last_seen_at < ? AND status = 'active'", cutoff).
		Update("status", "archived")
	return result.RowsAffected, result.Error
}

func (r *HotEventRepo) AddPlatform(ctx context.Context, eventID int64, platform *hotevent.EventPlatform) error {
	m := entity.HotEventPlatform{
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

func (r *HotEventRepo) GetPlatforms(ctx context.Context, eventID int64) ([]*hotevent.EventPlatform, error) {
	var models []entity.HotEventPlatform
	if err := r.db.WithContext(ctx).Where("hot_event_id = ?", eventID).Find(&models).Error; err != nil {
		return nil, err
	}
	platforms := make([]*hotevent.EventPlatform, len(models))
	for i := range models {
		platforms[i] = &hotevent.EventPlatform{
			Platform: models[i].Platform,
			Rank:     models[i].Rank,
			Title:    models[i].Title,
			URL:      models[i].URL,
			Heat:     models[i].Heat,
		}
	}
	return platforms, nil
}

func (r *HotEventRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("last_seen_at < ?", cutoff).Delete(&entity.HotEvent{})
	return result.RowsAffected, result.Error
}

// toHotEvent converts a GORM entity.HotEvent to a domain entity.HotEvent.
func toHotEvent(m entity.HotEvent) *hotevent.HotEvent {
	return &hotevent.HotEvent{
		ID:          m.ID,
		Name:        m.Name,
		HeatScore:   m.HeatScore,
		Platform:    m.Platform,
		Trend:       m.Trend,
		FirstSeenAt: m.FirstSeenAt,
		LastSeenAt:  m.LastSeenAt,
		PeakAt:      m.PeakAt,
		TopicIDs:    fromInt64Array(m.TopicIDs),
		PostIDs:     fromInt64Array(m.PostIDs),
		Summary:     m.Summary,
		Category:    m.Category,
		Status:      m.Status,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// toInt64Array converts []int64 to pkg.Int64Array for GORM serialization.
func toInt64Array(src []int64) pkg.Int64Array { return pkg.Int64Array(src) }

// fromInt64Array converts pkg.Int64Array back to []int64.
func fromInt64Array(src pkg.Int64Array) []int64 { return []int64(src) }
