package repository

import (
	"context"
)

type AnnotationRepository interface {
	UpsertEventAnnotation(ctx context.Context, eventID int64, manualTags, analystConclusion string) error
	UpsertTopicAnnotation(ctx context.Context, topicID int64, materialStatus, manualSummary string) error
}
