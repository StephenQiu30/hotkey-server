// Package obsidian renders topic notes as Obsidian-compatible Markdown
// and provides atomic file writing for sync-directory safety.
package obsidian

import (
	"strings"
	"unicode"
)

// Slugify converts a string into a filesystem-safe slug.
// It lowercases ASCII letters, replaces non-alphanumeric/non-Chinese characters
// with hyphens, collapses consecutive hyphens, and trims leading/trailing hyphens.
func Slugify(s string) string {
	s = strings.ToLower(s)

	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if r == '-' {
			if !prevDash {
				b.WriteByte('-')
			}
			prevDash = true
		} else if isSlugChar(r) {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}

	result := b.String()
	result = strings.Trim(result, "-")
	return result
}

// isSlugChar returns true for characters allowed in a slug:
// ASCII alphanumerics and CJK/unicode letters and digits.
func isSlugChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
