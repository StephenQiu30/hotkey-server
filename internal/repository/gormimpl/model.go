package gormimpl

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model"
	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

// ──────────────────────────────────────────────
// GORM models — each maps 1:1 to one DB table.
// ──────────────────────────────────────────────

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
	ID                   int64            `gorm:"column:id;primaryKey"`
	UserID               int64            `gorm:"column:user_id"`
	Name                 string           `gorm:"column:name"`
	QueryText            string           `gorm:"column:query_text"`
	Language             string           `gorm:"column:language"`
	Region               string           `gorm:"column:region"`
	Status               string           `gorm:"column:status"`
	PollIntervalMinutes  int              `gorm:"column:poll_interval_minutes"`
	AlertEnabled         bool             `gorm:"column:alert_enabled"`
	AlertThresholdConfig pkg.JSONB[map[string]any] `gorm:"column:alert_threshold_config;type:jsonb"`
	LastPolledAt         *time.Time       `gorm:"column:last_polled_at"`
	CreatedAt            time.Time        `gorm:"column:created_at"`
	UpdatedAt            time.Time        `gorm:"column:updated_at"`
}

func (KeywordMonitor) TableName() string { return "keyword_monitors" }

type MonitorRun struct {
	ID             int64            `gorm:"column:id;primaryKey"`
	MonitorID      int64            `gorm:"column:monitor_id"`
	Platform       string           `gorm:"column:platform"`
	RunType        string           `gorm:"column:run_type"`
	Status         string           `gorm:"column:status"`
	StartedAt      time.Time        `gorm:"column:started_at"`
	FinishedAt     *time.Time       `gorm:"column:finished_at"`
	FetchedCount   int              `gorm:"column:fetched_count"`
	StoredCount    int              `gorm:"column:stored_count"`
	ErrorMessage   string           `gorm:"column:error_message"`
	CursorSnapshot pkg.JSONB[string] `gorm:"column:cursor_snapshot;type:jsonb"`
}

func (MonitorRun) TableName() string { return "monitor_runs" }

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
	PublishedAt      *time.Time `gorm:"column:published_at"`
	LikeCount        int       `gorm:"column:like_count"`
	ReplyCount       int       `gorm:"column:reply_count"`
	RepostCount      int       `gorm:"column:repost_count"`
	QuoteCount       int       `gorm:"column:quote_count"`
	ViewCount        int       `gorm:"column:view_count"`
	RawPayload       pkg.JSONB[string] `gorm:"column:raw_payload;type:jsonb"`
	NormalizedHash   string    `gorm:"column:normalized_hash"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (PlatformPost) TableName() string { return "platform_posts" }

type PlatformAuthor struct {
	ID               int64            `gorm:"column:id;primaryKey"`
	Platform         string           `gorm:"column:platform"`
	PlatformAuthorID string           `gorm:"column:platform_author_id"`
	Handle           string           `gorm:"column:handle"`
	DisplayName      string           `gorm:"column:display_name"`
	FollowersCount   int              `gorm:"column:followers_count"`
	Verified         bool             `gorm:"column:verified"`
	RawPayload       pkg.JSONB[string] `gorm:"column:raw_payload;type:jsonb"`
	UpdatedAt        time.Time        `gorm:"column:updated_at"`
}

func (PlatformAuthor) TableName() string { return "platform_authors" }

type MonitorPostHit struct {
	ID                  int64             `gorm:"column:id;primaryKey"`
	MonitorID           int64             `gorm:"column:monitor_id"`
	PostID              int64             `gorm:"column:post_id"`
	MatchedKeywords     pkg.JSONB[[]string]  `gorm:"column:matched_keywords;type:jsonb"`
	RelevanceScore      float64           `gorm:"column:relevance_score"`
	HeatScore           float64           `gorm:"column:heat_score"`
	FreshnessScore      float64           `gorm:"column:freshness_score"`
	AuthorInfluenceScore float64          `gorm:"column:author_influence_score"`
	FinalScore          float64           `gorm:"column:final_score"`
	FirstSeenAt         time.Time         `gorm:"column:first_seen_at"`
	LastSeenAt          time.Time         `gorm:"column:last_seen_at"`
}

func (MonitorPostHit) TableName() string { return "monitor_post_hits" }

type Topic struct {
	ID                int64        `gorm:"column:id;primaryKey"`
	MonitorID         int64        `gorm:"column:monitor_id"`
	TopicKey          string       `gorm:"column:topic_key"`
	Title             string       `gorm:"column:title"`
	Summary           string       `gorm:"column:summary"`
	Status            string       `gorm:"column:status"`
	FirstDetectedAt   time.Time    `gorm:"column:first_detected_at"`
	LastActiveAt      time.Time    `gorm:"column:last_active_at"`
	CurrentHeatScore  float64      `gorm:"column:current_heat_score"`
	TrendDirection    string       `gorm:"column:trend_direction"`
	RepresentativePostID *int64    `gorm:"column:representative_post_id"`
	CreatedAt         time.Time    `gorm:"column:created_at"`
	UpdatedAt         time.Time    `gorm:"column:updated_at"`
}

func (Topic) TableName() string { return "topics" }

type TopicPost struct {
	ID                int64     `gorm:"column:id;primaryKey"`
	TopicID           int64     `gorm:"column:topic_id"`
	PostID            int64     `gorm:"column:post_id"`
	MembershipScore   float64   `gorm:"column:membership_score"`
	IsRepresentative  bool      `gorm:"column:is_representative"`
	AddedAt           time.Time `gorm:"column:added_at"`
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

type TopicDailyExport struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	MonitorID    int64      `gorm:"column:monitor_id"`
	TopicID      int64      `gorm:"column:topic_id"`
	ExportDate   string     `gorm:"column:export_date"`
	SummaryText  string     `gorm:"column:summary_text"`
	MarkdownPath string     `gorm:"column:markdown_path"`
	Status       string     `gorm:"column:status"`
	ErrorMessage string     `gorm:"column:error_message"`
	PublishedAt  *time.Time `gorm:"column:published_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
}

