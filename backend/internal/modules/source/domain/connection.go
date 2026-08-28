// Package domain contains SourceConnection control-plane facts and static
// validation only. It never resolves DNS or performs network I/O.
package domain

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

type SourceType string
type AuthType string
type HealthStatus string
type HackerNewsMode string

const (
	SourceTypeRSS               SourceType = "rss"
	SourceTypeHackerNews        SourceType = "hacker_news"
	SourceTypeX                 SourceType = "x"
	SourceTypeBingGrounding     SourceType = "bing_grounding"
	SourceTypeBilibili          SourceType = "bilibili"
	SourceTypeWeibo             SourceType = "weibo"
	SourceTypeGoogleAgentSearch SourceType = "google_agent_search"

	AuthTypeNone   AuthType = "none"
	AuthTypeAPIKey AuthType = "api_key"
	AuthTypeOAuth2 AuthType = "oauth2"
	AuthTypeBearer AuthType = "bearer"

	HealthStatusUnknown     HealthStatus = "unknown"
	HealthStatusHealthy     HealthStatus = "healthy"
	HealthStatusDegraded    HealthStatus = "degraded"
	HealthStatusUnavailable HealthStatus = "unavailable"

	HackerNewsModeNew  HackerNewsMode = "new"
	HackerNewsModeTop  HackerNewsMode = "top"
	HackerNewsModeBest HackerNewsMode = "best"

	HackerNewsEndpoint              = "https://hacker-news.firebaseio.com/v0"
	XRecentSearchEndpoint           = "https://api.x.com/2/tweets/search/recent"
	BilibiliOpenEndpoint            = "https://member.bilibili.com/arcopen/fn"
	WeiboCLIApiEndpoint             = "https://open.weibo.com/cli/api"
	WeiboDeveloperTerms             = "https://open.weibo.com/wiki/%E5%BC%80%E5%8F%91%E8%80%85%E5%8D%8F%E8%AE%AE"
	GoogleAgentSearchGlobalEndpoint = "https://discoveryengine.googleapis.com"
	GoogleAgentSearchUSEndpoint     = "https://us-discoveryengine.googleapis.com"
	GoogleAgentSearchEUEndpoint     = "https://eu-discoveryengine.googleapis.com"
	GoogleCloudTerms                = "https://cloud.google.com/terms"
	GoogleLegacySearchDeprecationAt = "2027-01-01"
	ManagedCredentialReference      = "managed:v1"
)

var credentialReferencePattern = regexp.MustCompile(`^env:[A-Z_][A-Z0-9_]{0,127}$`)
var foundryToolboxPathPattern = regexp.MustCompile(`^/api/projects/[A-Za-z0-9][A-Za-z0-9._-]{0,127}/toolboxes/[A-Za-z0-9][A-Za-z0-9._-]{0,127}/versions/[A-Za-z0-9][A-Za-z0-9._-]{0,127}/mcp$`)
var bilibiliOpenIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var googleServingConfigPattern = regexp.MustCompile(`^projects/[a-z][a-z0-9-]{4,28}[a-z0-9]/locations/(global|us|eu)/collections/default_collection/dataStores/[A-Za-z0-9_-]{1,63}/servingConfigs/[A-Za-z0-9_-]{1,63}$`)

type SourceConnection struct {
	ID             int64
	Version        int64
	SourceType     SourceType
	Name           string
	Endpoint       string
	AuthType       AuthType
	CredentialRef  string
	Config         SourceConfig
	Enabled        bool
	HealthStatus   HealthStatus
	TermsPolicyURL string
	Deleted        bool
}

