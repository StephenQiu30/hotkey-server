package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/hackernews"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/rss"
	xconnector "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/x"
)

func TestCollectionRequestBudgetStatusMapsEveryP0ProfileWithoutConsumingARequest(t *testing.T) {
	at := time.Date(2026, time.August, 29, 13, 5, 0, 0, time.UTC)
	config := domain.DefaultSourceConfig()
	config.RateLimitPerMinute = 17
	tests := []struct {
		name           string
		connection     domain.SourceConnection
		profileVersion string
		dailyLimit     int64
	}{
		{
			name: "rss", connection: domain.SourceConnection{ID: 1, SourceType: domain.SourceTypeRSS, Name: "RSS", Endpoint: "https://feeds.example.test/rss", AuthType: domain.AuthTypeNone, Config: config, Enabled: true},
			profileVersion: rss.ResourceLimitProfileVersion, dailyLimit: rss.DefaultResourceLimitProfile().DailyRequestQuota,
		},
		{
			name: "hacker news", connection: domain.SourceConnection{ID: 2, SourceType: domain.SourceTypeHackerNews, Name: "HN", Endpoint: domain.HackerNewsEndpoint, AuthType: domain.AuthTypeNone, Config: config, Enabled: true},
			profileVersion: hackernews.ResourceLimitProfileVersion, dailyLimit: hackernews.DefaultResourceLimitProfile().DailyRequestQuota,
		},
		{
			name: "x", connection: domain.SourceConnection{ID: 3, SourceType: domain.SourceTypeX, Name: "X", Endpoint: domain.XRecentSearchEndpoint, AuthType: domain.AuthTypeBearer, CredentialRef: domain.ManagedCredentialReference, Config: config, Enabled: true, HealthStatus: domain.HealthStatusHealthy},
			profileVersion: xconnector.ResourceLimitProfileVersion, dailyLimit: xconnector.DefaultResourceLimitProfile().DailyRequestQuota,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &budgetStatusReaderFake{}
			status, err := NewCollectionRequestBudgetStatus(reader)
			if err != nil {
				t.Fatal(err)
			}
			available, err := status.CollectionRequestAvailable(context.Background(), test.connection, at)
			if err != nil || !available {
				t.Fatalf("CollectionRequestAvailable() = %t/%v", available, err)
			}
			reservation := reader.reservation
			if reader.calls != 1 || reservation.SourceConnectionID != test.connection.ID || reservation.ResourceProfileVersion != test.profileVersion ||
				reservation.DailyLimit != test.dailyLimit || reservation.PerMinuteLimit != 17 || !reservation.At.Equal(at) {
				t.Fatalf("P0 budget preflight reservation = %#v, calls=%d", reservation, reader.calls)
			}
		})
	}
}

type budgetStatusReaderFake struct {
	reservation domain.ExternalRequestBudgetReservation
	calls       int
}

func (reader *budgetStatusReaderFake) CheckExternalRequest(_ context.Context, reservation domain.ExternalRequestBudgetReservation) (domain.ExternalRequestBudgetAvailability, error) {
	reader.calls++
	reader.reservation = reservation
	return domain.ExternalRequestBudgetAvailability{Allowed: true, ResetAt: reservation.At.UTC().Add(24 * time.Hour)}, nil
}
