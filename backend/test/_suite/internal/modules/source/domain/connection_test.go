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
	xConnection, err := NormalizeSourceConnection(SourceConnection{
		SourceType: SourceTypeX, Name: "X Recent Search", Endpoint: XRecentSearchEndpoint,
		AuthType: AuthTypeBearer, CredentialRef: "env:X_BEARER_TOKEN",
	})
	if err != nil || xConnection.Endpoint != XRecentSearchEndpoint {
		t.Fatalf("NormalizeSourceConnection(X) = %#v, %v", xConnection, err)
	}
	for _, invalid := range []SourceConnection{
		{SourceType: SourceTypeX, Name: "X", Endpoint: "https://example.test/2/tweets/search/recent", AuthType: AuthTypeBearer, CredentialRef: "env:X_BEARER_TOKEN"},
		{SourceType: SourceTypeX, Name: "X", Endpoint: XRecentSearchEndpoint, AuthType: AuthTypeNone},
		{SourceType: SourceTypeX, Name: "X", Endpoint: XRecentSearchEndpoint, AuthType: AuthTypeBearer},
	} {
		if _, err := NormalizeSourceConnection(invalid); err == nil {
			t.Fatalf("NormalizeSourceConnection(%#v) accepted invalid X connection", invalid)
		}
	}
}

func TestBingGroundingConnectionRequiresVersionedFoundryToolboxAndDerivedEvidencePolicy(t *testing.T) {
	t.Parallel()
	config := DefaultSourceConfig()
	config.RequiresAttribution = true
	config.GroundingDataBoundaryApproved = true
	connection := SourceConnection{
		SourceType: SourceTypeBingGrounding, Name: "Foundry Web Search",
		Endpoint: "https://hotkey.services.ai.azure.com/api/projects/hotkey/toolboxes/web-search/versions/1/mcp?api-version=v1",
		AuthType: AuthTypeBearer, CredentialRef: "env:AZURE_FOUNDRY_TOKEN", Config: config,
		TermsPolicyURL: "https://learn.microsoft.com/azure/foundry/web-search",
	}
	normalized, err := NormalizeSourceConnection(connection)
	if err != nil || normalized.Endpoint != connection.Endpoint || !normalized.Config.GroundingDataBoundaryApproved {
		t.Fatalf("NormalizeSourceConnection() = %#v, %v", normalized, err)
	}
	for _, endpoint := range []string{
		"https://api.bing.microsoft.com/v7.0/search",
		"https://hotkey.services.ai.azure.com/api/projects/hotkey/toolboxes/web-search/mcp?api-version=v1",
		"https://hotkey.services.ai.azure.com/api/projects/hotkey/toolboxes/web-search/versions/1/mcp?api-version=2025-05-01",
		"https://127.0.0.1/api/projects/hotkey/toolboxes/web-search/versions/1/mcp?api-version=v1",
	} {
		connection.Endpoint = endpoint
		if _, err := NormalizeSourceConnection(connection); err == nil {
			t.Fatalf("accepted unsupported endpoint %q", endpoint)
		}
	}
	connection.Endpoint = "https://hotkey.services.ai.azure.com/api/projects/hotkey/toolboxes/web-search/versions/1/mcp?api-version=v1"
	connection.Config.RequiresAttribution = false
	if _, err := NormalizeSourceConnection(connection); err == nil {
		t.Fatal("accepted Grounding without attribution")
	}
}

func TestWeiboConnectionRequiresOfficialCLIAPIAndCompliancePolicy(t *testing.T) {
	t.Parallel()
	config := DefaultSourceConfig()
	config.RequiresAttribution = true
	config.RequiresDeletionSync = true
	connection := SourceConnection{
		SourceType: SourceTypeWeibo, Name: "微博关键词", Endpoint: WeiboCLIApiEndpoint,
		AuthType: AuthTypeBearer, CredentialRef: "env:WEIBO_TOKEN", Config: config,
		TermsPolicyURL: WeiboDeveloperTerms,
	}
	if normalized, err := NormalizeSourceConnection(connection); err != nil || normalized.Endpoint != WeiboCLIApiEndpoint {
		t.Fatalf("NormalizeSourceConnection() = %#v, %v", normalized, err)
	}
	for _, mutate := range []func(*SourceConnection){
		func(value *SourceConnection) { value.Endpoint = "https://weibo.com/ajax/statuses/searchProfile" },
		func(value *SourceConnection) { value.AuthType = AuthTypeNone; value.CredentialRef = "" },
		func(value *SourceConnection) { value.TermsPolicyURL = "https://example.test/terms" },
		func(value *SourceConnection) { value.Config.RequiresDeletionSync = false },
	} {
		invalid := connection
		mutate(&invalid)
		if _, err := NormalizeSourceConnection(invalid); err == nil {
			t.Fatalf("accepted invalid Weibo connection %#v", invalid)
		}
	}
}

func TestGoogleAgentSearchConnectionRequiresMatchingOfficialRegionAndServingConfig(t *testing.T) {
	t.Parallel()
	config := DefaultSourceConfig()
	config.RequiresAttribution = true
	config.GoogleLocation = "global"
	config.GoogleServingConfig = "projects/hotkey-demo/locations/global/collections/default_collection/dataStores/news/servingConfigs/default_config"
	connection := SourceConnection{
		SourceType: SourceTypeGoogleAgentSearch, Name: "Google 限定域搜索", Endpoint: GoogleAgentSearchGlobalEndpoint,
		AuthType: AuthTypeBearer, CredentialRef: "env:GOOGLE_AGENT_SEARCH_TOKEN", Config: config,
		TermsPolicyURL: GoogleCloudTerms,
	}
	if normalized, err := NormalizeSourceConnection(connection); err != nil || normalized.Config.GoogleLocation != "global" {
		t.Fatalf("NormalizeSourceConnection() = %#v, %v", normalized, err)
	}
	for _, mutate := range []func(*SourceConnection){
		func(value *SourceConnection) { value.Endpoint = "https://www.googleapis.com/customsearch/v1" },
		func(value *SourceConnection) { value.Endpoint = GoogleAgentSearchUSEndpoint },
		func(value *SourceConnection) {
			value.Config.GoogleServingConfig = "projects/hotkey-demo/locations/us/collections/default_collection/dataStores/news/servingConfigs/default_config"
		},
		func(value *SourceConnection) {
			value.Config.GoogleServingConfig = "projects/hotkey-demo/locations/global/collections/default_collection/engines/news/servingConfigs/default_config"
		},
		func(value *SourceConnection) { value.AuthType = AuthTypeAPIKey },
		func(value *SourceConnection) { value.Config.RequiresAttribution = false },
		func(value *SourceConnection) { value.TermsPolicyURL = "https://example.test/terms" },
	} {
		invalid := connection
		mutate(&invalid)
		if _, err := NormalizeSourceConnection(invalid); err == nil {
			t.Fatalf("accepted invalid Google Agent Search connection %#v", invalid)
		}
	}
}

func TestCredentialReferenceMustMatchAuthType(t *testing.T) {
	t.Parallel()

	if got, err := NormalizeCredentialReference(AuthTypeBearer, "env:HOTKEY_TOKEN"); err != nil || got != "env:HOTKEY_TOKEN" {
		t.Errorf("NormalizeCredentialReference() = %q, %v", got, err)
	}
	if got, err := NormalizeCredentialReference(AuthTypeBearer, ManagedCredentialReference); err != nil || got != ManagedCredentialReference {
		t.Errorf("NormalizeCredentialReference(managed) = %q, %v", got, err)
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