// SourceConfig is the complete, defaulted P0 configuration. A Source
// Connection never carries arbitrary JSON in its domain model.
type SourceConfig struct {
	// AllowBodyStorage is a legacy compatibility flag. It is not a rights fact
	// and cannot authorize new CapturedItem, EvidenceSnapshot, or DocumentVersion
	// body persistence; only an exact current RightsDecision can do that.
	AllowBodyStorage                 bool
	RequiresAttribution              bool
	RequiresDeletionSync             bool
	ContentRetentionDays             int
	MetricsRetentionDays             int
	AllowedLanguages                 []string
	AllowedRegions                   []string
	RateLimitPerMinute               int
	RequestTimeoutSeconds            int
	MaxPagesPerRun                   int
	GroundingDataBoundaryApproved    bool
	BilibiliOpenID                   string
	GoogleLocation                   string
	GoogleServingConfig              string
	HackerNewsMode                   HackerNewsMode
	XMetricRefreshEnabled            bool
	XMetricRefreshIntervalMinutes    int
	XMetricRefreshObservationHours   int
	XMetricRefreshMaxPostsPerRun     int
	XMetricRefreshDailyRequestBudget int
}

func DefaultSourceConfig() SourceConfig {
	return SourceConfig{
		AllowBodyStorage: true, RequiresAttribution: false, RequiresDeletionSync: false,
		ContentRetentionDays: 30, MetricsRetentionDays: 30,
		AllowedLanguages: []string{}, AllowedRegions: []string{},
		RateLimitPerMinute: 60, RequestTimeoutSeconds: 30, MaxPagesPerRun: 1,
		HackerNewsMode:        HackerNewsModeNew,
		XMetricRefreshEnabled: false, XMetricRefreshIntervalMinutes: 60,
		XMetricRefreshObservationHours: 48, XMetricRefreshMaxPostsPerRun: 100,
		XMetricRefreshDailyRequestBudget: 24,
	}
}

