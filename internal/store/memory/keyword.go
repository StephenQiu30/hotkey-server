package memory

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/StephenQiu30/hotkey-server/internal/keyword"
)

// KeywordRepo is an in-memory implementation of repo.KeywordRepo.
type KeywordRepo struct {
	mu                sync.Mutex
	nextKeywordNumber int
	platformKeywords  map[string]keyword.PlatformKeyword
	userPreferences   map[string]keyword.UserPreferences
}

func NewKeywordRepo() *KeywordRepo {
	return &KeywordRepo{
		nextKeywordNumber: 1,
		platformKeywords:  make(map[string]keyword.PlatformKeyword),
		userPreferences:   make(map[string]keyword.UserPreferences),
	}
}

func (r *KeywordRepo) CreateKeyword(kw keyword.PlatformKeyword) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if kw.ID == "" {
		kw.ID = fmt.Sprintf("kw_%d", r.nextKeywordNumber)
		r.nextKeywordNumber++
	}
	r.platformKeywords[kw.ID] = kw
	return nil
}

func (r *KeywordRepo) ListKeywords() ([]keyword.PlatformKeyword, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keywords := make([]keyword.PlatformKeyword, 0, len(r.platformKeywords))
	for _, kw := range r.platformKeywords {
		keywords = append(keywords, kw)
	}
	sort.Slice(keywords, func(i, j int) bool {
		return keywords[i].ID < keywords[j].ID
	})
	return keywords, nil
}

func (r *KeywordRepo) ListKeywordsByTenant(tenantID string) ([]keyword.PlatformKeyword, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keywords := make([]keyword.PlatformKeyword, 0)
	for _, kw := range r.platformKeywords {
		if kw.TenantID == strings.TrimSpace(tenantID) {
			keywords = append(keywords, kw)
		}
	}
	sort.Slice(keywords, func(i, j int) bool {
		return keywords[i].ID < keywords[j].ID
	})
	return keywords, nil
}

func (r *KeywordRepo) GetKeyword(id string) (keyword.PlatformKeyword, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kw, ok := r.platformKeywords[id]
	if !ok {
		return keyword.PlatformKeyword{}, keyword.ErrKeywordNotFound
	}
	return kw, nil
}

func (r *KeywordRepo) UpdateKeyword(kw keyword.PlatformKeyword) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.platformKeywords[kw.ID]; !ok {
		return keyword.ErrKeywordNotFound
	}
	r.platformKeywords[kw.ID] = kw
	return nil
}

func (r *KeywordRepo) GetUserPreferences(userID string) (keyword.UserPreferences, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prefs, ok := r.userPreferences[userID]
	if !ok {
		return keyword.UserPreferences{UserID: userID}, nil
	}
	return prefs, nil
}

func (r *KeywordRepo) SaveUserPreferences(userID string, prefs keyword.UserPreferences) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userPreferences[userID] = prefs
	return nil
}
