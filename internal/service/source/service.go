package source

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type SourceType string

const (
	SourceTypeRSS        SourceType = "rss"
	SourceTypePublicPage SourceType = "public_page"
)

type SourceStatus string

const (
	SourceStatusEnabled  SourceStatus = "enabled"
	SourceStatusDisabled SourceStatus = "disabled"
)

type CollectionRunStatus string

const (
	CollectionRunStatusSuccess CollectionRunStatus = "success"
	CollectionRunStatusFailed  CollectionRunStatus = "failed"
)

var (
	ErrInvalidInput           = errors.New("invalid input")
	ErrComplianceNoteRequired = errors.New("compliance note required")
	ErrNotFound               = errors.New("not found")
	ErrAlreadyExists          = errors.New("already exists")
)

type Source struct {
	ID               string
	Name             string
	Type             SourceType
	URL              string
	Status           SourceStatus
	ComplianceNote   string
	FetchIntervalMin int
	RateLimitPerHour int
	ChannelIDs       []string
	LastError        string
	LastCollectedAt  *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CollectionRun struct {
	ID           string
	SourceID     string
	Status       CollectionRunStatus
	ItemsFetched int
	Error        string
	StartedAt    time.Time
	FinishedAt   time.Time
	CreatedAt    time.Time
}

type Repository interface {
	ListSources(ctx context.Context) ([]Source, error)
	ListCollectableSources(ctx context.Context) ([]Source, error)
	SourceByID(ctx context.Context, sourceID string) (Source, error)
	CreateSource(ctx context.Context, source Source) (Source, error)
	UpdateSource(ctx context.Context, source Source) (Source, error)
	CreateCollectionRun(ctx context.Context, run CollectionRun) (CollectionRun, error)
	ListCollectionRuns(ctx context.Context, sourceID string) ([]CollectionRun, error)
}

type Service struct {
	repo Repository
	now  func() time.Time
}

type CreateSourceInput struct {
	Name             string
	Type             SourceType
	URL              string
	ComplianceNote   string
	FetchIntervalMin int
	RateLimitPerHour int
	ChannelIDs       []string
}

type UpdateSourceInput struct {
	SourceID         string
	Name             string
	Type             SourceType
	URL              string
	ComplianceNote   string
	FetchIntervalMin int
	RateLimitPerHour int
	ChannelIDs       []string
}

type SetSourceStatusInput struct {
	SourceID string
	Status   SourceStatus
}

type RecordCollectionRunInput struct {
	SourceID     string
	Status       CollectionRunStatus
	ItemsFetched int
	Error        string
	StartedAt    time.Time
	FinishedAt   time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) ListSources(ctx context.Context) ([]Source, error) {
	return s.repo.ListSources(ctx)
}

func (s *Service) ListCollectableSources(ctx context.Context) ([]Source, error) {
	return s.repo.ListCollectableSources(ctx)
}

func (s *Service) SourceByID(ctx context.Context, sourceID string) (Source, error) {
	if strings.TrimSpace(sourceID) == "" {
		return Source{}, ErrInvalidInput
	}
	found, err := s.repo.SourceByID(ctx, strings.TrimSpace(sourceID))
	if err != nil {
		return Source{}, normalizeNotFound(err)
	}
	return found, nil
}

func (s *Service) CreateSource(ctx context.Context, input CreateSourceInput) (Source, error) {
	source, err := buildSource(Source{}, input.Name, input.Type, input.URL, input.ComplianceNote, input.FetchIntervalMin, input.RateLimitPerHour, input.ChannelIDs)
	if err != nil {
		return Source{}, err
	}
	now := s.now().UTC()
	source.ID = newID("src")
	source.Status = SourceStatusEnabled
	source.CreatedAt = now
	source.UpdatedAt = now
	return s.repo.CreateSource(ctx, source)
}

func (s *Service) UpdateSource(ctx context.Context, input UpdateSourceInput) (Source, error) {
	input.SourceID = strings.TrimSpace(input.SourceID)
	if input.SourceID == "" {
		return Source{}, ErrInvalidInput
	}
	found, err := s.repo.SourceByID(ctx, input.SourceID)
	if err != nil {
		return Source{}, normalizeNotFound(err)
	}
	updated, err := buildSource(found, input.Name, input.Type, input.URL, input.ComplianceNote, input.FetchIntervalMin, input.RateLimitPerHour, input.ChannelIDs)
	if err != nil {
		return Source{}, err
	}
	updated.UpdatedAt = s.now().UTC()
	return s.repo.UpdateSource(ctx, updated)
}

func (s *Service) SetSourceStatus(ctx context.Context, input SetSourceStatusInput) (Source, error) {
	input.SourceID = strings.TrimSpace(input.SourceID)
	if input.SourceID == "" {
		return Source{}, ErrInvalidInput
	}
	if input.Status != SourceStatusEnabled && input.Status != SourceStatusDisabled {
		return Source{}, ErrInvalidInput
	}
	found, err := s.repo.SourceByID(ctx, input.SourceID)
	if err != nil {
		return Source{}, normalizeNotFound(err)
	}
	found.Status = input.Status
	found.UpdatedAt = s.now().UTC()
	return s.repo.UpdateSource(ctx, found)
}

func (s *Service) RecordCollectionRun(ctx context.Context, input RecordCollectionRunInput) (CollectionRun, error) {
	if strings.TrimSpace(input.SourceID) == "" {
		return CollectionRun{}, ErrInvalidInput
	}
	if input.Status != CollectionRunStatusSuccess && input.Status != CollectionRunStatusFailed {
		return CollectionRun{}, ErrInvalidInput
	}
	if input.Status == CollectionRunStatusFailed && strings.TrimSpace(input.Error) == "" {
		return CollectionRun{}, ErrInvalidInput
	}
	now := s.now().UTC()
	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = now
	}
	finishedAt := input.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = now
	}
	if input.ItemsFetched < 0 {
		return CollectionRun{}, ErrInvalidInput
	}
	if finishedAt.Before(startedAt) {
		return CollectionRun{}, ErrInvalidInput
	}
	return s.repo.CreateCollectionRun(ctx, CollectionRun{
		ID:           newID("run"),
		SourceID:     strings.TrimSpace(input.SourceID),
		Status:       input.Status,
		ItemsFetched: input.ItemsFetched,
		Error:        strings.TrimSpace(input.Error),
		StartedAt:    startedAt.UTC(),
		FinishedAt:   finishedAt.UTC(),
		CreatedAt:    now,
	})
}

