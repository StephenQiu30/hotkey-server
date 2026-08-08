package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
)

const (
	MaximumActiveTokens = 10
	MinimumLifetime     = 24 * time.Hour
	MaximumLifetime     = 90 * 24 * time.Hour
)

type Scope string

const (
	ScopeMonitorsRead Scope = "monitors.read"
	ScopeEventsRead   Scope = "events.read"
	ScopeContentsRead Scope = "contents.read"
	ScopeReportsRead  Scope = "reports.read"
	ScopeSearchRun    Scope = "search.run"
	ScopeAlertsWrite  Scope = "alerts.write"
)

var allScopes = map[Scope]struct{}{
	ScopeMonitorsRead: {}, ScopeEventsRead: {}, ScopeContentsRead: {},
	ScopeReportsRead: {}, ScopeSearchRun: {}, ScopeAlertsWrite: {},
}

type Token struct {
	ID          int64
	Version     int64
	UserID      int64
	Name        string
	TokenPrefix string
	TokenHash   string
	Scopes      []Scope
	ExpiresAt   time.Time
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Principal struct {
	TokenID int64
	UserID  int64
	Role    identitydomain.Role
	Scopes  []Scope
}

func NormalizeScopes(values []Scope) ([]Scope, error) {
	seen := make(map[Scope]struct{}, len(values))
	for _, value := range values {
		scope := Scope(strings.TrimSpace(string(value)))
		if _, ok := allScopes[scope]; !ok {
			return nil, fmt.Errorf("unknown agent scope %q", value)
		}
		seen[scope] = struct{}{}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("at least one agent scope is required")
	}
	result := make([]Scope, 0, len(seen))
	for scope := range seen {
		result = append(result, scope)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func ScopesAllowedForRole(role identitydomain.Role, scopes []Scope) bool {
	if !role.Valid() {
		return false
	}
	for _, scope := range scopes {
		if _, ok := allScopes[scope]; !ok {
			return false
		}
		if scope == ScopeSearchRun && role == identitydomain.RoleViewer {
			return false
		}
	}
	return true
}

func (token Token) Validate(now time.Time) error {
	name := strings.TrimSpace(token.Name)
	if token.UserID <= 0 || name != token.Name || len([]rune(name)) == 0 || len([]rune(name)) > 64 {
		return fmt.Errorf("invalid agent token owner or name")
	}
	scopes, err := NormalizeScopes(token.Scopes)
	if err != nil || len(scopes) != len(token.Scopes) {
		return fmt.Errorf("invalid agent token scopes")
	}
	lifetime := token.ExpiresAt.UTC().Sub(now.UTC())
	if lifetime < MinimumLifetime || lifetime > MaximumLifetime {
		return fmt.Errorf("agent token lifetime must be between 1 and 90 days")
	}
	return nil
}

func ScopeStrings(scopes []Scope) []string {
	values := make([]string, len(scopes))
	for index, scope := range scopes {
		values[index] = string(scope)
	}
	return values
}
