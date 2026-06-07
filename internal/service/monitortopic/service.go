package monitortopic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	defaultSimilarityThreshold = 0.80
	defaultCollectIntervalMin  = 30
	maxIncludeKeywords         = 50
	maxExcludeKeywords         = 100
	minCollectIntervalMin      = 5
)

// Repository defines persistence operations for monitor topics.
type Repository interface {
	CreateTopic(ctx context.Context, topic MonitorTopic) (MonitorTopic, error)
	TopicByID(ctx context.Context, topicID string) (MonitorTopic, error)
	ListTopics(ctx context.Context, userID string) ([]MonitorTopic, error)
	UpdateTopic(ctx context.Context, topic MonitorTopic) (MonitorTopic, error)
	DeleteTopic(ctx context.Context, topicID string) error
	CreateKeyword(ctx context.Context, kw TopicKeyword) (TopicKeyword, error)
	ListKeywords(ctx context.Context, topicID string) ([]TopicKeyword, error)
	DeleteKeyword(ctx context.Context, keywordID string) error
	CountKeywords(ctx context.Context, topicID string, kwType KeywordType) (int, error)
}

// Service provides monitor topic business logic.
type Service struct {
	repo Repository
	now  func() time.Time
}

// CreateTopicInput holds fields for creating a monitor topic.
type CreateTopicInput struct {
	UserID              string
	Name                string
	Description         string
	Language            Language
	Platforms           []Platform
	SimilarityThreshold *float64
	CollectIntervalMin  *int
	DailyReportEnabled  *bool
	ObsidianOutputDir   string
}

// UpdateTopicInput holds fields for updating a monitor topic.
type UpdateTopicInput struct {
	TopicID             string
	Name                *string
	Description         *string
	Language            *Language
	Platforms           []Platform
	SimilarityThreshold *float64
	CollectIntervalMin  *int
	DailyReportEnabled  *bool
	ObsidianOutputDir   *string
}

// AddKeywordInput holds fields for adding a keyword to a topic.
type AddKeywordInput struct {
	TopicID string
	Word    string
	Type    KeywordType
}

// NewService creates a new monitor topic service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// CreateTopic validates input and creates a new monitor topic.
func (s *Service) CreateTopic(ctx context.Context, input CreateTopicInput) (MonitorTopic, error) {
	if err := validateCreateInput(input); err != nil {
		return MonitorTopic{}, err
	}
	now := s.now().UTC()
	topic := MonitorTopic{
		ID:                  newID("mtp"),
		UserID:              strings.TrimSpace(input.UserID),
		Name:                strings.TrimSpace(input.Name),
		Description:         strings.TrimSpace(input.Description),
		Status:              TopicStatusDraft,
		Language:            input.Language,
		Platforms:           compactPlatforms(input.Platforms),
		SimilarityThreshold: defaultSimilarityThreshold,
		CollectIntervalMin:  defaultCollectIntervalMin,
		DailyReportEnabled:  true,
		ObsidianOutputDir:   strings.TrimSpace(input.ObsidianOutputDir),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if input.SimilarityThreshold != nil {
		topic.SimilarityThreshold = *input.SimilarityThreshold
	}
	if input.CollectIntervalMin != nil {
		topic.CollectIntervalMin = *input.CollectIntervalMin
	}
	if input.DailyReportEnabled != nil {
		topic.DailyReportEnabled = *input.DailyReportEnabled
	}
	return s.repo.CreateTopic(ctx, topic)
}

// GetTopic retrieves a topic by ID.
func (s *Service) GetTopic(ctx context.Context, topicID string) (MonitorTopic, error) {
	trimmedID := strings.TrimSpace(topicID)
	if trimmedID == "" {
		return MonitorTopic{}, ErrInvalidInput
	}
	topic, err := s.repo.TopicByID(ctx, trimmedID)
	if err != nil {
		return MonitorTopic{}, normalizeNotFound(err)
	}
	return topic, nil
}

// ListTopics returns all topics for a user.
func (s *Service) ListTopics(ctx context.Context, userID string) ([]MonitorTopic, error) {
	trimmedUserID := strings.TrimSpace(userID)
	if trimmedUserID == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListTopics(ctx, trimmedUserID)
}

// UpdateTopic validates and applies partial updates.
func (s *Service) UpdateTopic(ctx context.Context, input UpdateTopicInput) (MonitorTopic, error) {
	trimmedID := strings.TrimSpace(input.TopicID)
	if trimmedID == "" {
		return MonitorTopic{}, ErrInvalidInput
	}
	found, err := s.repo.TopicByID(ctx, trimmedID)
	if err != nil {
		return MonitorTopic{}, normalizeNotFound(err)
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return MonitorTopic{}, ErrInvalidInput
		}
		found.Name = name
	}
	if input.Description != nil {
		found.Description = strings.TrimSpace(*input.Description)
	}
	if input.Language != nil {
		if !validLanguages[*input.Language] {
			return MonitorTopic{}, ErrInvalidInput
		}
		found.Language = *input.Language
	}
	if len(input.Platforms) > 0 {
		for _, p := range input.Platforms {
			if !validPlatforms[p] {
				return MonitorTopic{}, ErrInvalidInput
			}
		}
		found.Platforms = compactPlatforms(input.Platforms)
	}
	if input.SimilarityThreshold != nil {
		if *input.SimilarityThreshold < 0 || *input.SimilarityThreshold > 1.0 {
			return MonitorTopic{}, ErrInvalidInput
		}
		found.SimilarityThreshold = *input.SimilarityThreshold
	}
	if input.CollectIntervalMin != nil {
		if *input.CollectIntervalMin < minCollectIntervalMin {
			return MonitorTopic{}, ErrInvalidInput
		}
		found.CollectIntervalMin = *input.CollectIntervalMin
	}
	if input.DailyReportEnabled != nil {
		found.DailyReportEnabled = *input.DailyReportEnabled
	}
	if input.ObsidianOutputDir != nil {
		found.ObsidianOutputDir = strings.TrimSpace(*input.ObsidianOutputDir)
	}
	found.UpdatedAt = s.now().UTC()
	return s.repo.UpdateTopic(ctx, found)
}

