// Package jobs implements background job orchestration for the hotkey-server.
// AggregateTopicsJob clusters posts from a monitor into topics.
package jobs

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// PostCandidate provides post data needed for topic clustering.
type PostCandidate struct {
	PostID int64
	Text   string
}

// PostCandidateProvider abstracts fetching unclustered posts for a monitor.
type PostCandidateProvider interface {
	GetUnclusteredPosts(monitorID int64) ([]PostCandidate, error)
}

// TopicPersister abstracts persisting topic clustering results.
type TopicPersister interface {
	UpsertTopic(monitorID int64, t topic.Topic) (topicID int64, err error)
	AddPostToTopic(topicID, postID int64, membershipScore float64) error
}

// AggregateTopicsInput holds parameters for a topic aggregation run.
type AggregateTopicsInput struct {
	MonitorID int64
	RunTime   time.Time
}

// AggregateTopicsResult holds the outcome of a topic aggregation run.
type AggregateTopicsResult struct {
	TopicsCreated int
	PostsClustered int
}

// AggregateTopicsJob orchestrates topic clustering for a monitor.
type AggregateTopicsJob struct {
	provider  PostCandidateProvider
	persister TopicPersister
}

// NewAggregateTopicsJob creates an AggregateTopicsJob.
func NewAggregateTopicsJob(provider PostCandidateProvider, persister TopicPersister) *AggregateTopicsJob {
	return &AggregateTopicsJob{provider: provider, persister: persister}
}

// Run executes topic aggregation for the given monitor.
func (j *AggregateTopicsJob) Run(in AggregateTopicsInput) (AggregateTopicsResult, error) {
	posts, err := j.provider.GetUnclusteredPosts(in.MonitorID)
	if err != nil {
		return AggregateTopicsResult{}, err
	}

	if len(posts) == 0 {
		return AggregateTopicsResult{}, nil
	}

	// Convert to CandidatePost for clustering
	candidates := make([]topic.CandidatePost, 0, len(posts))
	for _, p := range posts {
		candidates = append(candidates, topic.CandidatePost{
			PostID: p.PostID,
			Tokens: topic.ExtractTokens(p.Text),
		})
	}

	// Cluster posts into topics
	svc := topic.NewService(nil)
	topics := svc.Cluster(candidates)

	// Persist each topic and its member posts
	postsClustered := 0
	for _, t := range topics {
		topicID, err := j.persister.UpsertTopic(in.MonitorID, t)
		if err != nil {
			return AggregateTopicsResult{TopicsCreated: 0, PostsClustered: postsClustered}, err
		}

		for _, postID := range t.PostIDs {
			if err := j.persister.AddPostToTopic(topicID, postID, 0); err != nil {
				return AggregateTopicsResult{TopicsCreated: 0, PostsClustered: postsClustered}, err
			}
			postsClustered++
		}
	}

	return AggregateTopicsResult{
		TopicsCreated:  len(topics),
		PostsClustered: postsClustered,
	}, nil
}
