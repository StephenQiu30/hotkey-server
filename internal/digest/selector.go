package digest

import "time"

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

// TopicFilter abstracts the data access for digest topic queries.
// Implementations query the database for topics and posts within a day window.
type TopicFilter interface {
	// ListTopicsForDay returns active topics for a monitor that have posts
	// within the given window, ordered by current_heat_score DESC.
	ListTopicsForDay(monitorID int64, window Window) ([]TopicEntry, error)

	// FetchRepresentativePosts returns up to limit posts for a topic,
	// ordered by membership_score DESC.
	FetchRepresentativePosts(topicID int64, limit int) ([]PostEntry, error)
}

// Service provides digest selection operations.
type Service struct {
	filter TopicFilter
}

// NewService creates a digest Service with the given TopicFilter.
func NewService(f TopicFilter) *Service {
	return &Service{filter: f}
}

// SelectTopicsForDay returns the top N active topics for a monitor on the
// given CST date, sorted by current_heat_score DESC.
func (s *Service) SelectTopicsForDay(monitorID int64, date time.Time, topN int) ([]TopicEntry, error) {
	window := DayWindow(date)

	topics, err := s.filter.ListTopicsForDay(monitorID, window)
	if err != nil {
		return nil, err
	}

	if len(topics) > topN {
		topics = topics[:topN]
	}
	return topics, nil
}

// SelectRepresentativePosts returns up to limit representative posts for a topic.
func (s *Service) SelectRepresentativePosts(topicID int64, limit int) ([]PostEntry, error) {
	return s.filter.FetchRepresentativePosts(topicID, limit)
}
