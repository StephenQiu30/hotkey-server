package config

import "testing"

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
