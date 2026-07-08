package entity

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

type Event struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	MonitorID     int64     `gorm:"column:monitor_id"`
	EventKey      string    `gorm:"column:event_key"`
	Title         string    `gorm:"column:title"`
	Summary       string    `gorm:"column:summary"`
	MachineStatus string    `gorm:"column:machine_status"`
	SourcePostID  *int64    `gorm:"column:source_post_id"`
	FirstSeenAt   time.Time `gorm:"column:first_seen_at"`
	LastActiveAt  time.Time `gorm:"column:last_active_at"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (Event) TableName() string { return "events" }

type TopicEvent struct {
	ID               int64     `gorm:"column:id;primaryKey"`
	TopicID          int64     `gorm:"column:topic_id"`
	EventID          int64     `gorm:"column:event_id"`
	RelationshipType string    `gorm:"column:relationship_type"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (TopicEvent) TableName() string { return "topic_events" }

type HotEvent struct {
	ID          int64          `gorm:"column:id;primaryKey"`
	Name        string         `gorm:"column:name"`
	HeatScore   float64        `gorm:"column:heat_score"`
	Platform    string         `gorm:"column:platform"`
	Trend       string         `gorm:"column:trend"`
	FirstSeenAt time.Time      `gorm:"column:first_seen_at"`
	LastSeenAt  time.Time      `gorm:"column:last_seen_at"`
	PeakAt      *time.Time     `gorm:"column:peak_at"`
	TopicIDs    pkg.Int64Array `gorm:"column:topic_ids;type:bigint[]"`
	PostIDs     pkg.Int64Array `gorm:"column:post_ids;type:bigint[]"`
	Summary     string         `gorm:"column:summary"`
	Category    string         `gorm:"column:category"`
	Status      string         `gorm:"column:status"`
	CreatedAt   time.Time      `gorm:"column:created_at"`
	UpdatedAt   time.Time      `gorm:"column:updated_at"`
}

func (HotEvent) TableName() string { return "hot_events" }

type HotEventPlatform struct {
	HotEventID int64     `gorm:"column:hot_event_id;primaryKey"`
	Platform   string    `gorm:"column:platform;primaryKey"`
	Rank       int       `gorm:"column:rank"`
	Title      string    `gorm:"column:title"`
	URL        string    `gorm:"column:url"`
	Heat       float64   `gorm:"column:heat"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (HotEventPlatform) TableName() string { return "hot_event_platforms" }
