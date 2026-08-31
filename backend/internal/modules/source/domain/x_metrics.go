package domain

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

var xPostIDPattern = regexp.MustCompile(`^[0-9]{1,19}$`)

type XPostMetricLookupRequest struct {
	SourceConnectionID int64
	PostIDs            []string
}

func (request XPostMetricLookupRequest) Validate() error {
	if request.SourceConnectionID <= 0 || len(request.PostIDs) == 0 || len(request.PostIDs) > 100 {
		return fmt.Errorf("x metric lookup requires a source and 1-100 post IDs")
	}
	for _, postID := range request.PostIDs {
		if !xPostIDPattern.MatchString(postID) {
			return fmt.Errorf("x metric lookup post ID is invalid")
		}
	}
	return nil
}

type XPostMetricObservation struct {
	PostID     string
	Metrics    SourceMetrics
	CapturedAt time.Time
}

type XPostMetricLookupResult struct {
	Observations []XPostMetricObservation
	Snapshots    []EvidenceSnapshot
	RateLimit    RateLimit
	Diagnostics  []FetchDiagnostic
}

type XPostMetricLookup interface {
	LookupPostMetrics(context.Context, XPostMetricLookupRequest) (XPostMetricLookupResult, error)
}

// XMetricRefreshCandidateQuery is the bounded, source-scoped eligibility
// contract for known X posts. Implementations must return each content once
// even when it is relevant to more than one Monitor.
type XMetricRefreshCandidateQuery struct {
	SourceConnectionID int64
	PublishedAfter     time.Time
	SnapshotDueBefore  time.Time
	Limit              int
}

func (query XMetricRefreshCandidateQuery) Validate() error {
	if query.SourceConnectionID <= 0 || query.PublishedAfter.IsZero() || query.SnapshotDueBefore.IsZero() ||
		!query.SnapshotDueBefore.After(query.PublishedAfter) || query.Limit < 1 || query.Limit > 100 {
		return fmt.Errorf("x metric refresh candidate query is invalid")
	}
	return nil
}

type XMetricRefreshCandidate struct {
	ContentID int64
	PostID    string
}

func (candidate XMetricRefreshCandidate) Validate() error {
	if candidate.ContentID <= 0 || !xPostIDPattern.MatchString(candidate.PostID) {
		return fmt.Errorf("x metric refresh candidate is invalid")
	}
	return nil
}

type XMetricRefreshCandidateReader interface {
	ListXMetricRefreshCandidates(context.Context, XMetricRefreshCandidateQuery) ([]XMetricRefreshCandidate, error)
}

type XMetricRefreshSchedule struct {
	SourceConnectionID int64
	SourceVersion      int64
	IntervalMinutes    int
	DailyRequestBudget int
}

func (schedule XMetricRefreshSchedule) Validate() error {
	if schedule.SourceConnectionID <= 0 || schedule.SourceVersion <= 0 || schedule.IntervalMinutes < 15 || schedule.IntervalMinutes > 1440 ||
		schedule.DailyRequestBudget < 1 || schedule.DailyRequestBudget > 1440 {
		return fmt.Errorf("x metric refresh schedule is invalid")
	}
	return nil
}

type XMetricRefreshScheduleReader interface {
	ListXMetricRefreshSchedules(context.Context) ([]XMetricRefreshSchedule, error)
}
