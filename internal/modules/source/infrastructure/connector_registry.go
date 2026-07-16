// Package infrastructure wires Source protocol adapters without exposing
// their selection to collection request input.
package infrastructure

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/hackernews"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/rss"
)

// ConnectorRegistry constructs a connector bound to one immutable
// SourceConnection. Connector constructors retain endpoint validation, so the
// registry is only a type dispatcher and never accepts request-supplied URLs.
type ConnectorRegistry struct{}

var _ domain.CollectionConnectorRegistry = (*ConnectorRegistry)(nil)

func NewConnectorRegistry() *ConnectorRegistry { return &ConnectorRegistry{} }

func (*ConnectorRegistry) Resolve(_ context.Context, connection domain.SourceConnection) (domain.Connector, error) {
	switch connection.SourceType {
	case domain.SourceTypeRSS:
		return rss.New(connection)
	case domain.SourceTypeHackerNews:
		return hackernews.New(connection)
	default:
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, fmt.Errorf("unsupported collection source type"))
	}
}
