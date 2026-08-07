package infrastructure

import (
	"context"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
)

func TestConnectorRegistryBindsOnlyKnownSourceTypes(t *testing.T) {
	resolver, err := sourcenet.NewResolver("")
	if err != nil {
		t.Fatalf("NewResolver(): %v", err)
	}
	registry := NewConnectorRegistry(resolver)
	groundingConfig := domain.DefaultSourceConfig()
	groundingConfig.RequiresAttribution = true
	groundingConfig.GroundingDataBoundaryApproved = true
	weiboConfig := domain.DefaultSourceConfig()
	weiboConfig.RequiresAttribution = true
	weiboConfig.RequiresDeletionSync = true
	for _, connection := range []domain.SourceConnection{
		{ID: 1, SourceType: domain.SourceTypeRSS, Name: "RSS", Endpoint: "https://feeds.example.test/rss", AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusUnknown},
		{ID: 2, SourceType: domain.SourceTypeHackerNews, Name: "HN", Endpoint: domain.HackerNewsEndpoint, AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusUnknown},
		{ID: 3, SourceType: domain.SourceTypeX, Name: "X", Endpoint: domain.XRecentSearchEndpoint, AuthType: domain.AuthTypeBearer, CredentialRef: "env:X_BEARER_TOKEN", Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusHealthy},
		{ID: 4, SourceType: domain.SourceTypeBingGrounding, Name: "Foundry Web Search", Endpoint: "https://hotkey.services.ai.azure.com/api/projects/hotkey/toolboxes/web-search/versions/1/mcp?api-version=v1", AuthType: domain.AuthTypeBearer, CredentialRef: "env:AZURE_FOUNDRY_TOKEN", Config: groundingConfig, Enabled: true, HealthStatus: domain.HealthStatusHealthy, TermsPolicyURL: "https://learn.microsoft.com/azure/foundry/web-search"},
		{ID: 5, SourceType: domain.SourceTypeWeibo, Name: "Weibo", Endpoint: domain.WeiboCLIApiEndpoint, AuthType: domain.AuthTypeBearer, CredentialRef: "env:WEIBO_TOKEN", Config: weiboConfig, Enabled: true, HealthStatus: domain.HealthStatusHealthy, TermsPolicyURL: domain.WeiboDeveloperTerms},
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
