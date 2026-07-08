package entity

import "time"

type Theme struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	MonitorID int64     `gorm:"column:monitor_id"`
	ThemeKey  string    `gorm:"column:theme_key"`
	Title     string    `gorm:"column:title"`
	Summary   string    `gorm:"column:summary"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Theme) TableName() string { return "themes" }

type ThemeMembership struct {
	ID         int64     `gorm:"column:id;primaryKey"`
	ThemeID    int64     `gorm:"column:theme_id"`
	EventID    *int64    `gorm:"column:event_id"`
	TopicID    *int64    `gorm:"column:topic_id"`
	SourceKind string    `gorm:"column:source_kind"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

func (ThemeMembership) TableName() string { return "theme_memberships" }
