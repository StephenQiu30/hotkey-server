package entity

import "time"

type Topic struct {
	ID                   int64     `gorm:"column:id;primaryKey"`
	MonitorID            int64     `gorm:"column:monitor_id"`
	TopicKey             string    `gorm:"column:topic_key"`
	Title                string    `gorm:"column:title"`
	Summary              string    `gorm:"column:summary"`
	Status               string    `gorm:"column:status"`
	FirstDetectedAt      time.Time `gorm:"column:first_detected_at"`
	LastActiveAt         time.Time `gorm:"column:last_active_at"`
	CurrentHeatScore     float64   `gorm:"column:current_heat_score"`
	TrendDirection       string    `gorm:"column:trend_direction"`
	RepresentativePostID *int64    `gorm:"column:representative_post_id"`
	CreatedAt            time.Time `gorm:"column:created_at"`
	UpdatedAt            time.Time `gorm:"column:updated_at"`
}

func (Topic) TableName() string { return "topics" }

type TopicPost struct {
	ID               int64     `gorm:"column:id;primaryKey"`
	TopicID          int64     `gorm:"column:topic_id"`
	PostID           int64     `gorm:"column:post_id"`
	MembershipScore  float64   `gorm:"column:membership_score"`
	IsRepresentative bool      `gorm:"column:is_representative"`
	AddedAt          time.Time `gorm:"column:added_at"`
}

func (TopicPost) TableName() string { return "topic_posts" }

type TopicSnapshot struct {
	ID                int64     `gorm:"column:id;primaryKey"`
	TopicID           int64     `gorm:"column:topic_id"`
	SnapshotTime      time.Time `gorm:"column:snapshot_time"`
	PostCount         int       `gorm:"column:post_count"`
	UniqueAuthorCount int       `gorm:"column:unique_author_count"`
	EngagementSum     int       `gorm:"column:engagement_sum"`
	HeatScore         float64   `gorm:"column:heat_score"`
	TrendVelocity     float64   `gorm:"column:trend_velocity"`
}

func (TopicSnapshot) TableName() string { return "topic_snapshots" }
