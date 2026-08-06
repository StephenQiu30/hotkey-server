// Package domain contains Monitor control-plane facts and validation. It is
// deliberately free of transport and persistence dependencies.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

type MonitorStatus string

const (
	MonitorStatusDraft    MonitorStatus = "draft"
	MonitorStatusActive   MonitorStatus = "active"
	MonitorStatusPaused   MonitorStatus = "paused"
	MonitorStatusArchived MonitorStatus = "archived"
)

func (status MonitorStatus) Valid() bool {
	switch status {
	case MonitorStatusDraft, MonitorStatusActive, MonitorStatusPaused, MonitorStatusArchived:
		return true
	default:
		return false
	}
}

// CanTransition captures only real state changes. Application services decide
// idempotency after they have verified the command's expected Monitor version.
func CanTransition(from, to MonitorStatus) bool {
	switch from {
	case MonitorStatusDraft:
		return to == MonitorStatusActive || to == MonitorStatusArchived
	case MonitorStatusActive:
		return to == MonitorStatusPaused || to == MonitorStatusArchived
	case MonitorStatusPaused:
		return to == MonitorStatusActive || to == MonitorStatusArchived
	case MonitorStatusArchived:
		return to == MonitorStatusPaused
	default:
		return false
	}
}

type ConfigVersionState string

const (
	ConfigVersionDraft      ConfigVersionState = "draft"
	ConfigVersionPublished  ConfigVersionState = "published"
	ConfigVersionSuperseded ConfigVersionState = "superseded"
)

func (state ConfigVersionState) Valid() bool {
	return state == ConfigVersionDraft || state == ConfigVersionPublished || state == ConfigVersionSuperseded
}

type Monitor struct {
	ID                       int64
	Version                  int64
	Name                     string
	Description              string
	Status                   MonitorStatus
	DraftConfigVersionID     *int64
	PublishedConfigVersionID *int64
	CreatedAt                time.Time
	UpdatedAt                time.Time
	DeletedAt                *time.Time
}

type MonitorConfig struct {
	Timezone                  string
	Languages                 []string
	Regions                   []string
	CollectionIntervalSeconds int
	RelevanceThreshold        float64
	EventThreshold            float64
	RetentionDays             int
}

