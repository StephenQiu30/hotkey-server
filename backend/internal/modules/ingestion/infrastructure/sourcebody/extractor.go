// Package sourcebody dispatches already-verified evidence to the extractor
// that owns its frozen format contract.
package sourcebody

import (
	"context"
	"errors"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
)

type Extractor struct {
	feed     ingestionapplication.SelectedSourceBodyExtractor
	platform ingestionapplication.SelectedSourceBodyExtractor
}

func NewExtractor(feed, platform ingestionapplication.SelectedSourceBodyExtractor) (*Extractor, error) {
	if feed == nil || platform == nil {
		return nil, errors.New("feed and platform body extractors are required")
	}
	return &Extractor{feed: feed, platform: platform}, nil
}

func (extractor *Extractor) Extract(ctx context.Context, command ingestionapplication.ExtractSelectedSourceBodyCommand) (ingestionapplication.ExtractSelectedSourceBodyResult, error) {
	if extractor == nil || extractor.feed == nil || extractor.platform == nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, errors.New("source body extractor is not initialized")
	}
	if command.Evidence.SourceCode == "rss" {
		return extractor.feed.Extract(ctx, command)
	}
	return extractor.platform.Extract(ctx, command)
}
