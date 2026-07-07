package repository

import (
	"context"
)

type ThemeRepository interface {
	Create(ctx context.Context, monitorID int64, themeKey, title, summary string) (int64, error)
	AddMembership(ctx context.Context, themeID int64, sourceKind string, eventID, topicID *int64) error
}