func (s *Service) ListCollectionRuns(ctx context.Context, sourceID string) ([]CollectionRun, error) {
	if strings.TrimSpace(sourceID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListCollectionRuns(ctx, strings.TrimSpace(sourceID))
}

func buildSource(existing Source, name string, sourceType SourceType, rawURL string, complianceNote string, fetchIntervalMin int, rateLimitPerHour int, channelIDs []string) (Source, error) {
	name = strings.TrimSpace(name)
	rawURL = strings.TrimSpace(rawURL)
	complianceNote = strings.TrimSpace(complianceNote)
	if name == "" || rawURL == "" || !validSourceType(sourceType) {
		return Source{}, ErrInvalidInput
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Source{}, ErrInvalidInput
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Source{}, ErrInvalidInput
	}
	if sourceType == SourceTypePublicPage && complianceNote == "" {
		return Source{}, ErrComplianceNoteRequired
	}
	if fetchIntervalMin <= 0 {
		return Source{}, ErrInvalidInput
	}
	if rateLimitPerHour < 0 {
		return Source{}, ErrInvalidInput
	}
	existing.Name = name
	existing.Type = sourceType
	existing.URL = rawURL
	existing.ComplianceNote = complianceNote
	existing.FetchIntervalMin = fetchIntervalMin
	existing.RateLimitPerHour = rateLimitPerHour
	existing.ChannelIDs = compactUnique(channelIDs)
	return existing, nil
}

func validSourceType(sourceType SourceType) bool {
	return sourceType == SourceTypeRSS || sourceType == SourceTypePublicPage
}

func compactUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeNotFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func newID(prefix string) string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(data[:])
}
