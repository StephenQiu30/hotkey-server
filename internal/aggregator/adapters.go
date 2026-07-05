package aggregator

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// TopicQueryRepo implements TopicProvider via GORM.
type TopicQueryRepo struct {
	db *gorm.DB
}

func NewTopicQueryRepo(db *gorm.DB) *TopicQueryRepo {
	return &TopicQueryRepo{db: db}
}

func (r *TopicQueryRepo) GetRecentTopics(ctx context.Context, since time.Time) ([]TopicBrief, error) {
	var topics []TopicBrief
	err := r.db.WithContext(ctx).
		Model(&topicModel{}).
		Where("updated_at >= ? AND status = 'active'", since).
		Select("id, monitor_id, topic_key AS key, title, current_heat_score AS heat, updated_at AS seen_at").
		Limit(200).
		Find(&topics).Error
	return topics, err
}

// topicModel is a minimal GORM model for the topics table.
type topicModel struct {
	ID        int64     `gorm:"column:id"`
	MonitorID int64     `gorm:"column:monitor_id"`
	TopicKey  string    `gorm:"column:topic_key"`
	Title     string    `gorm:"column:title"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (topicModel) TableName() string { return "topics" }

// TrendingPostQueryRepo implements TrendingPostProvider via GORM.
type TrendingPostQueryRepo struct {
	db *gorm.DB
}

func NewTrendingPostQueryRepo(db *gorm.DB) *TrendingPostQueryRepo {
	return &TrendingPostQueryRepo{db: db}
}

func (r *TrendingPostQueryRepo) GetRecentTrendingPosts(ctx context.Context, since time.Time) ([]TrendingPostBrief, error) {
	var posts []TrendingPostBrief
	err := r.db.WithContext(ctx).
		Model(&trendingPostModel{}).
		Where("platform IN ('weibo', 'zhihu', 'baidu') AND updated_at >= ?", since).
		Select("id, platform, content_text AS title, like_count AS heat, updated_at AS seen_at").
		Limit(200).
		Find(&posts).Error
	return posts, err
}

type trendingPostModel struct {
	ID int64 `gorm:"column:id"`
	Platform string `gorm:"column:platform"`
	ContentText string `gorm:"column:content_text"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (trendingPostModel) TableName() string { return "platform_posts" }