func NormalizeSourceConnection(connection SourceConnection) (SourceConnection, error) {
	if !connection.SourceType.Valid() {
		return SourceConnection{}, fmt.Errorf("unsupported source type %q", connection.SourceType)
	}
	name := strings.TrimSpace(connection.Name)
	if count := utf8.RuneCountInString(name); count < 1 || count > 120 {
		return SourceConnection{}, fmt.Errorf("source name must be 1-120 Unicode code points")
	}
	endpoint, err := NormalizeEndpoint(connection.SourceType, connection.Endpoint)
	if err != nil {
		return SourceConnection{}, err
	}
	credentialRef, err := NormalizeCredentialReference(connection.AuthType, connection.CredentialRef)
	if err != nil {
		return SourceConnection{}, err
	}
	if connection.SourceType == SourceTypeX && (connection.AuthType != AuthTypeBearer || credentialRef == "") {
		return SourceConnection{}, fmt.Errorf("X source requires a Bearer credential reference")
	}
	if connection.SourceType == SourceTypeBingGrounding && (connection.AuthType != AuthTypeBearer || credentialRef == "") {
		return SourceConnection{}, fmt.Errorf("Bing Grounding source requires a Bearer credential reference")
	}
	if connection.SourceType == SourceTypeBilibili && (connection.AuthType != AuthTypeOAuth2 || credentialRef == "") {
		return SourceConnection{}, fmt.Errorf("Bilibili source requires an OAuth2 credential reference")
	}
	if connection.SourceType == SourceTypeWeibo && (connection.AuthType != AuthTypeBearer || credentialRef == "") {
		return SourceConnection{}, fmt.Errorf("Weibo source requires a Bearer credential reference")
	}
	if connection.SourceType == SourceTypeGoogleAgentSearch && (connection.AuthType != AuthTypeBearer || credentialRef == "") {
		return SourceConnection{}, fmt.Errorf("Google Agent Search source requires a Bearer credential reference")
	}
	config := connection.Config
	if config.isZero() {
		config = DefaultSourceConfig()
	}
	config, err = config.Normalize()
	if err != nil {
		return SourceConnection{}, err
	}
	if connection.SourceType != SourceTypeX && config.XMetricRefreshEnabled {
		return SourceConnection{}, fmt.Errorf("X metric refresh is available only for X sources")
	}
	if connection.SourceType == SourceTypeBingGrounding && (!config.AllowBodyStorage || !config.RequiresAttribution || config.MaxPagesPerRun != 1) {
		return SourceConnection{}, fmt.Errorf("Bing Grounding source requires body storage, attribution, and one page per run")
	}
	if connection.SourceType == SourceTypeBilibili && (!config.AllowBodyStorage || !config.RequiresAttribution || !config.RequiresDeletionSync || config.BilibiliOpenID == "") {
		return SourceConnection{}, fmt.Errorf("Bilibili source requires an authorized OpenID, body storage, attribution, and deletion sync")
	}
	if connection.SourceType == SourceTypeBilibili && strings.TrimSpace(connection.TermsPolicyURL) != "https://openhome.bilibili.com/agreement/privacy-policy" {
		return SourceConnection{}, fmt.Errorf("Bilibili source requires the official privacy policy URL")
	}
	if connection.SourceType == SourceTypeWeibo && (!config.AllowBodyStorage || !config.RequiresAttribution || !config.RequiresDeletionSync) {
		return SourceConnection{}, fmt.Errorf("Weibo source requires body storage, attribution, and deletion sync")
	}
	if connection.SourceType == SourceTypeWeibo && strings.TrimSpace(connection.TermsPolicyURL) != WeiboDeveloperTerms {
		return SourceConnection{}, fmt.Errorf("Weibo source requires the official developer agreement URL")
	}
	if connection.SourceType == SourceTypeGoogleAgentSearch {
		expectedEndpoint, ok := googleAgentSearchEndpoint(config.GoogleLocation)
		if !ok || connection.Endpoint != expectedEndpoint || !googleServingConfigPattern.MatchString(config.GoogleServingConfig) || !strings.Contains(config.GoogleServingConfig, "/locations/"+config.GoogleLocation+"/") {
			return SourceConnection{}, fmt.Errorf("Google Agent Search source requires a matching official endpoint, location, and ServingConfig")
		}
		if !config.AllowBodyStorage || !config.RequiresAttribution || strings.TrimSpace(connection.TermsPolicyURL) != GoogleCloudTerms {
			return SourceConnection{}, fmt.Errorf("Google Agent Search source requires snippet storage, attribution, and the official Google Cloud terms URL")
		}
	}
	if connection.SourceType == SourceTypeBingGrounding {
		termsPolicyURL, err := url.Parse(strings.TrimSpace(connection.TermsPolicyURL))
		if err != nil || termsPolicyURL.Scheme != "https" || termsPolicyURL.Hostname() == "" || termsPolicyURL.User != nil || termsPolicyURL.Fragment != "" {
			return SourceConnection{}, fmt.Errorf("Bing Grounding source requires an HTTPS terms and policy URL")
		}
		connection.TermsPolicyURL = termsPolicyURL.String()
	}
	if connection.HealthStatus == "" {
		connection.HealthStatus = HealthStatusUnknown
	}
	if !connection.HealthStatus.Valid() {
		return SourceConnection{}, fmt.Errorf("health status is invalid")
	}
	connection.Name = name
	connection.Endpoint = endpoint
	connection.CredentialRef = credentialRef
	connection.Config = config
	return connection, nil
}

func (sourceType SourceType) Valid() bool {
	return sourceType == SourceTypeRSS || sourceType == SourceTypeHackerNews || sourceType == SourceTypeX || sourceType == SourceTypeBingGrounding || sourceType == SourceTypeBilibili || sourceType == SourceTypeWeibo || sourceType == SourceTypeGoogleAgentSearch
}

func (authType AuthType) Valid() bool {
	return authType == AuthTypeNone || authType == AuthTypeAPIKey || authType == AuthTypeOAuth2 || authType == AuthTypeBearer
}

