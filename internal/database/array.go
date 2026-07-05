package database

import (
	"encoding/json"
	"fmt"
	"strings"
)

// toInt64Array converts a Go []int64 to PostgreSQL bigint[] format ([]byte).
func toInt64Array(values []int64) []byte {
	if len(values) == 0 {
		return []byte("{}")
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return []byte("{" + strings.Join(parts, ",") + "}")
}

// fromInt64Array parses PostgreSQL bigint[] format ([]byte) into Go []int64.
// Handles both "{1,2,3}" and JSON array "[1,2,3]" formats.
func fromInt64Array(data []byte) []int64 {
	if data == nil || len(data) == 0 {
		return nil
	}
	s := string(data)

	// Try JSON array first
	if len(s) > 0 && s[0] == '[' {
		var result []int64
		if err := json.Unmarshal(data, &result); err == nil {
			return result
		}
		return nil
	}

	// PostgreSQL array format: {1,2,3}
	s = strings.Trim(s, "{}")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]int64, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var v int64
		if _, err := fmt.Sscanf(p, "%d", &v); err == nil {
			result[i] = v
		}
	}
	return result
}
