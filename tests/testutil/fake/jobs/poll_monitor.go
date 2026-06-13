package fakejobs

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
)

// APIError is a test error sentinel with a status code.
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api error %d: %s", e.Code, e.Message)
}

// RunRepo is a fake implementing jobs.RunRepository.
type RunRepo struct {
	CreateCount     int
	CreateMonitorID int64
	UpdateCount     int
	UpdateRunID     int64
	UpdateStatus    string
	LastRun         jobs.MonitorRun
	UpdateErr       error
	CreateErr       error
	nextRunID       int64
}

func (r *RunRepo) CreateRun(_ context.Context, run jobs.MonitorRun) (int64, error) {
	if r.CreateErr != nil {
		return 0, r.CreateErr
	}
	r.CreateCount++
	r.CreateMonitorID = run.MonitorID
	r.nextRunID++
	return r.nextRunID, nil
}

func (r *RunRepo) UpdateRun(_ context.Context, runID int64, run jobs.MonitorRun) error {
	if r.UpdateErr != nil {
		return r.UpdateErr
	}
	r.UpdateCount++
	r.UpdateRunID = runID
	r.UpdateStatus = run.Status
	r.LastRun = run
	return nil
}

// Connector is a fake implementing jobs.PlatformConnector.
type Connector struct {
	Posts      []jobs.PostResult
	NextCursor string
	Err        error
}

func (c *Connector) SearchPosts(_ context.Context, _ string, _ string) ([]jobs.PostResult, string, error) {
	if c.Err != nil {
		return nil, "", c.Err
	}
	return c.Posts, c.NextCursor, nil
}

// PostRepo is a fake implementing jobs.PostRepository.
type PostRepo struct {
	Err    error
	nextID int64
}

func (r *PostRepo) UpsertPost(_ context.Context, _ jobs.PostResult) (int64, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.nextID++
	return r.nextID, nil
}

// HitRepo is a fake implementing jobs.HitRepository.
type HitRepo struct {
	Hits []jobs.HitResult
	Err  error
}

func (r *HitRepo) UpsertHit(_ context.Context, hit jobs.HitResult) error {
	if r.Err != nil {
		return r.Err
	}
	r.Hits = append(r.Hits, hit)
	return nil
}

// ScorerCall records a single invocation of ScoreHit.
type ScorerCall struct {
	PostID              int64
	Post                jobs.PostResult
	MatchedKeywords     []string
	TotalKeywords       int
	PublishedMinutesAgo float64
}

// Scorer is a fake implementing jobs.HitScorer.
type Scorer struct {
	Calls []ScorerCall
	Err   error
}

func (s *Scorer) ScoreHit(hitID int64, post jobs.PostResult, matchedKeywords []string, totalKeywords int, publishedMinutesAgo float64) error {
	s.Calls = append(s.Calls, ScorerCall{
		PostID:              hitID,
		Post:                post,
		MatchedKeywords:     matchedKeywords,
		TotalKeywords:       totalKeywords,
		PublishedMinutesAgo: publishedMinutesAgo,
	})
	return s.Err
}