func (status HealthStatus) Valid() bool {
	return status == HealthStatusUnknown || status == HealthStatusHealthy || status == HealthStatusDegraded || status == HealthStatusUnavailable
}

func (mode HackerNewsMode) Valid() bool {
	return mode == HackerNewsModeNew || mode == HackerNewsModeTop || mode == HackerNewsModeBest
}

func NormalizeCredentialReference(authType AuthType, value string) (string, error) {
	if !authType.Valid() {
		return "", fmt.Errorf("auth type is invalid")
	}
	normalized := strings.TrimSpace(value)
	if authType == AuthTypeNone {
		if normalized != "" {
			return "", fmt.Errorf("credential reference must be empty for auth_type none")
		}
		return "", nil
	}
	if normalized != ManagedCredentialReference && !credentialReferencePattern.MatchString(normalized) {
		return "", fmt.Errorf("credential reference must use env:NAME or the managed credential store")
	}
	return normalized, nil
}

// NormalizeEndpoint enforces static SSRF protections that can be checked
// without network access. Connectors must additionally re-check DNS answers
// and every redirect at connection time.
func NormalizeEndpoint(sourceType SourceType, value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if sourceType == SourceTypeHackerNews {
		if normalized != HackerNewsEndpoint {
			return "", fmt.Errorf("hacker news endpoint must be the official endpoint")
		}
		return HackerNewsEndpoint, nil
	}
	if sourceType == SourceTypeX {
		if normalized != XRecentSearchEndpoint {
			return "", fmt.Errorf("X endpoint must be the official recent search endpoint")
		}
		return XRecentSearchEndpoint, nil
	}
	if sourceType == SourceTypeBingGrounding {
		parsed, err := url.Parse(normalized)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
			return "", fmt.Errorf("Bing Grounding endpoint must be an HTTPS Foundry Toolbox URI")
		}
		if port := parsed.Port(); port != "" && port != "443" {
			return "", fmt.Errorf("Bing Grounding endpoint must use port 443")
		}
		host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
		account := strings.TrimSuffix(host, ".services.ai.azure.com")
		if account == host || account == "" || strings.Contains(account, ".") || !validDNSName(host) || !foundryToolboxPathPattern.MatchString(parsed.Path) {
			return "", fmt.Errorf("Bing Grounding endpoint must be a versioned Foundry Toolbox MCP URI")
		}
		query := parsed.Query()
		if len(query) != 1 || len(query["api-version"]) != 1 || query.Get("api-version") != "v1" {
			return "", fmt.Errorf("Bing Grounding endpoint must use api-version=v1")
		}
		parsed.Scheme, parsed.Host = "https", host
		return parsed.String(), nil
	}
	if sourceType == SourceTypeBilibili {
		if normalized != BilibiliOpenEndpoint {
			return "", fmt.Errorf("Bilibili endpoint must be the official Open Platform endpoint")
		}
		return BilibiliOpenEndpoint, nil
	}
	if sourceType == SourceTypeWeibo {
		if normalized != WeiboCLIApiEndpoint {
			return "", fmt.Errorf("Weibo endpoint must be the official CLI API endpoint")
		}
		return WeiboCLIApiEndpoint, nil
	}
	if sourceType == SourceTypeGoogleAgentSearch {
		for _, endpoint := range []string{GoogleAgentSearchGlobalEndpoint, GoogleAgentSearchUSEndpoint, GoogleAgentSearchEUEndpoint} {
			if normalized == endpoint {
				return endpoint, nil
			}
		}
		return "", fmt.Errorf("Google Agent Search endpoint must be an official regional endpoint")
	}
	if sourceType != SourceTypeRSS {
		return "", fmt.Errorf("unsupported source type %q", sourceType)
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("RSS endpoint must be an HTTPS URI")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("RSS endpoint cannot contain userinfo or fragment")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return "", fmt.Errorf("RSS endpoint must use port 443")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || net.ParseIP(host) != nil || looksLikeObfuscatedIPAddress(host) || !validDNSName(host) {
		return "", fmt.Errorf("RSS endpoint host must be a DNS name")
	}
	for key := range parsed.Query() {
		switch strings.ToLower(key) {
		case "token", "key", "secret", "password", "authorization":
			return "", fmt.Errorf("RSS endpoint query contains a credential-shaped key")
		}
	}
	parsed.Scheme = "https"
	if parsed.Port() == "443" {
		parsed.Host = host
	} else {
		parsed.Host = host
	}
	return parsed.String(), nil
}

