package digest

import (
	"context"
	"time"
)

// TopicEntry represents a topic candidate for the daily digest.
type TopicEntry struct {
	ID    int64
	Title string
	Heat  float64
}

// PostEntry represents a representative post for a topic.
type PostEntry struct {
	PostID          int64
	AuthorName      string
	ContentExcerpt  string
	PostURL         string
	MembershipScore float64
}

// EventEntry represents a hot event candidate for the daily digest.
type EventEntry struct {
	ID        int64
	Name      string
	HeatScore float64
	Platform  string
	Summary   string
	Trend     string
}

// TopicFilter abstracts the data access for digest topic queries.
type TopicFilter interface {
	ListTopicsForDay(ctx context.Context, monitorID int64, window Window) ([]TopicEntry, error)
	FetchRepresentativePosts(ctx context.Context, topicID int64, limit int) ([]PostEntry, error)
}

// EventFilter abstracts the data access for digest hot event queries.
type EventFilter interface {
	ListEventsForDay(ctx context.Context, window Window, topN int) ([]EventEntry, error)
}

// Service provides digest selection operations.
type Service struct {
	filter      TopicFilter
	eventFilter EventFilter
}

// NewService creates a digest Service with the given TopicFilter.
func NewService(filter TopicFilter) *Service {
	return &Service{filter: filter}
}

// NewServiceWithEvents creates a digest Service with topic and event filters.
func NewServiceWithEvents(filter TopicFilter, eventFilter EventFilter) *Service {
	return &Service{filter: filter, eventFilter: eventFilter}
}

// SelectTopicsForDay returns the top N active topics for a monitor on the
// given CST date, sorted by current_heat_score DESC.
func (s *Service) SelectTopicsForDay(ctx context.Context, monitorID int64, date time.Time, topN int) ([]TopicEntry, error) {
	window := DayWindow(date)
	topics, err := s.filter.ListTopicsForDay(ctx, monitorID, window)
	if err != nil {
		return nil, err
	}
	if len(topics) > topN {
		topics = topics[:topN]
	}
	return topics, nil
}

// SelectRepresentativePosts returns up to limit representative posts for a topic.
func (s *Service) SelectRepresentativePosts(ctx context.Context, topicID int64, limit int) ([]PostEntry, error) {
	return s.filter.FetchRepresentativePosts(ctx, topicID, limit)
}

// SelectEventsForDay returns the top N hot events within the given date window.
func (s *Service) SelectEventsForDay(ctx context.Context, date time.Time, topN int) ([]EventEntry, error) {
	if s.eventFilter == nil {
		return nil, nil
	}
	window := DayWindow(date)
	return s.eventFilter.ListEventsForDay(ctx, window, topN)
}
