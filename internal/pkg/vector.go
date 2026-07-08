// Package pkg provides shared utility types for the hotkey-server.
package pkg

import (
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"math"
)

// Vector384 is a 384-dimensional float32 vector for pgvector.
type Vector384 [384]float32

// Scan implements sql.Scanner for pgvector binary format.
func (v *Vector384) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	var data []byte
	switch s := src.(type) {
	case []byte:
		data = s
	case string:
		data = []byte(s)
	default:
		return fmt.Errorf("unexpected vector type: %T", src)
	}
	if len(data) < 4 {
		return fmt.Errorf("vector data too short: %d bytes", len(data))
	}
	dim := binary.LittleEndian.Uint32(data[:4])
	if dim != 384 {
		return fmt.Errorf("expected dim 384, got %d", dim)
	}
	if len(data) < 4+384*4 {
		return fmt.Errorf("vector data too short for dim %d: %d bytes", dim, len(data))
	}
	for i := range 384 {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[4+i*4:]))
	}
	return nil
}

// Value implements driver.Valuer for pgvector binary format.
func (v Vector384) Value() (driver.Value, error) {
	buf := make([]byte, 4+384*4)
	binary.LittleEndian.PutUint32(buf[:4], 384)
	for i := range 384 {
		binary.LittleEndian.PutUint32(buf[4+i*4:], math.Float32bits(v[i]))
	}
	return buf, nil
}

// Dim returns the vector dimension (always 384).
func (v Vector384) Dim() int { return 384 }
