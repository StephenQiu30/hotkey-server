// Package strutil provides shared string and time utility functions
// used across multiple service packages.
package strutil

import (
	"time"
	"unicode/utf8"
)

// TrimRunes truncates a string to the specified rune limit.
// Returns the original string if limit is 0 or the string is within the limit.
func TrimRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

// CloneTime creates a UTC copy of a time pointer.
// Returns nil if the input is nil.
func CloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
