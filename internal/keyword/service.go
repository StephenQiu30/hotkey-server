package keyword

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrInvalidKeyword  = errors.New("invalid keyword")
	ErrInvalidUserID   = errors.New("invalid user id")
	ErrKeywordNotFound = errors.New("keyword not found")
)

type PlatformKeyword struct {
	ID       string `json:"id"`
	Term     string `json:"term"`
	Category string `json:"category"`
	Enabled  bool   `json:"enabled"`
}

type CreatePlatformKeywordInput struct {
	Term     string
	Category string
}

type UserPreferences struct {
	UserID             string   `json:"userId"`
	FollowedKeywords   []string `json:"followedKeywords"`
	BlockedKeywords    []string `json:"blockedKeywords"`
	AdditionalKeywords []string `json:"additionalKeywords"`
}

type Service struct {
	mu                sync.Mutex
	nextKeywordNumber int
	platformKeywords  map[string]PlatformKeyword
	userPreferences   map[string]preferenceSets
}

type preferenceSets struct {
	followed   map[string]string
	blocked    map[string]string
	additional map[string]string
}

func NewService() *Service {
	return &Service{
		nextKeywordNumber: 1,
		platformKeywords:  make(map[string]PlatformKeyword),
		userPreferences:   make(map[string]preferenceSets),
	}
}

func (s *Service) CreatePlatformKeyword(input CreatePlatformKeywordInput) (PlatformKeyword, error) {
	term, err := normalizeKeyword(input.Term)
	if err != nil {
		return PlatformKeyword{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	keyword := PlatformKeyword{
		ID:       fmt.Sprintf("kw_%d", s.nextKeywordNumber),
		Term:     term,
		Category: strings.TrimSpace(input.Category),
		Enabled:  true,
	}
	s.nextKeywordNumber++
	s.platformKeywords[keyword.ID] = keyword
	return keyword, nil
}

func (s *Service) ListPlatformKeywords() []PlatformKeyword {
	s.mu.Lock()
	defer s.mu.Unlock()

	keywords := make([]PlatformKeyword, 0, len(s.platformKeywords))
	for _, keyword := range s.platformKeywords {
		keywords = append(keywords, keyword)
	}
	sort.Slice(keywords, func(i, j int) bool {
		return keywords[i].ID < keywords[j].ID
	})
	return keywords
}

func (s *Service) SetPlatformKeywordEnabled(id string, enabled bool) (PlatformKeyword, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keyword, ok := s.platformKeywords[id]
	if !ok {
		return PlatformKeyword{}, ErrKeywordNotFound
	}
	keyword.Enabled = enabled
	s.platformKeywords[id] = keyword
	return keyword, nil
}

func (s *Service) FollowKeyword(userID string, term string) error {
	return s.updatePreference(userID, term, func(sets preferenceSets, normalized string) {
		delete(sets.blocked, normalized)
		sets.followed[normalized] = normalized
	})
}

func (s *Service) BlockKeyword(userID string, term string) error {
	return s.updatePreference(userID, term, func(sets preferenceSets, normalized string) {
		delete(sets.followed, normalized)
		sets.blocked[normalized] = normalized
	})
}

func (s *Service) AddUserKeyword(userID string, term string) error {
	return s.updatePreference(userID, term, func(sets preferenceSets, normalized string) {
		sets.additional[normalized] = normalized
	})
}

func (s *Service) GetUserPreferences(userID string) UserPreferences {
	s.mu.Lock()
	defer s.mu.Unlock()

	sets := s.preferenceSetsFor(userID)
	return UserPreferences{
		UserID:             userID,
		FollowedKeywords:   sortedValues(sets.followed),
		BlockedKeywords:    sortedValues(sets.blocked),
		AdditionalKeywords: sortedValues(sets.additional),
	}
}

func (s *Service) updatePreference(userID string, term string, update func(preferenceSets, string)) error {
	if strings.TrimSpace(userID) == "" {
		return ErrInvalidUserID
	}
	normalized, err := normalizeKeyword(term)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sets := s.preferenceSetsFor(userID)
	update(sets, normalized)
	s.userPreferences[userID] = sets
	return nil
}

func (s *Service) preferenceSetsFor(userID string) preferenceSets {
	sets, ok := s.userPreferences[userID]
	if ok {
		return sets
	}
	return preferenceSets{
		followed:   make(map[string]string),
		blocked:    make(map[string]string),
		additional: make(map[string]string),
	}
}

func normalizeKeyword(term string) (string, error) {
	normalized := strings.TrimSpace(term)
	if normalized == "" {
		return "", ErrInvalidKeyword
	}
	return normalized, nil
}

func sortedValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
