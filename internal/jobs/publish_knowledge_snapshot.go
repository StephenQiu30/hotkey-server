package jobs

import (
	"context"
	"time"
)

// DigestResult holds topic digests for one knowledge run.
type DigestResult struct {
	Topics []TopicDigest
}

// TopicDigest represents a single topic in the digest output.
type TopicDigest struct {
	ID    int64
	Title string
}

// EventSnapshot represents an event assembled for export.
type EventSnapshot struct {
	ID    int64
	Title string
}

// KnowledgeRunResult holds the outcome of a knowledge sync run.
type KnowledgeRunResult struct {
	EventsPublished int
}

// digestBuilder builds digest from monitor data.
type digestBuilder interface {
	BuildDigest(ctx context.Context, now time.Time) (DigestResult, error)
}

// eventAssembler builds events from topic digests.
type eventAssembler interface {
	BuildEvents(ctx context.Context, topics []TopicDigest) ([]EventSnapshot, error)
}

// knowledgeExporter publishes knowledge objects.
type knowledgeExporter interface {
	Publish(ctx context.Context, digest DigestResult, events []EventSnapshot) (KnowledgeRunResult, error)
}

// PublishKnowledgeSnapshotJob orchestrates the knowledge sync pipeline:
// digest selection → event assembly → export.
type PublishKnowledgeSnapshotJob struct {
	digestSvc digestBuilder
	events    eventAssembler
	exporter  knowledgeExporter
}

// NewPublishKnowledgeSnapshotJob creates a new knowledge snapshot job.
func NewPublishKnowledgeSnapshotJob(
	digestSvc digestBuilder,
	events eventAssembler,
	exporter knowledgeExporter,
) *PublishKnowledgeSnapshotJob {
	return &PublishKnowledgeSnapshotJob{
		digestSvc: digestSvc,
		events:    events,
		exporter:  exporter,
	}
}

// Run executes the knowledge sync pipeline.
func (j *PublishKnowledgeSnapshotJob) Run(ctx context.Context, now time.Time) (KnowledgeRunResult, error) {
	digestResult, err := j.digestSvc.BuildDigest(ctx, now)
	if err != nil {
		return KnowledgeRunResult{}, err
	}

	events, err := j.events.BuildEvents(ctx, digestResult.Topics)
	if err != nil {
		return KnowledgeRunResult{}, err
	}

	return j.exporter.Publish(ctx, digestResult, events)
}
