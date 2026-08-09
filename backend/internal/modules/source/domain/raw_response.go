package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"mime"
	"net/url"
	"strings"
	"time"
)

const maxEvidenceResponseRedirects = 10

// CollectorProfileVersion identifies the complete collector behavior that
// produced evidence bytes. It is part of evidence identity and is never
// inferred from response metadata.
type CollectorProfileVersion struct {
	value string
}

func NewCollectorProfileVersion(value string) (CollectorProfileVersion, error) {
	if value == "" || value != strings.TrimSpace(value) || value != strings.ToLower(value) || len(value) > 64 {
		return CollectorProfileVersion{}, fmt.Errorf("collector profile version is invalid")
	}
	for index, character := range value {
		alphanumeric := (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9')
		if alphanumeric || (index > 0 && (character == '-' || character == '_' || character == '.' || character == ':')) {
			continue
		}
		return CollectorProfileVersion{}, fmt.Errorf("collector profile version is invalid")
	}
	return CollectorProfileVersion{value: value}, nil
}

func (version CollectorProfileVersion) String() string {
	return version.value
}

func (version CollectorProfileVersion) Valid() bool {
	parsed, err := NewCollectorProfileVersion(version.value)
	return err == nil && parsed == version
}

// RawResponseHeaders is the only response-header projection permitted to
// cross the Connector boundary. Its fields are private so callers cannot add
// credentials or non-allowlisted provider headers after construction.
type RawResponseHeaders struct {
	values map[string][]string
}

func NewRawResponseHeaders(values map[string][]string) (RawResponseHeaders, error) {
	return normalizeRawResponseHeaders(values)
}

func (headers RawResponseHeaders) Values() map[string][]string {
	copy := make(map[string][]string, len(headers.values))
	for name, values := range headers.values {
		copy[name] = append([]string(nil), values...)
	}
	return copy
}

// Equal reports equality without exposing the internal header map.
func (headers RawResponseHeaders) Equal(other RawResponseHeaders) bool {
	leftValues, rightValues := headers.Values(), other.Values()
	if len(leftValues) != len(rightValues) {
		return false
	}
	for name, values := range leftValues {
		otherValues, found := rightValues[name]
		if !found || len(values) != len(otherValues) {
			return false
		}
		for index := range values {
			if values[index] != otherValues[index] {
				return false
			}
		}
	}
	return true
}

// EvidenceSnapshot is one immutable provider response captured by a
// Connector. Within a Source endpoint scope, Key depends only on the payload
// digest and CollectorProfileVersion; receipt metadata never changes identity.
type EvidenceSnapshot struct {
	Key                     string
	Payload                 []byte
	CollectorProfileVersion CollectorProfileVersion
	MIMEType                string
	StatusCode              int
	RequestedURL            string
	FinalURL                string
	RedirectChain           []string
	ResponseHeaders         RawResponseHeaders
	CapturedAt              time.Time
	PayloadSHA256           string
}

// NewEvidenceSnapshot validates and defensively copies a captured entity. Key
// and PayloadSHA256 may be empty or may contain matching declared identities;
// mismatched declarations are rejected.
func NewEvidenceSnapshot(candidate EvidenceSnapshot) (EvidenceSnapshot, error) {
	if !candidate.CollectorProfileVersion.Valid() {
		return EvidenceSnapshot{}, fmt.Errorf("collector profile version is required")
	}
	mimeType, err := normalizeSnapshotMIME(candidate.MIMEType)
	if err != nil {
		return EvidenceSnapshot{}, err
	}
	if candidate.StatusCode < 100 || candidate.StatusCode > 599 {
		return EvidenceSnapshot{}, fmt.Errorf("evidence response status is invalid")
	}
	if err := validateSnapshotURL(candidate.RequestedURL); err != nil {
		return EvidenceSnapshot{}, fmt.Errorf("evidence response requested URL is invalid")
	}
	if err := validateSnapshotURL(candidate.FinalURL); err != nil {
		return EvidenceSnapshot{}, fmt.Errorf("evidence response final URL is invalid")
	}
	if len(candidate.RedirectChain) > maxEvidenceResponseRedirects {
		return EvidenceSnapshot{}, fmt.Errorf("evidence response redirect chain is invalid")
	}
	redirects := append([]string(nil), candidate.RedirectChain...)
	for _, redirect := range redirects {
		if err := validateSnapshotURL(redirect); err != nil {
			return EvidenceSnapshot{}, fmt.Errorf("evidence response redirect chain is invalid")
		}
	}
	if candidate.RequestedURL == candidate.FinalURL {
		if len(redirects) != 0 {
			return EvidenceSnapshot{}, fmt.Errorf("evidence response redirect chain is invalid")
		}
	} else if len(redirects) == 0 || redirects[len(redirects)-1] != candidate.FinalURL {
		return EvidenceSnapshot{}, fmt.Errorf("evidence response redirect chain is invalid")
	}
	if candidate.CapturedAt.IsZero() {
		return EvidenceSnapshot{}, fmt.Errorf("evidence response capture time is required")
	}
	if candidate.Payload == nil {
		return EvidenceSnapshot{}, fmt.Errorf("evidence response payload is required")
	}
	responseHeaders, err := normalizeRawResponseHeaders(candidate.ResponseHeaders.Values())
	if err != nil {
		return EvidenceSnapshot{}, err
	}
	if values := responseHeaders.values["Content-Type"]; len(values) == 1 && values[0] != mimeType {
		return EvidenceSnapshot{}, fmt.Errorf("evidence response Content-Type does not match MIME type")
	}

	payload := append([]byte(nil), candidate.Payload...)
	payloadDigest := sha256.Sum256(payload)
	payloadSHA256 := hex.EncodeToString(payloadDigest[:])
	key, err := EvidenceSnapshotIdentity(payloadSHA256, candidate.CollectorProfileVersion)
	if err != nil {
		return EvidenceSnapshot{}, err
	}
	if (candidate.PayloadSHA256 != "" && candidate.PayloadSHA256 != payloadSHA256) || (candidate.Key != "" && candidate.Key != key) {
		return EvidenceSnapshot{}, ErrRawEvidenceConflict
	}
	return EvidenceSnapshot{
		Key: key, Payload: payload, CollectorProfileVersion: candidate.CollectorProfileVersion,
		MIMEType: mimeType, StatusCode: candidate.StatusCode,
		RequestedURL: candidate.RequestedURL, FinalURL: candidate.FinalURL, RedirectChain: redirects,
		ResponseHeaders: responseHeaders, CapturedAt: candidate.CapturedAt.UTC(), PayloadSHA256: payloadSHA256,
	}, nil
}

func (snapshot EvidenceSnapshot) VerifyPayload() bool {
	if !validSHA256(snapshot.PayloadSHA256) || !snapshot.CollectorProfileVersion.Valid() {
		return false
	}
	digest := sha256.Sum256(snapshot.Payload)
	if snapshot.PayloadSHA256 != hex.EncodeToString(digest[:]) {
		return false
	}
	key, err := EvidenceSnapshotIdentity(snapshot.PayloadSHA256, snapshot.CollectorProfileVersion)
	return err == nil && snapshot.Key == key
}

// EvidenceSnapshotIdentity returns the endpoint-local evidence key. Source
// endpoint scope is supplied by repositories and object-key namespaces.
func EvidenceSnapshotIdentity(payloadSHA256 string, profile CollectorProfileVersion) (string, error) {
	if !validSHA256(payloadSHA256) || !profile.Valid() {
		return "", fmt.Errorf("evidence snapshot identity is invalid")
	}
	digest := sha256.New()
	writeFingerprintPart(digest, payloadSHA256)
	writeFingerprintPart(digest, profile.String())
	return hex.EncodeToString(digest.Sum(nil)), nil
}

var rawResponseHeaderAllowlist = map[string]struct{}{
	"Content-Type":  {},
	"ETag":          {},
	"Last-Modified": {},
	"Date":          {},
	"Link":          {},
	"Retry-After":   {},
}

func normalizeRawResponseHeaders(input map[string][]string) (RawResponseHeaders, error) {
	allowed := make(map[string][]string, len(rawResponseHeaderAllowlist))
	for rawName, rawValues := range input {
		name := canonicalRawResponseHeaderName(rawName)
		if _, keep := rawResponseHeaderAllowlist[name]; !keep {
			continue
		}
		if name != "Link" && len(allowed[name])+len(rawValues) > 1 {
			return RawResponseHeaders{}, fmt.Errorf("evidence response allowlisted header is ambiguous")
		}
		if len(allowed[name])+len(rawValues) > 16 {
			return RawResponseHeaders{}, fmt.Errorf("evidence response allowlisted header has too many values")
		}
		for _, rawValue := range rawValues {
			value := strings.TrimSpace(rawValue)
			if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") {
				return RawResponseHeaders{}, fmt.Errorf("evidence response allowlisted header is invalid")
			}
			if name == "Content-Type" {
				normalized, err := normalizeSnapshotMIME(value)
				if err != nil {
					return RawResponseHeaders{}, fmt.Errorf("evidence response allowlisted header is invalid")
				}
				value = normalized
			}
			allowed[name] = append(allowed[name], value)
		}
	}
	return RawResponseHeaders{values: allowed}, nil
}

func canonicalRawResponseHeaderName(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "content-type":
		return "Content-Type"
	case "etag":
		return "ETag"
	case "last-modified":
		return "Last-Modified"
	case "date":
		return "Date"
	case "link":
		return "Link"
	case "retry-after":
		return "Retry-After"
	default:
		return ""
	}
}

func normalizeSnapshotMIME(value string) (string, error) {
	if value != strings.TrimSpace(value) || value == "" || len(value) > 255 || strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("evidence response MIME type is invalid")
	}
	mediaType, parameters, err := mime.ParseMediaType(value)
	if err != nil || !strings.Contains(mediaType, "/") {
		return "", fmt.Errorf("evidence response MIME type is invalid")
	}
	mediaType = strings.ToLower(mediaType)
	if charset, ok := parameters["charset"]; ok {
		parameters["charset"] = strings.ToLower(charset)
	}
	return mime.FormatMediaType(mediaType, parameters), nil
}

func validateSnapshotURL(value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("invalid URL")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("invalid URL")
	}
	return nil
}

func writeFingerprintPart(writer hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write([]byte(value))
}
