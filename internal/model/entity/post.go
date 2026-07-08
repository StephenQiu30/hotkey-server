package entity

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

type PlatformPost struct {
	ID               int64             `gorm:"column:id;primaryKey"`
	Platform         string            `gorm:"column:platform"`
	PlatformPostID   string            `gorm:"column:platform_post_id"`
	AuthorPlatformID string            `gorm:"column:author_platform_id"`
	AuthorName       string            `gorm:"column:author_name"`
	AuthorHandle     string            `gorm:"column:author_handle"`
	ContentText      string            `gorm:"column:content_text"`
	ContentLang      string            `gorm:"column:content_lang"`
	PostURL          string            `gorm:"column:post_url"`
	PublishedAt      *time.Time        `gorm:"column:published_at"`
	LikeCount        int               `gorm:"column:like_count"`
	ReplyCount       int               `gorm:"column:reply_count"`
	RepostCount      int               `gorm:"column:repost_count"`
	QuoteCount       int               `gorm:"column:quote_count"`
	ViewCount        int               `gorm:"column:view_count"`
	RawPayload       pkg.JSONB[string] `gorm:"column:raw_payload;type:jsonb"`
	NormalizedHash   string            `gorm:"column:normalized_hash"`
	Embedding        *pkg.Vector384    `gorm:"type:vector(384);column:embedding"`
	CreatedAt        time.Time         `gorm:"column:created_at"`
	UpdatedAt        time.Time         `gorm:"column:updated_at"`
}

func (PlatformPost) TableName() string { return "platform_posts" }

type PlatformAuthor struct {
	ID               int64             `gorm:"column:id;primaryKey"`
	Platform         string            `gorm:"column:platform"`
	PlatformAuthorID string            `gorm:"column:platform_author_id"`
	Handle           string            `gorm:"column:handle"`
	DisplayName      string            `gorm:"column:display_name"`
	FollowersCount   int               `gorm:"column:followers_count"`
	Verified         bool              `gorm:"column:verified"`
	RawPayload       pkg.JSONB[string] `gorm:"column:raw_payload;type:jsonb"`
	UpdatedAt        time.Time         `gorm:"column:updated_at"`
}

func (PlatformAuthor) TableName() string { return "platform_authors" }
