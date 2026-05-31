package config

import (
	"testing"
	"time"
)

func TestLoadRuntimeMode(t *testing.T) {
	tests := []struct {
		env  string
		want RuntimeMode
	}{
		{"", RuntimeModeAll},
		{"all", RuntimeModeAll},
		{"api", RuntimeModeAPI},
		{"worker", RuntimeModeWorker},
		{"bogus", RuntimeModeAll},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			t.Setenv("HOTKEY_RUNTIME_MODE", tt.env)
			got := Load()
			if got.RuntimeMode != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got.RuntimeMode)
			}
		})
	}
}

func TestLoadEmbeddingAndHotspotDefaults(t *testing.T) {
	t.Setenv("HOTKEY_DASHSCOPE_API_KEY", "")
	t.Setenv("HOTKEY_EMBEDDING_MODEL", "")
	t.Setenv("HOTKEY_HOTSPOT_SIMILARITY_THRESHOLD", "")
	t.Setenv("HOTKEY_HOTSPOT_WINDOW", "")

	got := Load()

	if got.DashScopeAPIKey != "" {
		t.Fatalf("expected empty DashScope API key, got %q", got.DashScopeAPIKey)
	}
	if got.EmbeddingModel != "text-embedding-v2" {
		t.Fatalf("expected default embedding model, got %q", got.EmbeddingModel)
	}
	if got.HotspotSimilarityThreshold != 0.82 {
		t.Fatalf("expected default hotspot threshold, got %f", got.HotspotSimilarityThreshold)
	}
	if got.HotspotWindow != 24*time.Hour {
		t.Fatalf("expected default hotspot window, got %s", got.HotspotWindow)
	}
}

func TestLoadEmbeddingAndHotspotOverrides(t *testing.T) {
	t.Setenv("HOTKEY_DASHSCOPE_API_KEY", "dashscope-key")
	t.Setenv("HOTKEY_EMBEDDING_MODEL", "custom-model")
	t.Setenv("HOTKEY_HOTSPOT_SIMILARITY_THRESHOLD", "0.91")
	t.Setenv("HOTKEY_HOTSPOT_WINDOW", "12h")

	got := Load()

	if got.DashScopeAPIKey != "dashscope-key" || got.EmbeddingModel != "custom-model" {
		t.Fatalf("unexpected embedding config: %+v", got)
	}
	if got.HotspotSimilarityThreshold != 0.91 {
		t.Fatalf("unexpected threshold: %f", got.HotspotSimilarityThreshold)
	}
	if got.HotspotWindow != 12*time.Hour {
		t.Fatalf("unexpected window: %s", got.HotspotWindow)
	}
}
