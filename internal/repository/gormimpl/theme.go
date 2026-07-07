package gormimpl

import (
	"context"

	"gorm.io/gorm"
)

type ThemeRepo struct {
	db *gorm.DB
}

func NewThemeRepo(db *gorm.DB) *ThemeRepo {
	return &ThemeRepo{db: db}
}

func (r *ThemeRepo) Create(ctx context.Context, monitorID int64, themeKey, title, summary string) (int64, error) {
	m := Theme{
		MonitorID: monitorID,
		ThemeKey:  themeKey,
		Title:     title,
		Summary:   summary,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return 0, err
	}
	return m.ID, nil
}

func (r *ThemeRepo) AddMembership(ctx context.Context, themeID int64, sourceKind string, eventID, topicID *int64) error {
	m := ThemeMembership{
		ThemeID:    themeID,
		SourceKind: sourceKind,
		EventID:    eventID,
		TopicID:    topicID,
	}
	return r.db.WithContext(ctx).Create(&m).Error
}
