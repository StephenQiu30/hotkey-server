package fakedigest

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/digest"
)

// TopicFilter is a fake implementing digest.TopicFilter.
type TopicFilter struct {
	Topics []digest.TopicEntry
	Posts  map[int64][]digest.PostEntry
	Err    error
}

func (f *TopicFilter) ListTopicsForDay(_ context.Context, _ int64, _ digest.Window) ([]digest.TopicEntry, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Topics, nil
}

func (f *TopicFilter) FetchRepresentativePosts(_ context.Context, topicID int64, limit int) ([]digest.PostEntry, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	posts := f.Posts[topicID]
	if len(posts) > limit {
		posts = posts[:limit]
	}
	return posts, nil
}