// looksLikeObfuscatedIPAddress rejects the legacy integer, octal and
// hexadecimal IPv4 spellings that some URL stacks normalize to an address
// even though net.ParseIP deliberately accepts only canonical forms. A real
// DNS name must contain at least one non-numeric label.
func looksLikeObfuscatedIPAddress(host string) bool {
	labels := strings.Split(strings.ToLower(host), ".")
	if len(labels) == 0 || len(labels) > 4 {
		return false
	}
	for _, label := range labels {
		if label == "" {
			return false
		}
		digits := label
		base := byte(10)
		if strings.HasPrefix(label, "0x") {
			digits = strings.TrimPrefix(label, "0x")
			base = 16
		}
		if digits == "" {
			return false
		}
		for index := range len(digits) {
			character := digits[index]
			if character >= '0' && character <= '9' {
				continue
			}
			if base == 16 && character >= 'a' && character <= 'f' {
				continue
			}
			return false
		}
	}
	return true
}

func NormalizeSourceConfig(input map[string]any) (SourceConfig, error) {
	config := DefaultSourceConfig()
	for key, value := range input {
		switch key {
		case "allow_body_storage":
			boolean, ok := value.(bool)
			if !ok {
				return SourceConfig{}, fmt.Errorf("%s must be boolean", key)
			}
			config.AllowBodyStorage = boolean
		case "requires_attribution":
			boolean, ok := value.(bool)
			if !ok {
				return SourceConfig{}, fmt.Errorf("%s must be boolean", key)
			}
			config.RequiresAttribution = boolean
		case "requires_deletion_sync":
			boolean, ok := value.(bool)
			if !ok {
				return SourceConfig{}, fmt.Errorf("%s must be boolean", key)
			}
			config.RequiresDeletionSync = boolean
		case "content_retention_days":
			integer, err := configInteger(value)
			if err != nil {
				return SourceConfig{}, fmt.Errorf("%s: %w", key, err)
			}
			config.ContentRetentionDays = integer
		case "metrics_retention_days":
			integer, err := configInteger(value)
			if err != nil {
				return SourceConfig{}, fmt.Errorf("%s: %w", key, err)
			}
			config.MetricsRetentionDays = integer
		case "allowed_languages":
			items, err := configStrings(value)
			if err != nil {
				return SourceConfig{}, fmt.Errorf("%s: %w", key, err)
			}
			config.AllowedLanguages = items
		case "allowed_regions":
			items, err := configStrings(value)
			if err != nil {
				return SourceConfig{}, fmt.Errorf("%s: %w", key, err)
			}
			config.AllowedRegions = items
		case "rate_limit_per_minute":
			integer, err := configInteger(value)
			if err != nil {
				return SourceConfig{}, fmt.Errorf("%s: %w", key, err)
			}
			config.RateLimitPerMinute = integer
		case "request_timeout_seconds":
			integer, err := configInteger(value)
			if err != nil {
				return SourceConfig{}, fmt.Errorf("%s: %w", key, err)
			}
			config.RequestTimeoutSeconds = integer
		case "max_pages_per_run":
			integer, err := configInteger(value)
			if err != nil {
				return SourceConfig{}, fmt.Errorf("%s: %w", key, err)
			}
			config.MaxPagesPerRun = integer
		case "grounding_data_boundary_approved":
			boolean, ok := value.(bool)
			if !ok {
				return SourceConfig{}, fmt.Errorf("%s must be boolean", key)
			}
			config.GroundingDataBoundaryApproved = boolean
		case "bilibili_open_id":
			text, ok := value.(string)
			if !ok {
				return SourceConfig{}, fmt.Errorf("%s must be string", key)
			}
			config.BilibiliOpenID = text
		case "google_location":
			text, ok := value.(string)
			if !ok {
				return SourceConfig{}, fmt.Errorf("%s must be string", key)
			}
			config.GoogleLocation = text
		case "google_serving_config":
			text, ok := value.(string)
			if !ok {
				return SourceConfig{}, fmt.Errorf("%s must be string", key)
			}
			config.GoogleServingConfig = text
		case "hacker_news_mode":
			text, ok := value.(string)
			if !ok {
				return SourceConfig{}, fmt.Errorf("%s must be string", key)
			}
			config.HackerNewsMode = HackerNewsMode(text)
		case "x_metric_refresh_enabled":
			boolean, ok := value.(bool)
			if !ok {
				return SourceConfig{}, fmt.Errorf("%s must be boolean", key)
			}
			config.XMetricRefreshEnabled = boolean
		case "x_metric_refresh_interval_minutes":
			integer, err := configInteger(value)
			if err != nil {
				return SourceConfig{}, fmt.Errorf("%s: %w", key, err)
			}
			config.XMetricRefreshIntervalMinutes = integer
		case "x_metric_refresh_observation_hours":
			integer, err := configInteger(value)
			if err != nil {
				return SourceConfig{}, fmt.Errorf("%s: %w", key, err)
			}
			config.XMetricRefreshObservationHours = integer
		case "x_metric_refresh_max_posts_per_run":
			integer, err := configInteger(value)
			if err != nil {
				return SourceConfig{}, fmt.Errorf("%s: %w", key, err)
			}
			config.XMetricRefreshMaxPostsPerRun = integer
		case "x_metric_refresh_daily_request_budget":
			integer, err := configInteger(value)
			if err != nil {
				return SourceConfig{}, fmt.Errorf("%s: %w", key, err)
			}
			config.XMetricRefreshDailyRequestBudget = integer
		default:
			return SourceConfig{}, fmt.Errorf("source config key %q is not allowed", key)
		}
	}
	return config.Normalize()
}

