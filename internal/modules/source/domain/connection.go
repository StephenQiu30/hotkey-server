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

const (
	SourceTypeRSS        SourceType = "rss"
	SourceTypeHackerNews SourceType = "hacker_news"

	AuthTypeNone   AuthType = "none"
	AuthTypeAPIKey AuthType = "api_key"
	AuthTypeOAuth2 AuthType = "oauth2"
	AuthTypeBearer AuthType = "bearer"

	HealthStatusUnknown     HealthStatus = "unknown"
	HealthStatusHealthy     HealthStatus = "healthy"
	HealthStatusDegraded    HealthStatus = "degraded"
	HealthStatusUnavailable HealthStatus = "unavailable"

	HackerNewsEndpoint = "https://hacker-news.firebaseio.com/v0"
)

var credentialReferencePattern = regexp.MustCompile(`^env:[A-Z_][A-Z0-9_]{0,127}$`)

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
	AllowBodyStorage      bool
	RequiresAttribution   bool
	RequiresDeletionSync  bool
	ContentRetentionDays  int
	MetricsRetentionDays  int
	AllowedLanguages      []string
	AllowedRegions        []string
	RateLimitPerMinute    int
	RequestTimeoutSeconds int
	MaxPagesPerRun        int
}

func DefaultSourceConfig() SourceConfig {
	return SourceConfig{
		AllowBodyStorage: false, RequiresAttribution: false, RequiresDeletionSync: false,
		ContentRetentionDays: 30, MetricsRetentionDays: 30,
		AllowedLanguages: []string{}, AllowedRegions: []string{},
		RateLimitPerMinute: 60, RequestTimeoutSeconds: 30, MaxPagesPerRun: 1,
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
	config := connection.Config
	if config.isZero() {
		config = DefaultSourceConfig()
	}
	config, err = config.Normalize()
	if err != nil {
		return SourceConnection{}, err
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
	return sourceType == SourceTypeRSS || sourceType == SourceTypeHackerNews
}

func (authType AuthType) Valid() bool {
	return authType == AuthTypeNone || authType == AuthTypeAPIKey || authType == AuthTypeOAuth2 || authType == AuthTypeBearer
}

func (status HealthStatus) Valid() bool {
	return status == HealthStatusUnknown || status == HealthStatusHealthy || status == HealthStatusDegraded || status == HealthStatusUnavailable
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
	if !credentialReferencePattern.MatchString(normalized) {
		return "", fmt.Errorf("credential reference must use env:NAME")
	}
	return normalized, nil
}

// NormalizeEndpoint enforces static SSRF protections that can be checked
// without network access. PLAN-006 must additionally re-check DNS answers and
// every redirect at connection time.
func NormalizeEndpoint(sourceType SourceType, value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if sourceType == SourceTypeHackerNews {
		if normalized != HackerNewsEndpoint {
			return "", fmt.Errorf("hacker news endpoint must be the official endpoint")
		}
		return HackerNewsEndpoint, nil
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
	if host == "" || net.ParseIP(host) != nil || !validDNSName(host) {
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
		default:
			return SourceConfig{}, fmt.Errorf("source config key %q is not allowed", key)
		}
	}
	return config.Normalize()
}

func (config SourceConfig) Normalize() (SourceConfig, error) {
	var err error
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
	return config, nil
}

func (config SourceConfig) isZero() bool {
	return !config.AllowBodyStorage && !config.RequiresAttribution && !config.RequiresDeletionSync && config.ContentRetentionDays == 0 && config.MetricsRetentionDays == 0 && len(config.AllowedLanguages) == 0 && len(config.AllowedRegions) == 0 && config.RateLimitPerMinute == 0 && config.RequestTimeoutSeconds == 0 && config.MaxPagesPerRun == 0
}

func (config SourceConfig) Map() map[string]any {
	return map[string]any{
		"allow_body_storage": config.AllowBodyStorage, "requires_attribution": config.RequiresAttribution, "requires_deletion_sync": config.RequiresDeletionSync,
		"content_retention_days": config.ContentRetentionDays, "metrics_retention_days": config.MetricsRetentionDays,
		"allowed_languages": append([]string(nil), config.AllowedLanguages...), "allowed_regions": append([]string(nil), config.AllowedRegions...),
		"rate_limit_per_minute": config.RateLimitPerMinute, "request_timeout_seconds": config.RequestTimeoutSeconds, "max_pages_per_run": config.MaxPagesPerRun,
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
