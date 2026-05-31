package channel

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu               sync.RWMutex
	channels         map[string]Channel
	channelOrder     []string
	subscriptions    map[string]map[string]Subscription
	keywords         map[string]Keyword
	keywordsByUser   map[string][]string
	settings         map[string]string
	userDailySendAts map[string]string
}

func NewMemoryRepository() *MemoryRepository {
	repo := &MemoryRepository{
		channels:         make(map[string]Channel),
		subscriptions:    make(map[string]map[string]Subscription),
		keywords:         make(map[string]Keyword),
		keywordsByUser:   make(map[string][]string),
		settings:         map[string]string{defaultDailySendAtKey: defaultDailySendAt},
		userDailySendAts: make(map[string]string),
	}
	now := time.Now().UTC()
	for _, seed := range []Channel{
		{ID: "chn_ai_models", Name: "AI 模型", Slug: "ai-models", Description: "AI 模型发布、能力更新与评测", Status: ChannelStatusActive, CreatedAt: now, UpdatedAt: now},
		{ID: "chn_ai_products", Name: "AI 产品", Slug: "ai-products", Description: "AI 产品发布、增长与使用场景", Status: ChannelStatusActive, CreatedAt: now, UpdatedAt: now},
		{ID: "chn_ai_open_source", Name: "AI 开源", Slug: "ai-open-source", Description: "AI 开源项目、框架与社区动态", Status: ChannelStatusActive, CreatedAt: now, UpdatedAt: now},
		{ID: "chn_ai_funding", Name: "AI 投融资", Slug: "ai-funding", Description: "AI 公司融资、并购与资本动态", Status: ChannelStatusActive, CreatedAt: now, UpdatedAt: now},
	} {
		repo.channels[seed.ID] = seed
		repo.channelOrder = append(repo.channelOrder, seed.ID)
	}
	return repo
}

func (r *MemoryRepository) ListChannels(_ context.Context, activeOnly bool) ([]Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var channels []Channel
	for _, id := range r.channelOrder {
		channel, exists := r.channels[id]
		if !exists {
			continue
		}
		if activeOnly && channel.Status != ChannelStatusActive {
			continue
		}
		channels = append(channels, channel)
	}
	return channels, nil
}

func (r *MemoryRepository) ChannelByID(_ context.Context, channelID string) (Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	channel, exists := r.channels[channelID]
	if !exists {
		return Channel{}, sql.ErrNoRows
	}
	return channel, nil
}

func (r *MemoryRepository) CreateChannel(_ context.Context, channel Channel) (Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.channels[channel.ID]; exists {
		return Channel{}, ErrAlreadyExists
	}
	for _, existing := range r.channels {
		if existing.Slug == channel.Slug {
			return Channel{}, ErrAlreadyExists
		}
	}
	r.channels[channel.ID] = channel
	r.channelOrder = append(r.channelOrder, channel.ID)
	return channel, nil
}

func (r *MemoryRepository) UpdateChannel(_ context.Context, channel Channel) (Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.channels[channel.ID]; !exists {
		return Channel{}, sql.ErrNoRows
	}
	r.channels[channel.ID] = channel
	return channel, nil
}

func (r *MemoryRepository) DeleteChannel(_ context.Context, channelID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.channels[channelID]; !exists {
		return sql.ErrNoRows
	}
	delete(r.channels, channelID)
	for i, id := range r.channelOrder {
		if id == channelID {
			r.channelOrder = append(r.channelOrder[:i], r.channelOrder[i+1:]...)
			break
		}
	}
	for userID := range r.subscriptions {
		delete(r.subscriptions[userID], channelID)
	}
	return nil
}

func (r *MemoryRepository) UpsertSubscription(_ context.Context, userID string, channelID string, createdAt time.Time) (Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	channel, exists := r.channels[channelID]
	if !exists {
		return Subscription{}, sql.ErrNoRows
	}
	if r.subscriptions[userID] == nil {
		r.subscriptions[userID] = make(map[string]Subscription)
	}
	subscription := Subscription{UserID: userID, Channel: channel, CreatedAt: createdAt}
	if existing, exists := r.subscriptions[userID][channelID]; exists {
		subscription.CreatedAt = existing.CreatedAt
	}
	r.subscriptions[userID][channelID] = subscription
	return subscription, nil
}

func (r *MemoryRepository) DeleteSubscription(_ context.Context, userID string, channelID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.subscriptions[userID][channelID]; !exists {
		return sql.ErrNoRows
	}
	delete(r.subscriptions[userID], channelID)
	return nil
}

func (r *MemoryRepository) ListSubscriptions(_ context.Context, userID string) ([]Subscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var subscriptions []Subscription
	for _, channelID := range r.channelOrder {
		subscription, exists := r.subscriptions[userID][channelID]
		if !exists {
			continue
		}
		channel, exists := r.channels[channelID]
		if !exists {
			continue
		}
		subscription.Channel = channel
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions, nil
}

func (r *MemoryRepository) CreateKeyword(_ context.Context, keyword Keyword) (Keyword, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keywords[keyword.ID] = keyword
	r.keywordsByUser[keyword.UserID] = append(r.keywordsByUser[keyword.UserID], keyword.ID)
	return keyword, nil
}

func (r *MemoryRepository) UpdateKeyword(_ context.Context, keyword Keyword) (Keyword, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.keywords[keyword.ID]; !exists {
		return Keyword{}, sql.ErrNoRows
	}
	r.keywords[keyword.ID] = keyword
	return keyword, nil
}

func (r *MemoryRepository) KeywordByID(_ context.Context, userID string, keywordID string) (Keyword, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keyword, exists := r.keywords[keywordID]
	if !exists || keyword.UserID != userID {
		return Keyword{}, sql.ErrNoRows
	}
	return keyword, nil
}

func (r *MemoryRepository) DeleteKeyword(_ context.Context, userID string, keywordID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	keyword, exists := r.keywords[keywordID]
	if !exists || keyword.UserID != userID {
		return sql.ErrNoRows
	}
	delete(r.keywords, keywordID)
	return nil
}

func (r *MemoryRepository) ListKeywords(_ context.Context, userID string) ([]Keyword, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var keywords []Keyword
	for _, id := range r.keywordsByUser[userID] {
		keyword, exists := r.keywords[id]
		if exists {
			keywords = append(keywords, keyword)
		}
	}
	return keywords, nil
}

func (r *MemoryRepository) Setting(_ context.Context, key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, exists := r.settings[key]
	if !exists {
		return "", sql.ErrNoRows
	}
	return value, nil
}

func (r *MemoryRepository) UpsertSetting(_ context.Context, key string, value string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settings[key] = value
	return nil
}

func (r *MemoryRepository) UserDailySendAt(_ context.Context, userID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, exists := r.userDailySendAts[userID]
	if !exists {
		value = defaultDailySendAt
	}
	return value, nil
}

func (r *MemoryRepository) SetUserDailySendAt(_ context.Context, userID string, dailySendAt string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userDailySendAts[userID] = dailySendAt
	return nil
}
