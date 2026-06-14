package digest

import "time"

const DefaultTopN = 20
const DefaultRepresentativeLimit = 3

// DayDigest holds the result of a daily digest selection.
type DayDigest struct {
	ExportDate time.Time
	Topics     []TopicDigest
}

// TopicDigest pairs a topic with its representative posts.
type TopicDigest struct {
	Topic TopicEntry
	Posts []PostEntry
}

// BuildDayDigest orchestrates the full digest selection for a monitor:
// 1. Resolve the export date from now + target
// 2. Select top N topics for that day
// 3. Fetch representative posts for each topic
func (s *Service) BuildDayDigest(monitorID int64, now time.Time, target string, topN int) (*DayDigest, error) {
	if topN <= 0 {
		topN = DefaultTopN
	}

	exportDate := ResolveExportDate(now, target)

	topics, err := s.SelectTopicsForDay(monitorID, exportDate, topN)
	if err != nil {
		return nil, err
	}

	result := &DayDigest{
		ExportDate: exportDate,
		Topics:     make([]TopicDigest, 0, len(topics)),
	}

	for _, t := range topics {
		posts, err := s.SelectRepresentativePosts(t.ID, DefaultRepresentativeLimit)
		if err != nil {
			return nil, err
		}
		result.Topics = append(result.Topics, TopicDigest{
			Topic: t,
			Posts: posts,
		})
	}

	return result, nil
}