func (TopicDailyExport) TableName() string { return "topic_daily_exports" }

type Alert struct {
	ID           int64     `gorm:"column:id;primaryKey"`
	MonitorID    int64     `gorm:"column:monitor_id"`
	TopicID      *int64    `gorm:"column:topic_id"`
	AlertType    string    `gorm:"column:alert_type"`
	Title        string    `gorm:"column:title"`
	Message      string    `gorm:"column:message"`
	Severity     string    `gorm:"column:severity"`
	TriggerScore float64   `gorm:"column:trigger_score"`
	TriggerReason string   `gorm:"column:trigger_reason"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (Alert) TableName() string { return "alerts" }

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

type EmailDelivery struct {
	ID                int64      `gorm:"column:id;primaryKey"`
	NotificationID    int64      `gorm:"column:notification_id"`
	RecipientEmail    string     `gorm:"column:recipient_email"`
	Provider          string     `gorm:"column:provider"`
	ProviderMessageID string     `gorm:"column:provider_message_id"`
	Status            string     `gorm:"column:status"`
	ErrorMessage      string     `gorm:"column:error_message"`
	SentAt            *time.Time `gorm:"column:sent_at"`
}

func (EmailDelivery) TableName() string { return "email_deliveries" }

type KnowledgeWritebackLog struct {
	ID             int64            `gorm:"column:id;primaryKey"`
	ObjectType     string           `gorm:"column:object_type"`
	ObjectID       int64            `gorm:"column:object_id"`
	FieldName      string           `gorm:"column:field_name"`
	FieldValue     pkg.JSONB[string] `gorm:"column:field_value;type:jsonb"`
	Status         string           `gorm:"column:status"`
	ConflictReason string           `gorm:"column:conflict_reason"`
	SourcePath     string           `gorm:"column:source_path"`
	CreatedAt      time.Time        `gorm:"column:created_at"`
}

func (KnowledgeWritebackLog) TableName() string { return "knowledge_writeback_logs" }

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
	DateStart *time.Time `gorm:"column:date_start;type:date"`
	DateEnd   *time.Time `gorm:"column:date_end;type:date"`
	Status    string     `gorm:"column:status"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

func (ExportBundle) TableName() string { return "export_bundles" }

type EventAnnotation struct {
	ID                int64            `gorm:"column:id;primaryKey"`
	EventID           int64            `gorm:"column:event_id"`
	ManualTags        pkg.JSONB[string] `gorm:"column:manual_tags;type:jsonb"`
	AnalystConclusion string           `gorm:"column:analyst_conclusion"`
	CreatedAt         time.Time        `gorm:"column:created_at"`
	UpdatedAt         time.Time        `gorm:"column:updated_at"`
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
	ID         int64   `gorm:"column:id;primaryKey"`
	ThemeID    int64   `gorm:"column:theme_id"`
	EventID    *int64  `gorm:"column:event_id"`
	TopicID    *int64  `gorm:"column:topic_id"`
	SourceKind string  `gorm:"column:source_kind"`
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
	ID         int64        `gorm:"column:id;primaryKey"`
	Name       string       `gorm:"column:name"`
	HeatScore  float64      `gorm:"column:heat_score"`
	Platform   string       `gorm:"column:platform"`
	Trend      string       `gorm:"column:trend"`
	FirstSeenAt time.Time   `gorm:"column:first_seen_at"`
	LastSeenAt time.Time    `gorm:"column:last_seen_at"`
	PeakAt     *time.Time   `gorm:"column:peak_at"`
	TopicIDs   pkg.Int64Array `gorm:"column:topic_ids;type:bigint[]"`
	PostIDs    pkg.Int64Array `gorm:"column:post_ids;type:bigint[]"`
	Summary    string       `gorm:"column:summary"`
	Category   string       `gorm:"column:category"`
	Status     string       `gorm:"column:status"`
	CreatedAt  time.Time    `gorm:"column:created_at"`
	UpdatedAt  time.Time    `gorm:"column:updated_at"`
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

// ──────────────────────────────────────────────
// Conversion helpers: GORM model ↔ business model
// ──────────────────────────────────────────────

func ToUser(m User) model.User {
	return model.User{
		ID:           m.ID,
		Email:        m.Email,
		PasswordHash: m.PasswordHash,
		DisplayName:  m.DisplayName,
		Status:       m.Status,
		PlanType:     m.PlanType,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func FromUser(m model.User) User {
	return User{
		ID:           m.ID,
		Email:        m.Email,
		PasswordHash: m.PasswordHash,
		DisplayName:  m.DisplayName,
		Status:       m.Status,
		PlanType:     m.PlanType,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func ToKeywordMonitor(m KeywordMonitor) model.KeywordMonitor {
	return model.KeywordMonitor{
		ID:                   m.ID,
		UserID:               m.UserID,
		Name:                 m.Name,
		QueryText:            m.QueryText,
		Language:             m.Language,
		Region:               m.Region,
		Status:               m.Status,
		PollIntervalMinutes:  m.PollIntervalMinutes,
		AlertEnabled:         m.AlertEnabled,
		AlertThresholdConfig: m.AlertThresholdConfig,
		LastPolledAt:         m.LastPolledAt,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

func FromKeywordMonitor(m model.KeywordMonitor) KeywordMonitor {
	return KeywordMonitor{
		ID:                   m.ID,
		UserID:               m.UserID,
		Name:                 m.Name,
		QueryText:            m.QueryText,
		Language:             m.Language,
		Region:               m.Region,
		Status:               m.Status,
		PollIntervalMinutes:  m.PollIntervalMinutes,
		AlertEnabled:         m.AlertEnabled,
		AlertThresholdConfig: m.AlertThresholdConfig,
		LastPolledAt:         m.LastPolledAt,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

func ToPlatformPost(m PlatformPost) model.PlatformPost {
	return model.PlatformPost{
		ID:               m.ID,
		Platform:         m.Platform,
		PlatformPostID:   m.PlatformPostID,
		AuthorPlatformID: m.AuthorPlatformID,
		AuthorName:       m.AuthorName,
		AuthorHandle:     m.AuthorHandle,
		ContentText:      m.ContentText,
		ContentLang:      m.ContentLang,
		PostURL:          m.PostURL,
		PublishedAt:      func() time.Time { if m.PublishedAt != nil { return *m.PublishedAt }; return time.Time{} }(),
		LikeCount:        m.LikeCount,
		ReplyCount:       m.ReplyCount,
		RepostCount:      m.RepostCount,
		QuoteCount:       m.QuoteCount,
		ViewCount:        m.ViewCount,
		NormalizedHash:   m.NormalizedHash,
	}
}

func ToEvent(m Event) model.Event {
	return model.Event{
		ID:            m.ID,
		MonitorID:     m.MonitorID,
		EventKey:      m.EventKey,
		Title:         m.Title,
		Summary:       m.Summary,
		MachineStatus: m.MachineStatus,
		SourcePostID:  m.SourcePostID,
		FirstSeenAt:   m.FirstSeenAt,
		LastActiveAt:  m.LastActiveAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

func FromEvent(m model.Event) Event {
	return Event{
		ID:            m.ID,
		MonitorID:     m.MonitorID,
		EventKey:      m.EventKey,
		Title:         m.Title,
		Summary:       m.Summary,
		MachineStatus: m.MachineStatus,
		SourcePostID:  m.SourcePostID,
		FirstSeenAt:   m.FirstSeenAt,
		LastActiveAt:  m.LastActiveAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

func ToKnowledgeRun(m KnowledgeRun) model.KnowledgeRun {
	return model.KnowledgeRun{
		ID:           m.ID,
		RunKey:       m.RunKey,
		RunType:      m.RunType,
		TargetDate:   m.TargetDate,
		Status:       m.Status,
		ErrorMessage: m.ErrorMessage,
		StartedAt:    m.StartedAt,
		FinishedAt:   m.FinishedAt,
		CreatedAt:    m.CreatedAt,
	}
}

func ToHotEvent(m HotEvent) model.HotEvent {
	return model.HotEvent{
		ID:          m.ID,
		Name:        m.Name,
		HeatScore:   m.HeatScore,
		Platform:    m.Platform,
		Trend:       m.Trend,
		FirstSeenAt: m.FirstSeenAt,
		LastSeenAt:  m.LastSeenAt,
		PeakAt:      m.PeakAt,
		TopicIDs:    m.TopicIDs,
		PostIDs:     m.PostIDs,
		Summary:     m.Summary,
		Category:    m.Category,
		Status:      m.Status,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func FromHotEvent(m model.HotEvent) HotEvent {
	return HotEvent{
		ID:          m.ID,
		Name:        m.Name,
		HeatScore:   m.HeatScore,
		Platform:    m.Platform,
		Trend:       m.Trend,
		FirstSeenAt: m.FirstSeenAt,
		LastSeenAt:  m.LastSeenAt,
		PeakAt:      m.PeakAt,
		TopicIDs:    m.TopicIDs,
		PostIDs:     m.PostIDs,
		Summary:     m.Summary,
		Category:    m.Category,
		Status:      m.Status,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func ToNotification(m UserNotification) model.Notification {
	return model.Notification{
		ID:             m.ID,
		UserID:         m.UserID,
		AlertID:        m.AlertID,
		Channel:        m.Channel,
		DeliveryStatus: m.DeliveryStatus,
		ReadAt:         m.ReadAt,
		SentAt:         m.SentAt,
		CreatedAt:      m.CreatedAt,
	}
}

func ToTopicSnapshot(m TopicSnapshot) model.TopicSnapshot {
	return model.TopicSnapshot{
		ID:                m.ID,
		TopicID:           m.TopicID,
		SnapshotTime:      m.SnapshotTime,
		PostCount:         m.PostCount,
		UniqueAuthorCount: m.UniqueAuthorCount,
		EngagementSum:     m.EngagementSum,
		HeatScore:         m.HeatScore,
		TrendVelocity:     m.TrendVelocity,
	}
}

func ToMonitorSnapshot(m MonitorSnapshot) model.MonitorSnapshot {
	return model.MonitorSnapshot{
		ID:               m.ID,
		MonitorID:        m.MonitorID,
		SnapshotTime:     m.SnapshotTime,
		NewPostCount:     m.NewPostCount,
		ActiveTopicCount: m.ActiveTopicCount,
		TotalEngagement:  m.TotalEngagement,
		TopTopicID:       m.TopTopicID,
	}
}

func ToTopicDailyExport(m TopicDailyExport) model.TopicDailyExport {
	return model.TopicDailyExport{
		ID:           m.ID,
		MonitorID:    m.MonitorID,
		TopicID:      m.TopicID,
		ExportDate:   m.ExportDate,
		SummaryText:  m.SummaryText,
		MarkdownPath: m.MarkdownPath,
		Status:       m.Status,
		ErrorMessage: m.ErrorMessage,
		PublishedAt:  m.PublishedAt,
		CreatedAt:    m.CreatedAt,
	}
}
