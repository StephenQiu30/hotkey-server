package application

import (
	"context"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

// CapturedItemReaderDependencies contains only Source's collection boundary.
// Ingestion receives this application projection rather than a Source
// repository or a connector implementation.
type CapturedItemReaderDependencies struct {
	Runs domain.CollectionRepository
}

// CapturedItemReader is the Source application adapter consumed by ingestion.
// It has no Content repository and cannot alter collection/target outcomes.
type CapturedItemReader struct {
	runs domain.CollectionRepository
}

var _ domain.CapturedItemReader = (*CapturedItemReader)(nil)

func NewCapturedItemReader(dependencies CapturedItemReaderDependencies) (*CapturedItemReader, error) {
	if dependencies.Runs == nil {
		return nil, errors.New("captured item reader requires a collection repository")
	}
	return &CapturedItemReader{runs: dependencies.Runs}, nil
}

func (reader *CapturedItemReader) ListUnboundCaptured(ctx context.Context, query domain.CapturedItemQuery) (domain.CapturedItemPage, error) {
	if reader == nil || reader.runs == nil {
		return domain.CapturedItemPage{}, errors.New("captured item reader is not initialized")
	}
	return reader.runs.ListUnboundCaptured(ctx, query)
}

func (reader *CapturedItemReader) BindContent(ctx context.Context, binding domain.CapturedContentBinding) error {
	if reader == nil || reader.runs == nil {
		return errors.New("captured item reader is not initialized")
	}
	return reader.runs.BindContent(ctx, binding)
}

func (reader *CapturedItemReader) MarkIngestionFailure(ctx context.Context, failure domain.CapturedIngestionFailure) error {
	if reader == nil || reader.runs == nil {
		return errors.New("captured item reader is not initialized")
	}
	return reader.runs.MarkIngestionFailure(ctx, failure)
}
