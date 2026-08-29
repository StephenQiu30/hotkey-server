package infrastructure

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/hackernews"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/rss"
	xconnector "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/x"
)

// CollectionRequestBudgetStatus performs the non-consuming Budget and Rate
// Limit preflight. Connectors still reserve atomically before every physical
// request, retry, and redirect.
type CollectionRequestBudgetStatus struct {
	reader domain.ExternalRequestBudgetStatusReader
}

func NewCollectionRequestBudgetStatus(reader domain.ExternalRequestBudgetStatusReader) (*CollectionRequestBudgetStatus, error) {
	if reader == nil {
		return nil, errors.New("external request budget status reader is required")
	}
	return &CollectionRequestBudgetStatus{reader: reader}, nil
}

func (status *CollectionRequestBudgetStatus) CollectionRequestAvailable(ctx context.Context, connection domain.SourceConnection, at time.Time) (bool, error) {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || at.IsZero() {
		return false, errors.New("collection request budget status input is invalid")
	}
	profileVersion, dailyLimit, applies := collectionRequestProfile(normalized.SourceType)
	if !applies {
		return true, nil
	}
	availability, err := status.reader.CheckExternalRequest(ctx, domain.ExternalRequestBudgetReservation{
		SourceConnectionID: normalized.ID, ResourceProfileVersion: profileVersion,
		DailyLimit: dailyLimit, PerMinuteLimit: int64(normalized.Config.RateLimitPerMinute), At: at.UTC(),
	})
	if err != nil {
		return false, err
	}
	if err := availability.Validate(domain.ExternalRequestBudgetReservation{
		SourceConnectionID: normalized.ID, ResourceProfileVersion: profileVersion,
		DailyLimit: dailyLimit, PerMinuteLimit: int64(normalized.Config.RateLimitPerMinute), At: at.UTC(),
	}); err != nil {
		return false, err
	}
	return availability.Allowed, nil
}

func collectionRequestProfile(sourceType domain.SourceType) (string, int64, bool) {
	switch sourceType {
	case domain.SourceTypeRSS:
		profile := rss.DefaultResourceLimitProfile()
		return profile.Version, profile.DailyRequestQuota, true
	case domain.SourceTypeHackerNews:
		profile := hackernews.DefaultResourceLimitProfile()
		return profile.Version, profile.DailyRequestQuota, true
	case domain.SourceTypeX:
		profile := xconnector.DefaultResourceLimitProfile()
		return profile.Version, profile.DailyRequestQuota, true
	default:
		return "", 0, false
	}
}