func (config SourceConfig) Normalize() (SourceConfig, error) {
	var err error
	defaults := DefaultSourceConfig()
	if config.XMetricRefreshIntervalMinutes == 0 {
		config.XMetricRefreshIntervalMinutes = defaults.XMetricRefreshIntervalMinutes
	}
	if config.XMetricRefreshObservationHours == 0 {
		config.XMetricRefreshObservationHours = defaults.XMetricRefreshObservationHours
	}
	if config.XMetricRefreshMaxPostsPerRun == 0 {
		config.XMetricRefreshMaxPostsPerRun = defaults.XMetricRefreshMaxPostsPerRun
	}
	if config.XMetricRefreshDailyRequestBudget == 0 {
		config.XMetricRefreshDailyRequestBudget = defaults.XMetricRefreshDailyRequestBudget
	}
	if config.AllowedLanguages, err = normalizeLanguages(config.AllowedLanguages, 0, 8); err != nil {
		return SourceConfig{}, err
	}
	if config.AllowedRegions, err = normalizeRegions(config.AllowedRegions, 0, 8); err != nil {
		return SourceConfig{}, err
	}
	if config.ContentRetentionDays < 1 || config.ContentRetentionDays > 3650 || config.MetricsRetentionDays < 1 || config.MetricsRetentionDays > 3650 {
		return SourceConfig{}, fmt.Errorf("retention days must be from 1 to 3650")
	}
	if config.RateLimitPerMinute < 1 || config.RateLimitPerMinute > 600 {
		return SourceConfig{}, fmt.Errorf("rate limit must be from 1 to 600")
	}
	if config.RequestTimeoutSeconds < 1 || config.RequestTimeoutSeconds > 120 {
		return SourceConfig{}, fmt.Errorf("request timeout must be from 1 to 120")
	}
	if config.MaxPagesPerRun < 1 || config.MaxPagesPerRun > 20 {
		return SourceConfig{}, fmt.Errorf("max pages per run must be from 1 to 20")
	}
	if config.XMetricRefreshIntervalMinutes < 15 || config.XMetricRefreshIntervalMinutes > 1440 {
		return SourceConfig{}, fmt.Errorf("X metric refresh interval must be from 15 to 1440 minutes")
	}
	if config.XMetricRefreshObservationHours < 1 || config.XMetricRefreshObservationHours > 168 {
		return SourceConfig{}, fmt.Errorf("X metric refresh observation period must be from 1 to 168 hours")
	}
	if config.XMetricRefreshMaxPostsPerRun < 1 || config.XMetricRefreshMaxPostsPerRun > 100 {
		return SourceConfig{}, fmt.Errorf("X metric refresh batch must contain from 1 to 100 posts")
	}
	if config.XMetricRefreshDailyRequestBudget < 1 || config.XMetricRefreshDailyRequestBudget > 1440 {
		return SourceConfig{}, fmt.Errorf("X metric refresh daily request budget must be from 1 to 1440")
	}
	config.BilibiliOpenID = strings.TrimSpace(config.BilibiliOpenID)
	config.GoogleLocation = strings.ToLower(strings.TrimSpace(config.GoogleLocation))
	config.GoogleServingConfig = strings.TrimSpace(config.GoogleServingConfig)
	config.HackerNewsMode = HackerNewsMode(strings.ToLower(strings.TrimSpace(string(config.HackerNewsMode))))
	if config.BilibiliOpenID != "" && !bilibiliOpenIDPattern.MatchString(config.BilibiliOpenID) {
		return SourceConfig{}, fmt.Errorf("Bilibili OpenID is invalid")
	}
	if !config.HackerNewsMode.Valid() {
		return SourceConfig{}, fmt.Errorf("Hacker News mode must be new, top, or best")
	}
	return config, nil
}

