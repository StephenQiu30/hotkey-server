package domain

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"
)

const MaxSourcePartyAssertions = 32

type SourcePartyRole string

const (
	SourcePartyRolePublisher     SourcePartyRole = "publisher"
	SourcePartyRoleAuthor        SourcePartyRole = "author"
	SourcePartyRoleDistributor   SourcePartyRole = "distributor"
	SourcePartyRoleContentOrigin SourcePartyRole = "content_origin"
)

func (role SourcePartyRole) Valid() bool {
	switch role {
	case SourcePartyRolePublisher, SourcePartyRoleAuthor, SourcePartyRoleDistributor, SourcePartyRoleContentOrigin:
		return true
	default:
		return false
	}
}

type SourcePartyKind string

const (
	SourcePartyKindOrganization SourcePartyKind = "organization"
	SourcePartyKindPerson       SourcePartyKind = "person"
	SourcePartyKindAccount      SourcePartyKind = "account"
)

func (kind SourcePartyKind) Valid() bool {
	switch kind {
	case SourcePartyKindOrganization, SourcePartyKindPerson, SourcePartyKindAccount:
		return true
	default:
		return false
	}
}

// SourcePartyAssertion is an explicit upstream assertion about a party's role
// in one observed work. It is not inferred from URLs, author strings, source
// connection names, or model output.
type SourcePartyAssertion struct {
	Role              SourcePartyRole
	Kind              SourcePartyKind
	IdentityNamespace string
	ExternalID        string
	DisplayName       string
	HomepageURL       string
}

func NormalizeSourceParties(values []SourcePartyAssertion) ([]SourcePartyAssertion, error) {
	if len(values) > MaxSourcePartyAssertions {
		return nil, fmt.Errorf("source party assertion count exceeds %d", MaxSourcePartyAssertions)
	}
	if len(values) == 0 {
		return []SourcePartyAssertion{}, nil
	}
	normalized := make([]SourcePartyAssertion, len(values))
	unique := make(map[string]struct{}, len(values))
	singletonRoles := make(map[SourcePartyRole]struct{}, 2)
	for index, value := range values {
		value.IdentityNamespace = strings.ToLower(strings.TrimSpace(value.IdentityNamespace))
		value.ExternalID = strings.TrimSpace(value.ExternalID)
		value.DisplayName = strings.TrimSpace(value.DisplayName)
		value.HomepageURL = strings.TrimSpace(value.HomepageURL)
		if !value.Role.Valid() || !value.Kind.Valid() || !validPartyNamespace(value.IdentityNamespace) ||
			!validPartyText(value.ExternalID, 512) || !validPartyText(value.DisplayName, 512) {
			return nil, fmt.Errorf("source party assertion is invalid")
		}
		if value.HomepageURL != "" && !validPartyURL(value.HomepageURL) {
			return nil, fmt.Errorf("source party homepage URL is invalid")
		}
		if value.Role == SourcePartyRolePublisher || value.Role == SourcePartyRoleContentOrigin {
			if _, found := singletonRoles[value.Role]; found {
				return nil, fmt.Errorf("source observation has ambiguous %s parties", value.Role)
			}
			singletonRoles[value.Role] = struct{}{}
		}
		identity := string(value.Role) + "\x00" + value.IdentityNamespace + "\x00" + value.ExternalID
		if _, found := unique[identity]; found {
			return nil, fmt.Errorf("source party assertion is duplicated")
		}
		unique[identity] = struct{}{}
		normalized[index] = value
	}
	sort.Slice(normalized, func(left, right int) bool {
		leftKey := sourcePartyRoleSortKey(normalized[left].Role) + "\x00" + normalized[left].IdentityNamespace + "\x00" + normalized[left].ExternalID
		rightKey := sourcePartyRoleSortKey(normalized[right].Role) + "\x00" + normalized[right].IdentityNamespace + "\x00" + normalized[right].ExternalID
		return leftKey < rightKey
	})
	return normalized, nil
}

func sourcePartyRoleSortKey(role SourcePartyRole) string {
	switch role {
	case SourcePartyRolePublisher:
		return "0"
	case SourcePartyRoleContentOrigin:
		return "1"
	case SourcePartyRoleDistributor:
		return "2"
	default:
		return "3"
	}
}

func validPartyNamespace(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index, character := range value {
		alphanumeric := character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
		if alphanumeric || index > 0 && (character == '-' || character == '_' || character == '.' || character == ':') {
			continue
		}
		return false
	}
	return true
}

func validPartyText(value string, maximumBytes int) bool {
	if value == "" || len(value) > maximumBytes || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validPartyURL(value string) bool {
	if len(value) > 2048 || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed != nil && parsed.IsAbs() && parsed.User == nil && parsed.Fragment == "" &&
		(parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != ""
}
