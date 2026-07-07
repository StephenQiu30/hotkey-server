package pkg

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
)

// Int64Array is a PostgreSQL bigint[] array wrapper.
type Int64Array []int64

// Scan implements sql.Scanner for PostgreSQL bigint[].
// Handles both "{1,2,3}" and JSON array "[1,2,3]" formats.
func (a *Int64Array) Scan(value any) error {
	if value == nil {
		*a = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("int64array: expected []byte, got %T", value)
	}
	s := string(bytes)
	if len(s) > 0 && s[0] == '[' {
		return json.Unmarshal(bytes, (*[]int64)(a))
	}
	s = strings.Trim(s, "{}")
	if s == "" {
		*a = nil
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
	*a = result
	return nil
}

// Value implements driver.Valuer for PostgreSQL bigint[].
func (a Int64Array) Value() (driver.Value, error) {
	if a == nil {
		return []byte("{}"), nil
	}
	parts := make([]string, len(a))
	for i, v := range a {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return []byte("{" + strings.Join(parts, ",") + "}"), nil
}
