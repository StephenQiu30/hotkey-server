// Package jobs implements background job orchestration.
package jobs

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

type PostCandidate struct {
	PostID int64
	Text   string
}

type PostCandidateProvider interface {
	GetUnclusteredPosts(monitorID int64) ([]PostCandidate, error)
}

type TopicPersister interface {
	UpsertTopic(monitorID int64, t topic.Topic) (topicID int64, err error)
	AddPostToTopic(topicID, postID int64, membershipScore float64) error
}

type AggregateTopicsInput struct {
	MonitorID int64
	RunTime   time.Time
}

type AggregateTopicsResult struct {
	TopicsCreated int
	PostsClustered int
}

type AggregateTopicsJob struct {
	provider  PostCandidateProvider
	persister TopicPersister
}

func NewAggregateTopicsJob(provider PostCandidateProvider, persister TopicPersister) *AggregateTopicsJob {
	return &AggregateTopicsJob{provider: provider, persister: persister}
}

// Run executes topic aggregation for a monitor.
func (j *AggregateTopicsJob) Run(in AggregateTopicsInput) (AggregateTopicsResult, error) {
	posts, err := j.provider.GetUnclusteredPosts(in.MonitorID)
	if err != nil {
		return AggregateTopicsResult{}, err
	}

	if len(posts) == 0 {
		return AggregateTopicsResult{}, nil
	}

	candidates := make([]topic.CandidatePost, 0, len(posts))
	for _, p := range posts {
		candidates = append(candidates, topic.CandidatePost{
			PostID: p.PostID,
			Tokens: topic.ExtractTokens(p.Text),
		})
	}

	svc := topic.NewService(nil)
	topics := svc.Cluster(candidates)

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
