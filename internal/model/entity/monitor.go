package entity

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

type KeywordMonitor struct {
	ID                   int64                     `gorm:"column:id;primaryKey"`
	UserID               int64                     `gorm:"column:user_id"`
	Name                 string                    `gorm:"column:name"`
	QueryText            string                    `gorm:"column:query_text"`
	Language             string                    `gorm:"column:language"`
	Region               string                    `gorm:"column:region"`
	Status               string                    `gorm:"column:status"`
	PollIntervalMinutes  int                       `gorm:"column:poll_interval_minutes"`
	AlertEnabled         bool                      `gorm:"column:alert_enabled"`
	AlertThresholdConfig pkg.JSONB[map[string]any] `gorm:"column:alert_threshold_config;type:jsonb"`
	LastPolledAt         *time.Time                `gorm:"column:last_polled_at"`
	QueryEmbedding       *pkg.Vector384            `gorm:"type:vector(384);column:query_embedding"`
	CreatedAt            time.Time                 `gorm:"column:created_at"`
	UpdatedAt            time.Time                 `gorm:"column:updated_at"`
}

func (KeywordMonitor) TableName() string { return "keyword_monitors" }

type MonitorRun struct {
	ID             int64             `gorm:"column:id;primaryKey"`
	MonitorID      int64             `gorm:"column:monitor_id"`
	Platform       string            `gorm:"column:platform"`
	RunType        string            `gorm:"column:run_type"`
	Status         string            `gorm:"column:status"`
	StartedAt      time.Time         `gorm:"column:started_at"`
	FinishedAt     *time.Time        `gorm:"column:finished_at"`
	FetchedCount   int               `gorm:"column:fetched_count"`
	StoredCount    int               `gorm:"column:stored_count"`
	ErrorMessage   string            `gorm:"column:error_message"`
	CursorSnapshot pkg.JSONB[string] `gorm:"column:cursor_snapshot;type:jsonb"`
}

func (MonitorRun) TableName() string { return "monitor_runs" }

type MonitorPostHit struct {
	ID                   int64               `gorm:"column:id;primaryKey"`
	MonitorID            int64               `gorm:"column:monitor_id"`
	PostID               int64               `gorm:"column:post_id"`
	MatchedKeywords      pkg.JSONB[[]string] `gorm:"column:matched_keywords;type:jsonb"`
	RelevanceScore       float64             `gorm:"column:relevance_score"`
	HeatScore            float64             `gorm:"column:heat_score"`
	FreshnessScore       float64             `gorm:"column:freshness_score"`
	AuthorInfluenceScore float64             `gorm:"column:author_influence_score"`
	FinalScore           float64             `gorm:"column:final_score"`
	FirstSeenAt          time.Time           `gorm:"column:first_seen_at"`
	LastSeenAt           time.Time           `gorm:"column:last_seen_at"`
}

func (MonitorPostHit) TableName() string { return "monitor_post_hits" }