type MonitorConfigVersion struct {
	ID          int64
	Version     int64
	MonitorID   int64
	Revision    int64
	State       ConfigVersionState
	Config      MonitorConfig
	ConfigHash  string
	PublishedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ExpectedVersions is deliberately shaped like the HTTP command contract.
// A nil DraftVersion is meaningful only for first-draft creation from a
// currently published Monitor; it is not a missing value.
type ExpectedVersions struct {
	MonitorVersion int64
	DraftVersion   *int64
}

func (expected ExpectedVersions) ValidateMonitor() error {
	if expected.MonitorVersion <= 0 {
		return fmt.Errorf("monitor version must be positive")
	}
	return nil
}

func (expected ExpectedVersions) ValidateDraft(draftExists bool) error {
	if err := expected.ValidateMonitor(); err != nil {
		return err
	}
	if draftExists && (expected.DraftVersion == nil || *expected.DraftVersion <= 0) {
		return fmt.Errorf("existing draft requires a positive expected draft version")
	}
	if !draftExists && expected.DraftVersion != nil {
		return fmt.Errorf("first draft requires an explicit null expected draft version")
	}
	return nil
}

func NormalizeMonitorName(value string) (string, error) {
	normalized := normalizeText(value)
	if length := utf8.RuneCountInString(normalized); length < 1 || length > 120 {
		return "", fmt.Errorf("monitor name must be 1-120 Unicode code points")
	}
	return normalized, nil
}

func NormalizeMonitorDescription(value string) (string, error) {
	normalized := normalizeText(value)
	if utf8.RuneCountInString(normalized) > 2000 {
		return "", fmt.Errorf("monitor description must be at most 2000 Unicode code points")
	}
	return normalized, nil
}

func NormalizeMonitorConfig(config MonitorConfig) (MonitorConfig, error) {
	if _, err := time.LoadLocation(strings.TrimSpace(config.Timezone)); err != nil {
		return MonitorConfig{}, fmt.Errorf("invalid IANA timezone: %w", err)
	}
	config.Timezone = strings.TrimSpace(config.Timezone)
	var err error
	if config.Languages, err = NormalizeLanguages(config.Languages, 1, 8); err != nil {
		return MonitorConfig{}, err
	}
	if config.Regions, err = NormalizeRegions(config.Regions, 0, 8); err != nil {
		return MonitorConfig{}, err
	}
	if config.CollectionIntervalSeconds < 300 || config.CollectionIntervalSeconds > 86_400 || config.CollectionIntervalSeconds%60 != 0 {
		return MonitorConfig{}, fmt.Errorf("collection interval must be a 60-second multiple from 300 to 86400")
	}
	if config.RelevanceThreshold < 60 || config.RelevanceThreshold > 100 {
		return MonitorConfig{}, fmt.Errorf("relevance threshold must be from 60 to 100")
	}
	if config.EventThreshold < 0 || config.EventThreshold > 100 {
		return MonitorConfig{}, fmt.Errorf("event threshold must be from 0 to 100")
	}
	if config.RetentionDays < 1 || config.RetentionDays > 3650 {
		return MonitorConfig{}, fmt.Errorf("retention days must be from 1 to 3650")
	}
	return config, nil
}

func NormalizeLanguages(values []string, min, max int) ([]string, error) {
	if len(values) < min || len(values) > max {
		return nil, fmt.Errorf("language count must be from %d to %d", min, max)
	}
	set := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := normalizeText(raw)
		if value == "" || strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("language tag is invalid")
		}
		tag, err := language.Parse(value)
		if err != nil || tag == language.Und {
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

func NormalizeRegions(values []string, min, max int) ([]string, error) {
	if len(values) < min || len(values) > max {
		return nil, fmt.Errorf("region count must be from %d to %d", min, max)
	}
	set := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.ToUpper(normalizeText(raw))
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

// ConfigHashInput contains every configuration fact that influences immutable
// Monitor configuration identity. The deterministic implementation never
// serializes a map.
type ConfigHashInput struct {
	MonitorID int64
	Revision  int64
	Config    MonitorConfig
	Rules     []MonitorRule
	Sources   []MonitorSource
}

func CanonicalConfigHash(input ConfigHashInput) (string, error) {
	config, err := NormalizeMonitorConfig(input.Config)
	if err != nil {
		return "", err
	}
	if input.MonitorID <= 0 || input.Revision <= 0 {
		return "", fmt.Errorf("monitor id and revision must be positive")
	}
	rules, err := canonicalRules(input.Rules)
	if err != nil {
		return "", err
	}
	sources, err := canonicalSources(input.Sources)
	if err != nil {
		return "", err
	}
	payload := struct {
		HashVersion int               `json:"hash_version"`
		MonitorID   int64             `json:"monitor_id"`
		Revision    int64             `json:"revision"`
		Timezone    string            `json:"timezone"`
		Languages   []string          `json:"languages"`
		Regions     []string          `json:"regions"`
		Interval    int               `json:"collection_interval_seconds"`
		Relevance   float64           `json:"relevance_threshold"`
		Event       float64           `json:"event_threshold"`
		Retention   int               `json:"retention_days"`
		Rules       []canonicalRule   `json:"rules"`
		Sources     []canonicalSource `json:"sources"`
	}{
		HashVersion: 1, MonitorID: input.MonitorID, Revision: input.Revision,
		Timezone: config.Timezone, Languages: config.Languages, Regions: config.Regions,
		Interval: config.CollectionIntervalSeconds, Relevance: config.RelevanceThreshold,
		Event: config.EventThreshold, Retention: config.RetentionDays, Rules: rules, Sources: sources,
	}
	return sha256JSON(payload)
}

func normalizeText(value string) string {
	return norm.NFC.String(strings.TrimSpace(value))
}

func sha256JSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