func (config SourceConfig) isZero() bool {
	return !config.AllowBodyStorage && !config.RequiresAttribution && !config.RequiresDeletionSync && !config.GroundingDataBoundaryApproved && !config.XMetricRefreshEnabled && config.BilibiliOpenID == "" && config.GoogleLocation == "" && config.GoogleServingConfig == "" && config.HackerNewsMode == "" && config.ContentRetentionDays == 0 && config.MetricsRetentionDays == 0 && len(config.AllowedLanguages) == 0 && len(config.AllowedRegions) == 0 && config.RateLimitPerMinute == 0 && config.RequestTimeoutSeconds == 0 && config.MaxPagesPerRun == 0 && config.XMetricRefreshIntervalMinutes == 0 && config.XMetricRefreshObservationHours == 0 && config.XMetricRefreshMaxPostsPerRun == 0 && config.XMetricRefreshDailyRequestBudget == 0
}

func (config SourceConfig) Map() map[string]any {
	return map[string]any{
		"allow_body_storage": config.AllowBodyStorage, "requires_attribution": config.RequiresAttribution, "requires_deletion_sync": config.RequiresDeletionSync,
		"content_retention_days": config.ContentRetentionDays, "metrics_retention_days": config.MetricsRetentionDays,
		"allowed_languages": append([]string(nil), config.AllowedLanguages...), "allowed_regions": append([]string(nil), config.AllowedRegions...),
		"rate_limit_per_minute": config.RateLimitPerMinute, "request_timeout_seconds": config.RequestTimeoutSeconds, "max_pages_per_run": config.MaxPagesPerRun,
		"grounding_data_boundary_approved":      config.GroundingDataBoundaryApproved,
		"bilibili_open_id":                      config.BilibiliOpenID,
		"google_location":                       config.GoogleLocation,
		"google_serving_config":                 config.GoogleServingConfig,
		"hacker_news_mode":                      string(config.HackerNewsMode),
		"x_metric_refresh_enabled":              config.XMetricRefreshEnabled,
		"x_metric_refresh_interval_minutes":     config.XMetricRefreshIntervalMinutes,
		"x_metric_refresh_observation_hours":    config.XMetricRefreshObservationHours,
		"x_metric_refresh_max_posts_per_run":    config.XMetricRefreshMaxPostsPerRun,
		"x_metric_refresh_daily_request_budget": config.XMetricRefreshDailyRequestBudget,
	}
}

