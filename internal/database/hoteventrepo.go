package database

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/hotevent"
	"gorm.io/gorm"
)

// HotEventRepo implements hotevent.Repository via GORM.
type HotEventRepo struct {
	db *gorm.DB
}

func NewHotEventRepo(db *gorm.DB) *HotEventRepo {
	return &HotEventRepo{db: db}
}

func (r *HotEventRepo) Create(event *hotevent.HotEvent) error {
	model := toGormEvent(event)
	if err := r.db.Create(model).Error; err != nil {
		return err
	}
	event.ID = model.ID
	return nil
}

func (r *HotEventRepo) GetByID(id int64) (*hotevent.HotEvent, error) {
	var model HotEvent
	if err := r.db.Where("id = ?", id).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, hotevent.ErrNotFound
		}
		return nil, err
	}
	return fromGormEvent(&model), nil
}

func (r *HotEventRepo) List(filter hotevent.ListFilter) ([]*hotevent.HotEvent, int64, error) {
	query := r.db.Model(&HotEvent{})
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

	events := make([]*hotevent.HotEvent, len(models))
	for i := range models {
		events[i] = fromGormEvent(&models[i])
	}
	return events, total, nil
}

func (r *HotEventRepo) Update(event *hotevent.HotEvent) error {
	return r.db.Model(&HotEvent{}).Where("id = ?", event.ID).Updates(map[string]any{
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

func (r *HotEventRepo) ArchiveOlderThan(cutoff time.Time) (int64, error) {
	result := r.db.Model(&HotEvent{}).
		Where("last_seen_at < ? AND status = 'active'", cutoff).
		Update("status", "archived")
	return result.RowsAffected, result.Error
}

func (r *HotEventRepo) AddPlatform(eventID int64, platform *hotevent.EventPlatform) error {
	model := HotEventPlatform{
		HotEventID: eventID,
		Platform:   platform.Platform,
		Rank:       platform.Rank,
		Title:      platform.Title,
		URL:        platform.URL,
		Heat:       platform.Heat,
		UpdatedAt:  time.Now(),
	}
	return r.db.Create(&model).Error
}

func (r *HotEventRepo) GetPlatforms(eventID int64) ([]*hotevent.EventPlatform, error) {
	var models []HotEventPlatform
	if err := r.db.Where("hot_event_id = ?", eventID).Find(&models).Error; err != nil {
		return nil, err
	}
	platforms := make([]*hotevent.EventPlatform, len(models))
	for i := range models {
		platforms[i] = fromGormPlatform(&models[i])
	}
	return platforms, nil
}

func (r *HotEventRepo) DeleteOlderThan(cutoff time.Time) (int64, error) {
	result := r.db.Where("last_seen_at < ?", cutoff).Delete(&HotEvent{})
	return result.RowsAffected, result.Error
}

// Helper conversions

func toGormEvent(e *hotevent.HotEvent) *HotEvent {
	return &HotEvent{
		Name:        e.Name,
		HeatScore:   e.HeatScore,
		Platform:    e.Platform,
		Trend:       e.Trend,
		FirstSeenAt: e.FirstSeenAt,
		LastSeenAt:  e.LastSeenAt,
		PeakAt:      e.PeakAt,
		TopicIDs:    toInt64Array(e.TopicIDs),
		PostIDs:     toInt64Array(e.PostIDs),
		Summary:     e.Summary,
		Category:    e.Category,
		Status:      e.Status,
	}
}

func fromGormEvent(m *HotEvent) *hotevent.HotEvent {
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

func fromGormPlatform(m *HotEventPlatform) *hotevent.EventPlatform {
	return &hotevent.EventPlatform{
		Platform: m.Platform,
		Rank:     m.Rank,
		Title:    m.Title,
		URL:      m.URL,
		Heat:     m.Heat,
	}
}
