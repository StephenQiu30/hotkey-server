package rss

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/sourcenet"
)

func TestExternalFeedHealthWithConfiguredDoH(t *testing.T) {
	feedURL := os.Getenv("HOTKEY_SOURCE_EXTERNAL_TEST_URL")
	dohURL := os.Getenv("HOTKEY_SOURCE_DOH_URL")
	if feedURL == "" || dohURL == "" {
		t.Skip("external source test requires HOTKEY_SOURCE_EXTERNAL_TEST_URL and HOTKEY_SOURCE_DOH_URL")
	}

	resolver, err := sourcenet.NewResolver(dohURL)
	if err != nil {
		t.Fatalf("NewResolver(): %v", err)
	}
	connection := domain.SourceConnection{
		ID:           1,
		SourceType:   domain.SourceTypeRSS,
		Name:         "External RSS fixture",
		Endpoint:     feedURL,
		AuthType:     domain.AuthTypeNone,
		Config:       domain.DefaultSourceConfig(),
		Enabled:      true,
		HealthStatus: domain.HealthStatusUnknown,
	}
	connector, err := New(connection, resolver)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result := connector.Health(ctx, connection)
	if !result.Healthy {
		t.Fatalf("Health() = %#v, want healthy external feed", result)
	}
}
