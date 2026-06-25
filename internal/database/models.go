package database

import "time"

// GORM models map database tables for ORM access and documentation.
// Complex queries may still use Raw() in repositories.

type User struct {
	ID           int64     `gorm:"column:id;primaryKey"`
	Email        string    `gorm:"column:email"`
	PasswordHash string    `gorm:"column:password_hash"`
	DisplayName  string    `gorm:"column:display_name"`
	Status       string    `gorm:"column:status"`
	PlanType     string    `gorm:"column:plan_type"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (User) TableName() string { return "users" }

type KeywordMonitor struct {
	ID                    int64     `gorm:"column:id;primaryKey"`
	UserID                int64     `gorm:"column:user_id"`
	Name                  string    `gorm:"column:name"`
	QueryText             string    `gorm:"column:query_text"`
	Language              string    `gorm:"column:language"`
	Region                string    `gorm:"column:region"`
	Status                string    `gorm:"column:status"`
	PollIntervalMinutes   int       `gorm:"column:poll_interval_minutes"`
	AlertEnabled          bool      `gorm:"column:alert_enabled"`
	AlertThresholdConfig  []byte    `gorm:"column:alert_threshold_config;type:jsonb"`
	LastPolledAt          *time.Time `gorm:"column:last_polled_at"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at"`
}

func (KeywordMonitor) TableName() string { return "keyword_monitors" }

type PlatformPost struct {
	ID               int64     `gorm:"column:id;primaryKey"`
	Platform         string    `gorm:"column:platform"`
	PlatformPostID   string    `gorm:"column:platform_post_id"`
	AuthorPlatformID string    `gorm:"column:author_platform_id"`
	AuthorName       string    `gorm:"column:author_name"`
	AuthorHandle     string    `gorm:"column:author_handle"`
	ContentText      string    `gorm:"column:content_text"`
	ContentLang      string    `gorm:"column:content_lang"`
	PostURL          string    `gorm:"column:post_url"`
	PublishedAt      time.Time `gorm:"column:published_at"`
	LikeCount        int       `gorm:"column:like_count"`
	ReplyCount       int       `gorm:"column:reply_count"`
	RepostCount      int       `gorm:"column:repost_count"`
	QuoteCount       int       `gorm:"column:quote_count"`
	ViewCount        int       `gorm:"column:view_count"`
	NormalizedHash   string    `gorm:"column:normalized_hash"`
}

func (PlatformPost) TableName() string { return "platform_posts" }

type Topic struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	MonitorID      int64     `gorm:"column:monitor_id"`
	TopicKey       string    `gorm:"column:topic_key"`
	Title          string    `gorm:"column:title"`
	Summary        string    `gorm:"column:summary"`
	CurrentHeatScore float64 `gorm:"column:current_heat_score"`
	TrendDirection string    `gorm:"column:trend_direction"`
	Status         string    `gorm:"column:status"`
}

func (Topic) TableName() string { return "topics" }

type UserNotification struct {
	ID             int64      `gorm:"column:id;primaryKey"`
	UserID         int64      `gorm:"column:user_id"`
	AlertID        int64      `gorm:"column:alert_id"`
	Channel        string     `gorm:"column:channel"`
	DeliveryStatus string     `gorm:"column:delivery_status"`
	ReadAt         *time.Time `gorm:"column:read_at"`
	SentAt         *time.Time `gorm:"column:sent_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

func (UserNotification) TableName() string { return "user_notifications" }
