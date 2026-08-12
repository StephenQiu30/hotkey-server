// Package infrastructure wires Source protocol adapters without exposing
// their selection to collection request input.
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/bilibili"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/binggrounding"
	googleagentsearch "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/googleagentsearch"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/hackernews"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/rss"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/weibo"
	xconnector "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/x"
)

// ConnectorRegistry constructs a connector bound to one immutable
// SourceConnection. Connector constructors retain endpoint validation, so the
// registry is only a type dispatcher and never accepts request-supplied URLs.
type ConnectorRegistry struct {
	resolver    sourcenet.Resolver
	credentials domain.ManagedCredentialStore
}

var _ domain.CollectionConnectorRegistry = (*ConnectorRegistry)(nil)

func NewConnectorRegistry(resolver sourcenet.Resolver, credentials ...domain.ManagedCredentialStore) *ConnectorRegistry {
	registry := &ConnectorRegistry{resolver: resolver}
	if len(credentials) > 0 {
		registry.credentials = credentials[0]
	}
	return registry
}

const managedCredentialEnvName = "HOTKEY_MANAGED_SOURCE_CREDENTIAL"

func (registry *ConnectorRegistry) Resolve(ctx context.Context, connection domain.SourceConnection) (domain.Connector, error) {
	if connection.CredentialRef != domain.ManagedCredentialReference {
		return registry.resolveConnector(connection, nil)
	}
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid managed source connection"))
	}
	managed := &managedCredentialConnector{source: normalized}
	if registry == nil || registry.credentials == nil {
		return managed, nil
	}
	secret, err := registry.credentials.Resolve(ctx, normalized.ID)
	if err != nil || secret == "" {
		return managed, nil
	}
	runtimeConnection := normalized
	runtimeConnection.CredentialRef = "env:" + managedCredentialEnvName
	lookup := func(name string) (string, bool) {
		if name != managedCredentialEnvName {
			return "", false
		}
		return secret, true
	}
	connector, err := registry.resolveConnector(runtimeConnection, lookup)
	if err != nil {
		return nil, err
	}
	managed.inner = connector
	managed.runtime = runtimeConnection
	return managed, nil
}

func (registry *ConnectorRegistry) resolveConnector(connection domain.SourceConnection, lookup func(string) (string, bool)) (domain.Connector, error) {
	switch connection.SourceType {
	case domain.SourceTypeRSS:
		return rss.New(connection, registry.resolver)
	case domain.SourceTypeHackerNews:
		return hackernews.New(connection, registry.resolver)
	case domain.SourceTypeX:
		if lookup != nil {
			return xconnector.NewWithCredentialLookup(connection, registry.resolver, lookup)
		}
		return xconnector.New(connection, registry.resolver)
	case domain.SourceTypeBingGrounding:
		if lookup != nil {
			return binggrounding.NewWithCredentialLookup(connection, registry.resolver, lookup)
		}
		return binggrounding.New(connection, registry.resolver)
	case domain.SourceTypeBilibili:
		if lookup != nil {
			return bilibili.NewWithCredentialLookup(connection, registry.resolver, lookup)
		}
		return bilibili.New(connection, registry.resolver)
	case domain.SourceTypeWeibo:
		if lookup != nil {
			return weibo.NewWithCredentialLookup(connection, registry.resolver, lookup)
		}
		return weibo.New(connection, registry.resolver)
	case domain.SourceTypeGoogleAgentSearch:
		if lookup != nil {
			return googleagentsearch.NewWithCredentialLookup(connection, registry.resolver, lookup)
		}
		return googleagentsearch.New(connection, registry.resolver)
	default:
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, fmt.Errorf("unsupported collection source type"))
	}
}

type managedCredentialConnector struct {
	source  domain.SourceConnection
	runtime domain.SourceConnection
	inner   domain.Connector
}

func (connector *managedCredentialConnector) Validate(ctx context.Context, connection domain.SourceConnection) error {
	if connector == nil || connection.ID != connector.source.ID || connection.CredentialRef != domain.ManagedCredentialReference {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("managed source connection does not match connector"))
	}
	if connector.inner == nil {
		return nil
	}
	translated := connection
	translated.CredentialRef = connector.runtime.CredentialRef
	return connector.inner.Validate(ctx, translated)
}

func (connector *managedCredentialConnector) Fetch(ctx context.Context, request domain.FetchRequest) (domain.FetchResult, error) {
	if connector == nil || connector.inner == nil {
		return domain.FetchResult{}, domain.NewCollectionError(domain.CollectionErrorAuthentication, errors.New("source credential is unavailable"))
	}
	return connector.inner.Fetch(ctx, request)
}

func (connector *managedCredentialConnector) LookupPostMetrics(ctx context.Context, request domain.XPostMetricLookupRequest) (domain.XPostMetricLookupResult, error) {
	if connector == nil || connector.inner == nil {
		return domain.XPostMetricLookupResult{}, domain.NewCollectionError(domain.CollectionErrorAuthentication, errors.New("source credential is unavailable"))
	}
	lookup, ok := connector.inner.(domain.XPostMetricLookup)
	if !ok {
		return domain.XPostMetricLookupResult{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("source does not support X metric lookup"))
	}
	return lookup.LookupPostMetrics(ctx, request)
}

func (connector *managedCredentialConnector) Health(ctx context.Context, connection domain.SourceConnection) domain.HealthResult {
	if err := connector.Validate(ctx, connection); err != nil {
		return domain.HealthResult{CheckedAt: time.Now().UTC(), ErrorKind: domain.CollectionErrorPermanent, DiagnosticCode: "invalid_source_connection"}
	}
	if connector.inner == nil {
		return domain.HealthResult{CheckedAt: time.Now().UTC(), ErrorKind: domain.CollectionErrorAuthentication, DiagnosticCode: "credential_unavailable"}
	}
	translated := connection
	translated.CredentialRef = connector.runtime.CredentialRef
	return connector.inner.Health(ctx, translated)
}
