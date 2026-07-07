package pkg

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONB is a generic type for PostgreSQL JSONB columns.
type JSONB[T any] struct {
	Data T
}

func (j *JSONB[T]) Scan(value any) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("jsonb: expected []byte, got %T", value)
	}
	return json.Unmarshal(bytes, &j.Data)
}

func (j JSONB[T]) Value() (driver.Value, error) {
	return json.Marshal(j.Data)
}
