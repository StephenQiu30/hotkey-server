package fakejobs

import (
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// PostCandidateProvider is a fake implementing jobs.PostCandidateProvider.
type PostCandidateProvider struct {
	Posts []jobs.PostCandidate
	Err   error
}

func (p *PostCandidateProvider) GetUnclusteredPosts(_ int64) ([]jobs.PostCandidate, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	return p.Posts, nil
}

// PostLink records a topic-post association.
type PostLink struct {
	TopicID int64
	PostID  int64
}

// TopicPersister is a fake implementing jobs.TopicPersister.
type TopicPersister struct {
	Err      error
	Topics   []topic.Topic
	PostLinks []PostLink
	nextID   int64
}

func (p *TopicPersister) UpsertTopic(_ int64, t topic.Topic) (int64, error) {
	if p.Err != nil {
		return 0, p.Err
	}
	p.nextID++
	p.Topics = append(p.Topics, t)
	return p.nextID, nil
}

func (p *TopicPersister) AddPostToTopic(topicID, postID int64, _ float64) error {
	if p.Err != nil {
		return p.Err
	}
	p.PostLinks = append(p.PostLinks, PostLink{TopicID: topicID, PostID: postID})
	return nil
}
