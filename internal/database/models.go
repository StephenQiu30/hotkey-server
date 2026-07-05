package database

import "time"

// GORM models — simple queries use ORM, complex ones still use Raw().

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


type Event struct {
	ID            int64      `gorm:"column:id;primaryKey"`
	MonitorID     int64      `gorm:"column:monitor_id"`
	EventKey      string     `gorm:"column:event_key"`
	Title         string     `gorm:"column:title"`
	Summary       string     `gorm:"column:summary"`
	MachineStatus string     `gorm:"column:machine_status"`
	SourcePostID  *int64     `gorm:"column:source_post_id"`
	FirstSeenAt   time.Time  `gorm:"column:first_seen_at"`
	LastActiveAt  time.Time  `gorm:"column:last_active_at"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
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

type KnowledgeRun struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	RunKey       string     `gorm:"column:run_key"`
	RunType      string     `gorm:"column:run_type"`
	TargetDate   *time.Time `gorm:"column:target_date"`
	Status       string     `gorm:"column:status"`
	ErrorMessage string     `gorm:"column:error_message"`
	StartedAt    *time.Time `gorm:"column:started_at"`
	FinishedAt   *time.Time `gorm:"column:finished_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
}

func (KnowledgeRun) TableName() string { return "knowledge_runs" }

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

type ExportBundle struct {
	ID        int64      `gorm:"column:id;primaryKey"`
	MonitorID int64      `gorm:"column:monitor_id"`
	BundleKey string     `gorm:"column:bundle_key"`
	BundleKind string    `gorm:"column:bundle_kind"`
	DateStart *time.Time `gorm:"column:date_start"`
	DateEnd   *time.Time `gorm:"column:date_end"`
	Status    string     `gorm:"column:status"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

func (ExportBundle) TableName() string { return "export_bundles" }

type EventAnnotation struct {
	ID                int64     `gorm:"column:id;primaryKey"`
	EventID           int64     `gorm:"column:event_id"`
	ManualTags        []byte    `gorm:"column:manual_tags;type:jsonb"`
	AnalystConclusion string    `gorm:"column:analyst_conclusion"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (EventAnnotation) TableName() string { return "event_annotations" }

type TopicAnnotation struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	TopicID       int64     `gorm:"column:topic_id"`
	MaterialStatus string   `gorm:"column:material_status"`
	ManualSummary string    `gorm:"column:manual_summary"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (TopicAnnotation) TableName() string { return "topic_annotations" }

type ThemeMembership struct {
	ID         int64     `gorm:"column:id;primaryKey"`
	ThemeID    int64     `gorm:"column:theme_id"`
	EventID    *int64    `gorm:"column:event_id"`
	TopicID    *int64    `gorm:"column:topic_id"`
	SourceKind string    `gorm:"column:source_kind"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

func (ThemeMembership) TableName() string { return "theme_memberships" }

type KnowledgeObjectRevision struct {
	ID         int64     `gorm:"column:id;primaryKey"`
	ObjectType string    `gorm:"column:object_type"`
	ObjectID   int64     `gorm:"column:object_id"`
	Revision   string    `gorm:"column:revision"`
	SourcePath string    `gorm:"column:source_path"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (KnowledgeObjectRevision) TableName() string { return "knowledge_object_revisions" }

type HotEvent struct {
	ID          int64      `gorm:"column:id;primaryKey"`
	Name        string     `gorm:"column:name"`
	HeatScore   float64    `gorm:"column:heat_score"`
	Platform    string     `gorm:"column:platform"`
	Trend       string     `gorm:"column:trend"`
	FirstSeenAt time.Time  `gorm:"column:first_seen_at"`
	LastSeenAt  time.Time  `gorm:"column:last_seen_at"`
	PeakAt      *time.Time `gorm:"column:peak_at"`
	TopicIDs    []byte     `gorm:"column:topic_ids;type:bigint[]"`
	PostIDs     []byte     `gorm:"column:post_ids;type:bigint[]"`
	Summary     string     `gorm:"column:summary"`
	Category    string     `gorm:"column:category"`
	Status      string     `gorm:"column:status"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
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