func googleAgentSearchEndpoint(location string) (string, bool) {
	switch location {
	case "global":
		return GoogleAgentSearchGlobalEndpoint, true
	case "us":
		return GoogleAgentSearchUSEndpoint, true
	case "eu":
		return GoogleAgentSearchEUEndpoint, true
	default:
		return "", false
	}
}

func configInteger(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		if typed > int64(^uint(0)>>1) || typed < -int64(^uint(0)>>1)-1 {
			return 0, fmt.Errorf("integer out of range")
		}
		return int(typed), nil
	case float64:
		if typed != float64(int(typed)) {
			return 0, fmt.Errorf("must be an integer")
		}
		return int(typed), nil
	case json.Number:
		integer, err := typed.Int64()
		if err != nil {
			return 0, fmt.Errorf("must be an integer")
		}
		return int(integer), nil
	default:
		return 0, fmt.Errorf("must be an integer")
	}
}

func configStrings(value any) ([]string, error) {
	values, ok := value.([]any)
	if !ok {
		if stringsValue, ok := value.([]string); ok {
			return append([]string(nil), stringsValue...), nil
		}
		return nil, fmt.Errorf("must be an array of strings")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		stringValue, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("must be an array of strings")
		}
		result = append(result, stringValue)
	}
	return result, nil
}

func validDNSName(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
				return false
			}
		}
	}
	return true
}

func normalizeLanguages(values []string, min, max int) ([]string, error) {
	if len(values) < min || len(values) > max {
		return nil, fmt.Errorf("language count must be from %d to %d", min, max)
	}
	set := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := norm.NFC.String(strings.TrimSpace(raw))
		tag, err := language.Parse(value)
		if value == "" || err != nil || tag == language.Und {
			return nil, fmt.Errorf("language tag %q is invalid", raw)
		}
		set[tag.String()] = struct{}{}
	}
	if len(set) < min || len(set) > max {
		return nil, fmt.Errorf("normalized language count must be from %d to %d", min, max)
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeRegions(values []string, min, max int) ([]string, error) {
	if len(values) < min || len(values) > max {
		return nil, fmt.Errorf("region count must be from %d to %d", min, max)
	}
	set := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.ToUpper(norm.NFC.String(strings.TrimSpace(raw)))
		if len(value) != 2 || value[0] < 'A' || value[0] > 'Z' || value[1] < 'A' || value[1] > 'Z' {
			return nil, fmt.Errorf("region %q is not an ISO alpha-2 code", raw)
		}
		set[value] = struct{}{}
	}
	if len(set) < min || len(set) > max {
		return nil, fmt.Errorf("normalized region count must be from %d to %d", min, max)
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

// SortedAllowedLanguages/Regions are useful to callers constructing stable
// preview inputs without exposing config JSON.
func (config SourceConfig) SortedAllowedLanguages() []string {
	result := append([]string(nil), config.AllowedLanguages...)
	sort.Strings(result)
	return result
}
func (config SourceConfig) SortedAllowedRegions() []string {
	result := append([]string(nil), config.AllowedRegions...)
	sort.Strings(result)
	return result
}
