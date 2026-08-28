package domain

import (
	"errors"
	stdhtml "html"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var (
	ErrVaultPathInvalid   = errors.New("vault path rejected: invalid")
	ErrVaultPathSymlink   = errors.New("vault path rejected: symlink")
	ErrVaultContentUnsafe = errors.New("vault content rejected: unsafe")
)

const (
	VaultReasonPathInvalid   = "vault_path_invalid"
	VaultReasonPathSymlink   = "vault_path_symlink"
	VaultReasonContentUnsafe = "vault_content_unsafe"
)

var (
	vaultStableKeyPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	vaultRawHTMLPattern        = regexp.MustCompile(`(?is)<\s*/?\s*[a-z][^>]*>`)
	vaultEventAttributePattern = regexp.MustCompile(`(?i)\bon[a-z0-9_-]+\s*=`)
)

// ValidateVaultLocation accepts only a controlled projection namespace and
// an ASCII stable key. Encoded or platform-specific separators are rejected
// instead of being decoded differently by later layers.
func ValidateVaultLocation(kind, key string) error {
	if !legacyKnowledgeKind(kind) || !vaultStableKeyPattern.MatchString(key) ||
		strings.Contains(key, "..") || strings.HasSuffix(key, ".") || strings.ContainsAny(key, "%/\\\x00\r\n") {
		return ErrVaultPathInvalid
	}
	return nil
}

// ValidateVaultMarkdown rejects active content after repeatedly decoding the
// two common obfuscation layers used in Markdown destinations. The policy is
// intentionally reject-only: automatic publishing never rewrites an unsafe
// payload into a representation whose meaning could vary by renderer.
func ValidateVaultMarkdown(value string) error {
	if len(value) > 256*1024 {
		return ErrVaultContentUnsafe
	}
	candidates := []string{strings.ToLower(value)}
	seen := map[string]struct{}{candidates[0]: {}}
	for index := 0; index < len(candidates) && index < 16; index++ {
		for _, decoded := range []string{stdhtml.UnescapeString(candidates[index]), queryUnescapeVaultContent(candidates[index])} {
			if decoded == candidates[index] {
				continue
			}
			if _, found := seen[decoded]; !found {
				seen[decoded] = struct{}{}
				candidates = append(candidates, decoded)
			}
		}
	}
	for _, candidate := range candidates {
		if strings.IndexByte(candidate, 0) >= 0 || vaultRawHTMLPattern.MatchString(candidate) || vaultEventAttributePattern.MatchString(candidate) {
			return ErrVaultContentUnsafe
		}
		compact := strings.Map(func(character rune) rune {
			if unicode.IsSpace(character) || unicode.IsControl(character) {
				return -1
			}
			return character
		}, candidate)
		for _, marker := range []string{"javascript:", "vbscript:", "data:text/html", "data:image/svg+xml", "file:", "srcdoc=", "expression("} {
			if strings.Contains(compact, marker) {
				return ErrVaultContentUnsafe
			}
		}
	}
	return nil
}

func VaultRejectionReason(err error) string {
	switch {
	case errors.Is(err, ErrVaultPathInvalid):
		return VaultReasonPathInvalid
	case errors.Is(err, ErrVaultPathSymlink):
		return VaultReasonPathSymlink
	case errors.Is(err, ErrVaultContentUnsafe):
		return VaultReasonContentUnsafe
	default:
		return ""
	}
}

func queryUnescapeVaultContent(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}
