// Package pagination provides signed-shape cursor encoding. Cursors bind to
// the selected sort and filter fingerprint so clients cannot reuse a cursor
// with a different query shape.
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

const cursorVersion = 1

var (
	ErrInvalidCursor = errors.New("invalid cursor")
	ErrStaleCursor   = errors.New("cursor does not match query")
)

type Cursor struct {
	Version           int    `json:"v"`
	Sort              string `json:"s"`
	Descending        bool   `json:"d"`
	FilterFingerprint string `json:"f"`
	ID                int64  `json:"id"`
}

func Encode(sort string, descending bool, filterFingerprint string, id int64) (string, error) {
	if sort == "" || id <= 0 {
		return "", fmt.Errorf("%w: sort and positive id are required", ErrInvalidCursor)
	}
	payload, err := json.Marshal(Cursor{
		Version:           cursorVersion,
		Sort:              sort,
		Descending:        descending,
		FilterFingerprint: filterFingerprint,
		ID:                id,
	})
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func Decode(encoded, sort string, descending bool, filterFingerprint string) (Cursor, error) {
	if encoded == "" {
		return Cursor{}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: decode", ErrInvalidCursor)
	}
	var cursor Cursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return Cursor{}, fmt.Errorf("%w: parse", ErrInvalidCursor)
	}
	if cursor.Version != cursorVersion || cursor.Sort == "" || cursor.ID <= 0 {
		return Cursor{}, fmt.Errorf("%w: malformed payload", ErrInvalidCursor)
	}
	if cursor.Sort != sort || cursor.Descending != descending || cursor.FilterFingerprint != filterFingerprint {
		return Cursor{}, ErrStaleCursor
	}
	return cursor, nil
}