// SetTopicStatus validates and applies a status transition.
func (s *Service) SetTopicStatus(ctx context.Context, topicID string, status TopicStatus) (MonitorTopic, error) {
	trimmedID := strings.TrimSpace(topicID)
	if trimmedID == "" {
		return MonitorTopic{}, ErrInvalidInput
	}
	found, err := s.repo.TopicByID(ctx, trimmedID)
	if err != nil {
		return MonitorTopic{}, normalizeNotFound(err)
	}
	if !found.Status.CanTransitionTo(status) {
		return MonitorTopic{}, ErrInvalidTransition
	}
	found.Status = status
	found.UpdatedAt = s.now().UTC()
	return s.repo.UpdateTopic(ctx, found)
}

// DeleteTopic removes a topic and cascades keyword cleanup.
func (s *Service) DeleteTopic(ctx context.Context, topicID string) error {
	trimmedID := strings.TrimSpace(topicID)
	if trimmedID == "" {
		return ErrInvalidInput
	}
	return normalizeNotFound(s.repo.DeleteTopic(ctx, trimmedID))
}

// AddKeyword validates and adds a keyword/exclusion word to a topic.
func (s *Service) AddKeyword(ctx context.Context, input AddKeywordInput) (TopicKeyword, error) {
	trimmedTopicID := strings.TrimSpace(input.TopicID)
	trimmedWord := strings.TrimSpace(input.Word)
	if trimmedTopicID == "" || trimmedWord == "" {
		return TopicKeyword{}, ErrInvalidInput
	}
	if input.Type != KeywordTypeInclude && input.Type != KeywordTypeExclude {
		return TopicKeyword{}, ErrInvalidInput
	}
	// Verify topic exists
	if _, err := s.repo.TopicByID(ctx, trimmedTopicID); err != nil {
		return TopicKeyword{}, normalizeNotFound(err)
	}
	// Check limits
	var (
		count int
		limit int
		kwErr error
	)
	if input.Type == KeywordTypeInclude {
		limit = maxIncludeKeywords
		count, kwErr = s.repo.CountKeywords(ctx, trimmedTopicID, KeywordTypeInclude)
	} else {
		limit = maxExcludeKeywords
		count, kwErr = s.repo.CountKeywords(ctx, trimmedTopicID, KeywordTypeExclude)
	}
	if kwErr != nil {
		return TopicKeyword{}, normalizeNotFound(kwErr)
	}
	if count >= limit {
		return TopicKeyword{}, ErrInvalidInput
	}
	now := s.now().UTC()
	kw, err := s.repo.CreateKeyword(ctx, TopicKeyword{
		ID:        newID("mkw"),
		TopicID:   trimmedTopicID,
		Word:      trimmedWord,
		Type:      input.Type,
		CreatedAt: now,
	})
	if err != nil {
		return TopicKeyword{}, normalizeNotFound(err)
	}
	return kw, nil
}

// ListKeywords returns all keywords for a topic.
func (s *Service) ListKeywords(ctx context.Context, topicID string) ([]TopicKeyword, error) {
	trimmedID := strings.TrimSpace(topicID)
	if trimmedID == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListKeywords(ctx, trimmedID)
}

// DeleteKeyword removes a keyword by ID.
func (s *Service) DeleteKeyword(ctx context.Context, keywordID string) error {
	trimmedID := strings.TrimSpace(keywordID)
	if trimmedID == "" {
		return ErrInvalidInput
	}
	return normalizeNotFound(s.repo.DeleteKeyword(ctx, trimmedID))
}

func validateCreateInput(input CreateTopicInput) error {
	if strings.TrimSpace(input.UserID) == "" {
		return ErrInvalidInput
	}
	if strings.TrimSpace(input.Name) == "" {
		return ErrInvalidInput
	}
	if !validLanguages[input.Language] {
		return ErrInvalidInput
	}
	if len(input.Platforms) == 0 {
		return ErrInvalidInput
	}
	for _, p := range input.Platforms {
		if !validPlatforms[p] {
			return ErrInvalidInput
		}
	}
	if input.SimilarityThreshold != nil {
		if *input.SimilarityThreshold < 0 || *input.SimilarityThreshold > 1.0 {
			return ErrInvalidInput
		}
	}
	if input.CollectIntervalMin != nil {
		if *input.CollectIntervalMin < minCollectIntervalMin {
			return ErrInvalidInput
		}
	}
	return nil
}

func compactPlatforms(platforms []Platform) []Platform {
	seen := make(map[Platform]struct{})
	var result []Platform
	for _, p := range platforms {
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		result = append(result, p)
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
