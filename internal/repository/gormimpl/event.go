package gormimpl

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EventRepo struct {
	db *gorm.DB
}

func NewEventRepo(db *gorm.DB) *EventRepo {
	return &EventRepo{db: db}
}

func (r *EventRepo) Create(ctx context.Context, e model.Event) (model.Event, error) {
	m := FromEvent(e)
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "monitor_id"}, {Name: "event_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "summary", "last_active_at", "updated_at"}),
		}).
		Create(&m).Error; err != nil {
		return model.Event{}, err
	}
	e.ID = m.ID
	return e, nil
}

func (r *EventRepo) GetByID(ctx context.Context, id int64) (*model.Event, error) {
	var m Event
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	result := ToEvent(m)
	return &result, nil
}

func (r *EventRepo) GetByKey(ctx context.Context, monitorID int64, eventKey string) (*model.Event, error) {
	var m Event
	if err := r.db.WithContext(ctx).
		Where("monitor_id = ? AND event_key = ?", monitorID, eventKey).
		First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	result := ToEvent(m)
	return &result, nil
}
