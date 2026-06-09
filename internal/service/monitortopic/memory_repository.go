package monitortopic

import (
	"context"
	"sync"
)

// MemoryRepository is an in-memory implementation of Repository.
type MemoryRepository struct {
	mu         sync.RWMutex
	topics     map[string]MonitorTopic
	topicOrder []string
	byUser     map[string][]string
	keywords   map[string]TopicKeyword
	kwByTopic  map[string][]string
}

// NewMemoryRepository creates a new in-memory repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		topics:    make(map[string]MonitorTopic),
		byUser:    make(map[string][]string),
		keywords:  make(map[string]TopicKeyword),
		kwByTopic: make(map[string][]string),
	}
}

func (r *MemoryRepository) CreateTopic(_ context.Context, topic MonitorTopic) (MonitorTopic, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.topics[topic.ID] = topic
	r.topicOrder = append(r.topicOrder, topic.ID)
	r.byUser[topic.UserID] = append(r.byUser[topic.UserID], topic.ID)
	return topic, nil
}

func (r *MemoryRepository) TopicByID(_ context.Context, topicID string) (MonitorTopic, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	topic, exists := r.topics[topicID]
	if !exists {
		return MonitorTopic{}, ErrNotFound
	}
	return topic, nil
}

func (r *MemoryRepository) ListTopics(_ context.Context, userID string) ([]MonitorTopic, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var topics []MonitorTopic
	for _, id := range r.byUser[userID] {
		if topic, exists := r.topics[id]; exists {
			topics = append(topics, topic)
		}
	}
	return topics, nil
}

func (r *MemoryRepository) UpdateTopic(_ context.Context, topic MonitorTopic) (MonitorTopic, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.topics[topic.ID]; !exists {
		return MonitorTopic{}, ErrNotFound
	}
	r.topics[topic.ID] = topic
	return topic, nil
}

func (r *MemoryRepository) DeleteTopic(_ context.Context, topicID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	topic, exists := r.topics[topicID]
	if !exists {
		return ErrNotFound
	}
	delete(r.topics, topicID)
	for i, id := range r.topicOrder {
		if id == topicID {
			r.topicOrder = append(r.topicOrder[:i], r.topicOrder[i+1:]...)
			break
		}
	}
	// Remove from user index
	userTopics := r.byUser[topic.UserID]
	for i, id := range userTopics {
		if id == topicID {
			r.byUser[topic.UserID] = append(userTopics[:i], userTopics[i+1:]...)
			break
		}
	}
	// Cascade delete keywords
	for _, kwID := range r.kwByTopic[topicID] {
		delete(r.keywords, kwID)
	}
	delete(r.kwByTopic, topicID)
	return nil
}

func (r *MemoryRepository) CreateKeyword(_ context.Context, kw TopicKeyword) (TopicKeyword, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.topics[kw.TopicID]; !exists {
		return TopicKeyword{}, ErrNotFound
	}
	r.keywords[kw.ID] = kw
	r.kwByTopic[kw.TopicID] = append(r.kwByTopic[kw.TopicID], kw.ID)
	return kw, nil
}

func (r *MemoryRepository) ListKeywords(_ context.Context, topicID string) ([]TopicKeyword, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var keywords []TopicKeyword
	for _, id := range r.kwByTopic[topicID] {
		if kw, exists := r.keywords[id]; exists {
			keywords = append(keywords, kw)
		}
	}
	return keywords, nil
}

func (r *MemoryRepository) DeleteKeyword(_ context.Context, keywordID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	kw, exists := r.keywords[keywordID]
	if !exists {
		return ErrNotFound
	}
	delete(r.keywords, keywordID)
	topicKWs := r.kwByTopic[kw.TopicID]
	for i, id := range topicKWs {
		if id == keywordID {
			r.kwByTopic[kw.TopicID] = append(topicKWs[:i], topicKWs[i+1:]...)
			break
		}
	}
	return nil
}

func (r *MemoryRepository) CountKeywords(_ context.Context, topicID string, kwType KeywordType) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, id := range r.kwByTopic[topicID] {
		if kw, exists := r.keywords[id]; exists && kw.Type == kwType {
			count++
		}
	}
	return count, nil
}
