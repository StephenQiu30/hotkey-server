package worker

import (
	"context"
	"time"
)

type RunRepository interface {
	TryStart(ctx context.Context, runKey string, runType string, targetDate time.Time, startedAt time.Time) (bool, error)
	MarkFinished(ctx context.Context, runKey string, finishedAt time.Time) error
	MarkFailed(ctx context.Context, runKey string, message string, failedAt time.Time) error
}
