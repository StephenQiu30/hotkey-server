package source

import "testing"

func TestSourcesCanBeIsolatedByTenant(t *testing.T) {
	service := NewEmptyService()
	if err := service.RegisterSource(Source{
		TenantID:               "tenant-alpha",
		ID:                     "custom-feed",
		Name:                   "Alpha Feed",
		Layer:                  LayerFact,
		Region:                 "global",
		Language:               "en",
		Categories:             []string{"ai"},
		AccessMode:             AccessModePublicFeed,
		RateLimitPerHour:       10,
		RefreshIntervalMinutes: 60,
	}); err != nil {
		t.Fatalf("register alpha source: %v", err)
	}
	if err := service.RegisterSource(Source{
		TenantID:               "tenant-beta",
		ID:                     "custom-feed",
		Name:                   "Beta Feed",
		Layer:                  LayerSignal,
		Region:                 "global",
		Language:               "en",
		Categories:             []string{"ai"},
		AccessMode:             AccessModePublicFeed,
		RateLimitPerHour:       20,
		RefreshIntervalMinutes: 120,
	}); err != nil {
		t.Fatalf("register beta source: %v", err)
	}

	alphaSources := service.ListSourcesByTenant("tenant-alpha")
	if len(alphaSources) != 1 || alphaSources[0].Name != "Alpha Feed" {
		t.Fatalf("alpha sources = %#v", alphaSources)
	}
	betaSources := service.ListSourcesByTenant("tenant-beta")
	if len(betaSources) != 1 || betaSources[0].Name != "Beta Feed" {
		t.Fatalf("beta sources = %#v", betaSources)
	}

	enabled := false
	updated, err := service.UpdateTenantSourceConfig("tenant-beta", "custom-feed", UpdateSourceConfigInput{Enabled: &enabled})
	if err != nil {
		t.Fatalf("update beta source: %v", err)
	}
	if updated.Enabled {
		t.Fatalf("beta source enabled = true, want false")
	}
	if service.ListSourcesByTenant("tenant-alpha")[0].Enabled != true {
		t.Fatalf("alpha source should not be updated by beta tenant")
	}
}
