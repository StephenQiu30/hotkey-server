package source

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

const (
	LayerFact   = "fact"
	LayerSignal = "signal"

	AccessModeOfficialAPI = "official_api"
	AccessModePublicFeed  = "public_feed"
	AccessModeBypass      = "bypass"
)

var (
	ErrInvalidSourceConfig = errors.New("invalid source config")
	ErrNonCompliantSource  = errors.New("non-compliant source")
	ErrSourceNotFound      = errors.New("source not found")
)

type Source struct {
	TenantID               string   `json:"tenantId,omitempty"`
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	Layer                  string   `json:"layer"`
	Region                 string   `json:"region"`
	Language               string   `json:"language"`
	Categories             []string `json:"categories"`
	AccessMode             string   `json:"accessMode"`
	AuthRequired           bool     `json:"authRequired"`
	Enabled                bool     `json:"enabled"`
	RateLimitPerHour       int      `json:"rateLimitPerHour"`
	RefreshIntervalMinutes int      `json:"refreshIntervalMinutes"`
	ComplianceNote         string   `json:"complianceNote"`
	LastStatus             string   `json:"lastStatus"`
}

type UpdateSourceConfigInput struct {
	Enabled          *bool
	RateLimitPerHour *int
}

type Service struct {
	mu      sync.Mutex
	sources map[string]Source
}

func NewService() *Service {
	service := NewEmptyService()
	mustRegister(service, Source{
		ID:                     "arxiv-ai",
		Name:                   "arXiv AI",
		Layer:                  LayerFact,
		Region:                 "global",
		Language:               "en",
		Categories:             []string{"research", "model", "ai"},
		AccessMode:             AccessModePublicFeed,
		AuthRequired:           false,
		Enabled:                true,
		RateLimitPerHour:       30,
		RefreshIntervalMinutes: 180,
		ComplianceNote:         "Use public metadata feeds and respect provider rate limits.",
		LastStatus:             "ready",
	})
	mustRegister(service, Source{
		ID:                     "github-trending-ai",
		Name:                   "GitHub Trending AI",
		Layer:                  LayerSignal,
		Region:                 "global",
		Language:               "en",
		Categories:             []string{"repository", "developer-signal", "ai"},
		AccessMode:             AccessModePublicFeed,
		AuthRequired:           false,
		Enabled:                true,
		RateLimitPerHour:       12,
		RefreshIntervalMinutes: 240,
		ComplianceNote:         "Use public pages or official API where available; no auth bypass or token capture.",
		LastStatus:             "ready",
	})
	return service
}

func NewEmptyService() *Service {
	service := &Service{
		sources: make(map[string]Source),
	}
	return service
}

func mustRegister(service *Service, src Source) {
	if err := service.RegisterSource(src); err != nil {
		panic(err)
	}
}

func (s *Service) RegisterSource(src Source) error {
	normalized, err := normalizeSource(src)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sources[sourceKey(normalized.TenantID, normalized.ID)] = normalized
	return nil
}

func (s *Service) ListSources() []Source {
	s.mu.Lock()
	defer s.mu.Unlock()

	sources := make([]Source, 0, len(s.sources))
	for _, src := range s.sources {
		sources = append(sources, cloneSource(src))
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].ID < sources[j].ID
	})
	return sources
}

func (s *Service) ListSourcesByTenant(tenantID string) []Source {
	tenantID = strings.TrimSpace(tenantID)
	s.mu.Lock()
	defer s.mu.Unlock()

	sources := make([]Source, 0)
	for _, src := range s.sources {
		if src.TenantID != tenantID {
			continue
		}
		sources = append(sources, cloneSource(src))
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].ID < sources[j].ID
	})
	return sources
}

func (s *Service) UpdateSourceConfig(id string, input UpdateSourceConfigInput) (Source, error) {
	return s.UpdateTenantSourceConfig("", id, input)
}

func (s *Service) UpdateTenantSourceConfig(tenantID string, id string, input UpdateSourceConfigInput) (Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := sourceKey(strings.TrimSpace(tenantID), id)
	src, ok := s.sources[key]
	if !ok {
		return Source{}, ErrSourceNotFound
	}
	if input.Enabled != nil {
		src.Enabled = *input.Enabled
	}
	if input.RateLimitPerHour != nil {
		if *input.RateLimitPerHour <= 0 {
			return Source{}, ErrInvalidSourceConfig
		}
		src.RateLimitPerHour = *input.RateLimitPerHour
	}
	s.sources[key] = src
	return cloneSource(src), nil
}

func (s *Service) RecordSourceStatus(id string, status string) error {
	normalized := strings.TrimSpace(status)
	if normalized == "" {
		return ErrInvalidSourceConfig
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	src, ok := s.sources[sourceKey("", id)]
	if !ok {
		return ErrSourceNotFound
	}
	src.LastStatus = normalized
	s.sources[sourceKey("", id)] = src
	return nil
}

func normalizeSource(src Source) (Source, error) {
	src.ID = strings.TrimSpace(src.ID)
	src.TenantID = strings.TrimSpace(src.TenantID)
	src.Name = strings.TrimSpace(src.Name)
	src.Layer = strings.TrimSpace(src.Layer)
	src.Region = strings.TrimSpace(src.Region)
	src.Language = strings.TrimSpace(src.Language)
	src.AccessMode = strings.TrimSpace(src.AccessMode)
	src.ComplianceNote = strings.TrimSpace(src.ComplianceNote)
	src.LastStatus = strings.TrimSpace(src.LastStatus)

	if src.ID == "" || src.Name == "" || src.Region == "" || src.Language == "" {
		return Source{}, ErrInvalidSourceConfig
	}
	if src.Layer != LayerFact && src.Layer != LayerSignal {
		return Source{}, ErrInvalidSourceConfig
	}
	if src.AccessMode == AccessModeBypass {
		return Source{}, ErrNonCompliantSource
	}
	if src.AccessMode != AccessModeOfficialAPI && src.AccessMode != AccessModePublicFeed {
		return Source{}, ErrInvalidSourceConfig
	}
	if src.RateLimitPerHour <= 0 || src.RefreshIntervalMinutes <= 0 {
		return Source{}, ErrInvalidSourceConfig
	}
	if src.LastStatus == "" {
		src.LastStatus = "ready"
	}
	if !src.Enabled {
		src.Enabled = true
	}

	src.Categories = normalizeCategories(src.Categories)
	if len(src.Categories) == 0 {
		return Source{}, ErrInvalidSourceConfig
	}
	return src, nil
}

func normalizeCategories(categories []string) []string {
	seen := make(map[string]struct{})
	normalized := make([]string, 0, len(categories))
	for _, category := range categories {
		value := strings.TrimSpace(category)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func cloneSource(src Source) Source {
	src.Categories = append([]string(nil), src.Categories...)
	return src
}

func sourceKey(tenantID string, id string) string {
	return strings.TrimSpace(tenantID) + ":" + strings.TrimSpace(id)
}
