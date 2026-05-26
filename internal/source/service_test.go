package source

import "testing"

func TestNewServiceSeedsForeignFactAndSignalSources(t *testing.T) {
	service := NewService()

	sources := service.ListSources()
	if len(sources) < 2 {
		t.Fatalf("sources len = %d, want at least 2", len(sources))
	}

	var hasForeignFact bool
	var hasSignal bool
	for _, src := range sources {
		if src.Layer == LayerFact && src.Region == "global" && src.Enabled {
			hasForeignFact = true
		}
		if src.Layer == LayerSignal && src.Enabled {
			hasSignal = true
		}
		if src.RateLimitPerHour <= 0 {
			t.Fatalf("source %s rateLimitPerHour = %d, want positive", src.ID, src.RateLimitPerHour)
		}
		if src.RefreshIntervalMinutes <= 0 {
			t.Fatalf("source %s refreshIntervalMinutes = %d, want positive", src.ID, src.RefreshIntervalMinutes)
		}
	}

	if !hasForeignFact {
		t.Fatalf("missing enabled global fact source")
	}
	if !hasSignal {
		t.Fatalf("missing enabled signal source")
	}
}

func TestRegisterSourceRejectsBypassCollection(t *testing.T) {
	service := NewService()

	err := service.RegisterSource(Source{
		ID:                     "blocked",
		Name:                   "Blocked Source",
		Layer:                  LayerFact,
		Region:                 "global",
		Language:               "en",
		Categories:             []string{"ai"},
		AccessMode:             AccessModeBypass,
		Enabled:                true,
		RateLimitPerHour:       60,
		RefreshIntervalMinutes: 120,
	})

	if err == nil {
		t.Fatalf("RegisterSource accepted bypass collection")
	}
	if err != ErrNonCompliantSource {
		t.Fatalf("RegisterSource error = %v, want %v", err, ErrNonCompliantSource)
	}
}

func TestUpdateSourceConfigCanDisableAndThrottleSource(t *testing.T) {
	service := NewService()

	enabled := false
	rateLimit := 12
	updated, err := service.UpdateSourceConfig("arxiv-ai", UpdateSourceConfigInput{
		Enabled:          &enabled,
		RateLimitPerHour: &rateLimit,
	})
	if err != nil {
		t.Fatalf("UpdateSourceConfig returned error: %v", err)
	}

	if updated.Enabled {
		t.Fatalf("updated.Enabled = true, want false")
	}
	if updated.RateLimitPerHour != 12 {
		t.Fatalf("rateLimitPerHour = %d, want 12", updated.RateLimitPerHour)
	}
}

func TestRecordSourceFailureDoesNotAffectOtherSources(t *testing.T) {
	service := NewService()

	if err := service.RecordSourceStatus("arxiv-ai", "failed"); err != nil {
		t.Fatalf("RecordSourceStatus returned error: %v", err)
	}

	sources := service.ListSources()
	var arxivStatus string
	var enabledSignalCount int
	for _, src := range sources {
		if src.ID == "arxiv-ai" {
			arxivStatus = src.LastStatus
		}
		if src.Layer == LayerSignal && src.Enabled && src.LastStatus == "ready" {
			enabledSignalCount++
		}
	}

	if arxivStatus != "failed" {
		t.Fatalf("arxiv status = %q, want failed", arxivStatus)
	}
	if enabledSignalCount == 0 {
		t.Fatalf("no ready enabled signal source remained after fact source failure")
	}
}
