package llm

import "context"

// TopicSummaryInput holds the data needed to generate a topic summary.
type TopicSummaryInput struct {
	MonitorName string      // keyword monitor display name
	QueryWords  []string    // monitor query terms
	TopicTitle  string      // topic title
	TopicKey    string      // topic key (e.g. "ai:监管:政策")
	Heat        float64     // current heat score
	Trend       string      // trend direction: rising / stable / falling
	PostCount   int         // number of posts in topic
	Posts       []PostInput // representative posts (top N)
}

// PostInput is a single post to include in the summarization prompt.
type PostInput struct {
	Author  string
	Content string
	URL     string
}

// Client generates summaries for daily digest topics.
type Client interface {
	SummarizeTopic(ctx context.Context, in TopicSummaryInput) (string, error)
}
