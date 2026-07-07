package cache

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/StephenQiu30/hotkey-server/internal/model"
)

// HotEventCache caches HotEvent lookups.
type HotEventCache struct {
	*Cache[model.HotEvent]
}

func NewHotEventCache(client *redis.Client) *HotEventCache {
	return &HotEventCache{
		Cache: NewCache[model.HotEvent](client, "hot_event", 5*time.Minute),
	}
}

// TopicCache caches topic summaries.
type TopicCache struct {
	*Cache[model.TopicSummary]
}

func NewTopicCache(client *redis.Client) *TopicCache {
	return &TopicCache{
		Cache: NewCache[model.TopicSummary](client, "topic", 5*time.Minute),
	}
}
