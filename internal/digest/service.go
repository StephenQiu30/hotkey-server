package digest

import (
	"context"
	"time"
)

const DefaultTopN = 20
const DefaultRepresentativeLimit = 3

// DayDigest holds the result of a daily digest selection.
type DayDigest struct {
	ExportDate  time.Time
	Topics      []TopicDigest
	Events      []EventDigest
	EventsCount int
}

// TopicDigest pairs a topic with its representative posts.
type TopicDigest struct {
	Topic TopicEntry
	Posts []PostEntry
}

// EventDigest pairs an event with its basic info.
type EventDigest struct {
	Event EventEntry
}

// BuildDayDigest orchestrates the full digest selection for a monitor:
// 1. Resolve the export date from now + target
// 2. Select top N topics for that day
// 3. Fetch representative posts for each topic
// 4. Select top N hot events for that day
func (s *Service) BuildDayDigest(ctx context.Context, monitorID int64, now time.Time, target string, topN int) (*DayDigest, error) {
	if topN <= 0 {
		topN = DefaultTopN
	}

	exportDate := ResolveExportDate(now, target)

	topics, err := s.SelectTopicsForDay(ctx, monitorID, exportDate, topN)
	if err != nil {
		return nil, err
	}

	result := &DayDigest{
		ExportDate: exportDate,
		Topics:     make([]TopicDigest, 0, len(topics)),
	}

	for _, t := range topics {
		posts, err := s.SelectRepresentativePosts(ctx, t.ID, DefaultRepresentativeLimit)
		if err != nil {
			return nil, err
		}
		result.Topics = append(result.Topics, TopicDigest{
			Topic: t,
			Posts: posts,
		})
	}

	// Fetch hot events for this day
	events, err := s.SelectEventsForDay(ctx, exportDate, topN)
	if err != nil {
		return nil, err
	}
	result.Events = make([]EventDigest, len(events))
	for i, ev := range events {
		result.Events[i] = EventDigest{Event: ev}
	}
	result.EventsCount = len(events)

	return result, nil
}
