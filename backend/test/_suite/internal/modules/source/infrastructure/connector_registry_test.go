package infrastructure

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
)

func TestConnectorRegistryBindsOnlyKnownSourceTypes(t *testing.T) {
	resolver, err := sourcenet.NewResolver("")
	if err != nil {
		t.Fatalf("NewResolver(): %v", err)
	}
	registry := NewConnectorRegistry(resolver, nil, allowingExternalRequestBudget{})
	groundingConfig := domain.DefaultSourceConfig()
	groundingConfig.RequiresAttribution = true
	groundingConfig.GroundingDataBoundaryApproved = true
	weiboConfig := domain.DefaultSourceConfig()
	weiboConfig.RequiresAttribution = true
	weiboConfig.RequiresDeletionSync = true
	googleConfig := domain.DefaultSourceConfig()
	googleConfig.RequiresAttribution = true
	googleConfig.GoogleLocation = "global"
	googleConfig.GoogleServingConfig = "projects/hotkey-demo/locations/global/collections/default_collection/dataStores/news/servingConfigs/default_config"
	for _, connection := range []domain.SourceConnection{
		{ID: 1, SourceType: domain.SourceTypeRSS, Name: "RSS", Endpoint: "https://feeds.example.test/rss", AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusUnknown},
		{ID: 2, SourceType: domain.SourceTypeHackerNews, Name: "HN", Endpoint: domain.HackerNewsEndpoint, AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusUnknown},
		{ID: 3, SourceType: domain.SourceTypeX, Name: "X", Endpoint: domain.XRecentSearchEndpoint, AuthType: domain.AuthTypeBearer, CredentialRef: "env:X_BEARER_TOKEN", Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusHealthy},
		{ID: 4, SourceType: domain.SourceTypeBingGrounding, Name: "Foundry Web Search", Endpoint: "https://hotkey.services.ai.azure.com/api/projects/hotkey/toolboxes/web-search/versions/1/mcp?api-version=v1", AuthType: domain.AuthTypeBearer, CredentialRef: "env:AZURE_FOUNDRY_TOKEN", Config: groundingConfig, Enabled: true, HealthStatus: domain.HealthStatusHealthy, TermsPolicyURL: "https://learn.microsoft.com/azure/foundry/web-search"},
		{ID: 5, SourceType: domain.SourceTypeWeibo, Name: "Weibo", Endpoint: domain.WeiboCLIApiEndpoint, AuthType: domain.AuthTypeBearer, CredentialRef: "env:WEIBO_TOKEN", Config: weiboConfig, Enabled: true, HealthStatus: domain.HealthStatusHealthy, TermsPolicyURL: domain.WeiboDeveloperTerms},
		{ID: 6, SourceType: domain.SourceTypeGoogleAgentSearch, Name: "Google Agent Search", Endpoint: domain.GoogleAgentSearchGlobalEndpoint, AuthType: domain.AuthTypeBearer, CredentialRef: "env:GOOGLE_AGENT_SEARCH_TOKEN", Config: googleConfig, Enabled: true, HealthStatus: domain.HealthStatusHealthy, TermsPolicyURL: domain.GoogleCloudTerms},
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

func TestConnectorRegistryFailsClosedWhenP0RequestBudgetIsUnavailable(t *testing.T) {
	resolver, err := sourcenet.NewResolver("")
	if err != nil {
		t.Fatal(err)
	}
	connections := []domain.SourceConnection{
		{ID: 1, SourceType: domain.SourceTypeRSS, Name: "RSS", Endpoint: "https://feeds.example.test/rss", AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true},
		{ID: 2, SourceType: domain.SourceTypeHackerNews, Name: "HN", Endpoint: domain.HackerNewsEndpoint, AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true},
		{ID: 3, SourceType: domain.SourceTypeX, Name: "X", Endpoint: domain.XRecentSearchEndpoint, AuthType: domain.AuthTypeBearer, CredentialRef: "env:X_BEARER_TOKEN", Config: domain.DefaultSourceConfig(), Enabled: true},
	}
	for _, connection := range connections {
		if _, err := NewConnectorRegistry(resolver, nil, nil).Resolve(context.Background(), connection); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
			t.Fatalf("missing %s request budget error = %v", connection.SourceType, err)
		}
	}
}

func TestConnectorRegistryResolvesManagedCredentialWithoutChangingSourceFact(t *testing.T) {
	resolver, err := sourcenet.NewResolver("")
	if err != nil {
		t.Fatalf("NewResolver(): %v", err)
	}
	credentials := &credentialStoreFake{value: "managed-x-token"}
	registry := NewConnectorRegistry(resolver, credentials, allowingExternalRequestBudget{})
	connection := domain.SourceConnection{
		ID: 9, SourceType: domain.SourceTypeX, Name: "Managed X", Endpoint: domain.XRecentSearchEndpoint,
		AuthType: domain.AuthTypeBearer, CredentialRef: domain.ManagedCredentialReference,
		Config: domain.DefaultSourceConfig(), Enabled: false, HealthStatus: domain.HealthStatusUnknown,
	}
	connector, err := registry.Resolve(context.Background(), connection)
	if err != nil {
		t.Fatalf("Resolve(managed) error = %v", err)
	}
	if credentials.resolvedID != 0 {
		t.Fatalf("credential decrypted during connector resolution for source %d", credentials.resolvedID)
	}
	if err := connector.Validate(context.Background(), connection); err != nil {
		t.Fatalf("Validate(original managed fact) error = %v", err)
	}
	if connection.CredentialRef != domain.ManagedCredentialReference {
		t.Fatalf("registry mutated source credential reference to %q", connection.CredentialRef)
	}

	unavailable, err := NewConnectorRegistry(resolver, nil, allowingExternalRequestBudget{}).Resolve(context.Background(), connection)
	if err != nil {
		t.Fatalf("Resolve(unconfigured store) error = %v", err)
	}
	health := unavailable.Health(context.Background(), connection)
	if health.Healthy || health.ErrorKind != domain.CollectionErrorAuthentication || health.DiagnosticCode != "credential_unavailable" {
		t.Fatalf("managed credential unavailable health = %#v", health)
	}
}

func TestConnectorRegistryDefersManagedCredentialDecryptionUntilTheRequestBoundary(t *testing.T) {
	resolver, err := sourcenet.NewResolver("")
	if err != nil {
		t.Fatal(err)
	}
	credentials := &credentialStoreFake{value: "managed-x-token"}
	connection := domain.SourceConnection{
		ID: 19, SourceType: domain.SourceTypeX, Name: "Lazy managed X", Endpoint: domain.XRecentSearchEndpoint,
		AuthType: domain.AuthTypeBearer, CredentialRef: domain.ManagedCredentialReference,
		Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusHealthy,
	}
	connector, err := NewConnectorRegistry(resolver, credentials, allowingExternalRequestBudget{}).Resolve(context.Background(), connection)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.resolvedID != 0 {
		t.Fatalf("managed credential decrypted during connector resolution for source %d", credentials.resolvedID)
	}
	if err := connector.Validate(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	if credentials.resolvedID != 0 {
		t.Fatalf("managed credential decrypted during static validation for source %d", credentials.resolvedID)
	}
}

type credentialStoreFake struct {
	value      string
	resolvedID int64
}

type allowingExternalRequestBudget struct{}

func (allowingExternalRequestBudget) ReserveExternalRequest(_ context.Context, reservation domain.ExternalRequestBudgetReservation) (domain.ExternalRequestBudgetDecision, error) {
	return domain.ExternalRequestBudgetDecision{Allowed: true, Used: 1, ResetAt: reservation.At.UTC().Add(24 * time.Hour)}, nil
}

func (*credentialStoreFake) Store(context.Context, int64, string, int64) error { return nil }
func (*credentialStoreFake) Delete(context.Context, int64) error               { return nil }
func (store *credentialStoreFake) Resolve(_ context.Context, sourceID int64) (string, error) {
	store.resolvedID = sourceID
	return store.value, nil
}
