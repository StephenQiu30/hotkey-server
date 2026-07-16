package infrastructure

import (
	"context"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

func TestConnectorRegistryBindsOnlyKnownSourceTypes(t *testing.T) {
	registry := NewConnectorRegistry()
	for _, connection := range []domain.SourceConnection{
		{ID: 1, SourceType: domain.SourceTypeRSS, Name: "RSS", Endpoint: "https://feeds.example.test/rss", AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusUnknown},
		{ID: 2, SourceType: domain.SourceTypeHackerNews, Name: "HN", Endpoint: domain.HackerNewsEndpoint, AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusUnknown},
	} {
		connector, err := registry.Resolve(context.Background(), connection)
		if err != nil || connector == nil {
			t.Fatalf("Resolve(%q) connector/error = %T / %v", connection.SourceType, connector, err)
		}
	}
	if _, err := registry.Resolve(context.Background(), domain.SourceConnection{SourceType: domain.SourceType("unknown")}); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent || strings.Contains(err.Error(), "unknown") {
		t.Fatalf("Resolve(unknown) error = %v, want safe permanent error", err)
	}
}
