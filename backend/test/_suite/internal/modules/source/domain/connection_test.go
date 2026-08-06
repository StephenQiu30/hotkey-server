package domain

import "testing"

func TestNormalizeSourceConnectionValidatesP0SourceTypesAndEndpoints(t *testing.T) {
	t.Parallel()

	connection, err := NormalizeSourceConnection(SourceConnection{
		SourceType:    SourceTypeRSS,
		Name:          "  Example feed ",
		Endpoint:      "https://feeds.example.test/news",
		AuthType:      AuthTypeNone,
		CredentialRef: "",
	})
	if err != nil {
		t.Fatalf("NormalizeSourceConnection() error = %v", err)
	}
	if connection.Name != "Example feed" {
		t.Errorf("normalized name = %q, want Example feed", connection.Name)
	}
	for _, endpoint := range []string{
		"http://feeds.example.test/news",
		"https://127.0.0.1/news",
		"https://feeds.example.test/news?token=secret",
		"https://user:pass@feeds.example.test/news",
		"https://feeds.example.test:8443/news",
	} {
		if _, err := NormalizeEndpoint(SourceTypeRSS, endpoint); err == nil {
			t.Errorf("NormalizeEndpoint(%q) = nil error, want static SSRF rejection", endpoint)
		}
	}
	if got, err := NormalizeEndpoint(SourceTypeHackerNews, "https://hacker-news.firebaseio.com/v0"); err != nil || got != HackerNewsEndpoint {
		t.Errorf("NormalizeEndpoint(hacker news) = %q, %v", got, err)
	}
}

func TestCredentialReferenceMustMatchAuthType(t *testing.T) {
	t.Parallel()

	if got, err := NormalizeCredentialReference(AuthTypeBearer, "env:HOTKEY_TOKEN"); err != nil || got != "env:HOTKEY_TOKEN" {
		t.Errorf("NormalizeCredentialReference() = %q, %v", got, err)
	}
	for _, test := range []struct {
		auth AuthType
		ref  string
	}{
		{AuthTypeNone, "env:HOTKEY_TOKEN"},
		{AuthTypeAPIKey, "literal-secret"},
		{AuthTypeOAuth2, "env:lowercase"},
	} {
		if _, err := NormalizeCredentialReference(test.auth, test.ref); err == nil {
			t.Errorf("NormalizeCredentialReference(%q, %q) = nil error, want rejection", test.auth, test.ref)
		}
	}
}

func TestNormalizeSourceConfigAppliesDefaultsAndRejectsSecretShapedInput(t *testing.T) {
	t.Parallel()

	config, err := NormalizeSourceConfig(map[string]any{"allowed_languages": []any{"zh-cn", "en"}, "max_pages_per_run": float64(2)})
	if err != nil {
		t.Fatalf("NormalizeSourceConfig() error = %v", err)
	}
	if config.ContentRetentionDays != 30 || config.RateLimitPerMinute != 60 || config.RequestTimeoutSeconds != 30 || config.MaxPagesPerRun != 2 {
		t.Errorf("defaults = %#v, want stable P0 defaults", config)
	}
	if got, want := config.AllowedLanguages[0], "en"; got != want {
		t.Errorf("first allowed language = %q, want %q", got, want)
	}
	for _, input := range []map[string]any{
		{"secret": "literal"},
		{"rate_limit_per_minute": "60"},
		{"allowed_regions": []any{"US", 7}},
		{"nested": map[string]any{"token": "secret"}},
	} {
		if _, err := NormalizeSourceConfig(input); err == nil {
			t.Errorf("NormalizeSourceConfig(%#v) = nil error, want rejection", input)
		}
	}
}
