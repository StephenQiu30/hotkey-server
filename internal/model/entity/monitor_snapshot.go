package entity

import "time"

type MonitorSnapshot struct {
	ID               int64     `gorm:"column:id;primaryKey"`
	MonitorID        int64     `gorm:"column:monitor_id"`
	SnapshotTime     time.Time `gorm:"column:snapshot_time"`
	NewPostCount     int       `gorm:"column:new_post_count"`
	ActiveTopicCount int       `gorm:"column:active_topic_count"`
	TotalEngagement  int       `gorm:"column:total_engagement"`
	TopTopicID       *int64    `gorm:"column:top_topic_id"`
}

func (MonitorSnapshot) TableName() string { return "monitor_snapshots" }
