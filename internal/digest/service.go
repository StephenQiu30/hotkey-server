// Package digest provides topic selection for daily digest exports.
// This is a stub implementation; full logic will be added in STE-304.
package digest

import (
	"context"
	"database/sql"
	"time"
)

// TopicCandidate represents a topic eligible for daily export.
type TopicCandidate struct {
	TopicID        int64
	TopicKey       string
	Title          string
	HeatScore      float64
	TrendDirection string
	PostCount      int
	Posts          []RepresentativePost
}

// RepresentativePost holds a top post for a topic.
type RepresentativePost struct {
	AuthorName string
	Text       string
	URL        string
}

// Service provides topic selection for a given day.
type Service struct {
	db *sql.DB
}

// NewService creates a digest Service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// ListTopicsForDay returns topics with activity on the given export date.
// TODO(STE-304): implement real topic selection logic.
func (s *Service) ListTopicsForDay(_ context.Context, _ int64, _ time.Time, _ int) ([]TopicCandidate, error) {
	return nil, nil
}
