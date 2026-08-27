// Package pagination provides signed-shape cursor encoding. Cursors bind to
// the selected sort and filter fingerprint so clients cannot reuse a cursor
// with a different query shape.
package pagination

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

const (
	cursorVersion            = 2
	maximumCursorTTL         = 24 * time.Hour
	maximumEncodedCursorSize = 2048
	maximumFilterSize        = 256
	cursorSigningContext     = "hotkey-pagination-cursor-v2"
	// DefaultTTL bounds ordinary P0 list traversals without making cursors durable credentials.
	DefaultTTL = 15 * time.Minute
)

var (
	ErrInvalidCursor  = errors.New("invalid cursor")
	ErrStaleCursor    = errors.New("cursor does not match query")
	ErrExpiredCursor  = errors.New("cursor expired")
	cursorSortPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

type Cursor struct {
	Version           int       `json:"v"`
	Sort              string    `json:"s"`
	Descending        bool      `json:"d"`
	FilterFingerprint string    `json:"f"`
	ID                int64     `json:"id"`
	IssuedAt          time.Time `json:"iat"`
	ExpiresAt         time.Time `json:"exp"`
}

// Codec signs short-lived list cursors with a purpose-derived HMAC key. The
// filter fingerprint is part of the signed payload and must include every
// authorization scope (for example, the requesting user or tenant) that owns
// the traversal.
type Codec struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

func NewCodec(secret string, ttl time.Duration) (*Codec, error) {
	secret = strings.TrimSpace(secret)
	if len([]byte(secret)) < sha256.Size {
		return nil, fmt.Errorf("cursor signing secret must be at least %d bytes", sha256.Size)
	}
	if ttl <= 0 || ttl > maximumCursorTTL {
		return nil, fmt.Errorf("cursor ttl must be positive and no greater than %s", maximumCursorTTL)
	}
	derivation := hmac.New(sha256.New, []byte(secret))
	_, _ = derivation.Write([]byte(cursorSigningContext))
	return &Codec{key: derivation.Sum(nil), ttl: ttl, now: time.Now}, nil
}

// NewTestCodec gives repository integration tests a deterministic key without
// hard-coding a production secret. Production composition must use NewCodec
// with configured secret material instead.
func NewTestCodec(seed string) *Codec {
	digest := sha256.Sum256([]byte("hotkey-pagination-test:" + seed))
	codec, err := NewCodec(fmt.Sprintf("%x", digest), DefaultTTL)
	if err != nil {
		panic(err)
	}
	return codec
}

func (codec *Codec) Encode(sort string, descending bool, filterFingerprint string, id int64) (string, error) {
	if !validCursorShape(sort, filterFingerprint, id) || codec == nil || len(codec.key) != sha256.Size || codec.ttl <= 0 {
		return "", fmt.Errorf("%w: cursor codec or shape is invalid", ErrInvalidCursor)
	}
	now := codec.currentTime()
	payload, err := json.Marshal(Cursor{
		Version: cursorVersion, Sort: sort, Descending: descending, FilterFingerprint: filterFingerprint, ID: id,
		IssuedAt: now, ExpiresAt: now.Add(codec.ttl),
	})
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	signature := hmac.New(sha256.New, codec.key)
	_, _ = signature.Write([]byte(payloadPart))
	encoded := payloadPart + "." + base64.RawURLEncoding.EncodeToString(signature.Sum(nil))
	if len(encoded) > maximumEncodedCursorSize {
		return "", fmt.Errorf("%w: encoded cursor is too large", ErrInvalidCursor)
	}
	return encoded, nil
}

func (codec *Codec) Decode(encoded, sort string, descending bool, filterFingerprint string) (Cursor, error) {
	if encoded == "" {
		return Cursor{}, nil
	}
	if codec == nil || len(codec.key) != sha256.Size || !validCursorShape(sort, filterFingerprint, 1) || len(encoded) > maximumEncodedCursorSize {
		return Cursor{}, ErrInvalidCursor
	}
	parts := strings.Split(encoded, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Cursor{}, ErrInvalidCursor
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(signature) != sha256.Size {
		return Cursor{}, ErrInvalidCursor
	}
	want := hmac.New(sha256.New, codec.key)
	_, _ = want.Write([]byte(parts[0]))
	if !hmac.Equal(signature, want.Sum(nil)) {
		return Cursor{}, ErrInvalidCursor
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(payload) == 0 || len(payload) > maximumEncodedCursorSize {
		return Cursor{}, ErrInvalidCursor
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var cursor Cursor
	if err := decoder.Decode(&cursor); err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	if err := ensureCursorJSONEOF(decoder); err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	now := codec.currentTime()
	if cursor.Version != cursorVersion || !validCursorShape(cursor.Sort, cursor.FilterFingerprint, cursor.ID) ||
		cursor.IssuedAt.IsZero() || cursor.ExpiresAt.IsZero() || !cursor.ExpiresAt.After(cursor.IssuedAt) ||
		cursor.ExpiresAt.Sub(cursor.IssuedAt) > maximumCursorTTL || cursor.IssuedAt.After(now.Add(time.Minute)) {
		return Cursor{}, ErrInvalidCursor
	}
	if !cursor.ExpiresAt.After(now) {
		return Cursor{}, ErrExpiredCursor
	}
	if cursor.Sort != sort || cursor.Descending != descending || cursor.FilterFingerprint != filterFingerprint {
		return Cursor{}, ErrStaleCursor
	}
	return cursor, nil
}

func (codec *Codec) currentTime() time.Time {
	if codec != nil && codec.now != nil {
		return codec.now().UTC()
	}
	return time.Now().UTC()
}

func validCursorShape(sort, filterFingerprint string, id int64) bool {
	if !cursorSortPattern.MatchString(sort) || id <= 0 || len(filterFingerprint) > maximumFilterSize {
		return false
	}
	for _, character := range filterFingerprint {
		if character < 0x20 || character > 0x7e {
			return false
		}
	}
	return true
}

func ensureCursorJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrInvalidCursor
	}
	return nil
}
