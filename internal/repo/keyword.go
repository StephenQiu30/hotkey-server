package repo

import "github.com/StephenQiu30/hotkey-server/internal/keyword"

// KeywordRepo defines the storage interface for platform keywords and user preferences.
type KeywordRepo interface {
	CreateKeyword(kw keyword.PlatformKeyword) error
	ListKeywords() ([]keyword.PlatformKeyword, error)
	ListKeywordsByTenant(tenantID string) ([]keyword.PlatformKeyword, error)
	GetKeyword(id string) (keyword.PlatformKeyword, error)
	UpdateKeyword(kw keyword.PlatformKeyword) error

	GetUserPreferences(userID string) (keyword.UserPreferences, error)
	SaveUserPreferences(userID string, prefs keyword.UserPreferences) error
}
