package domain

import "errors"

type CollectionErrorKind string

const (
	CollectionErrorAuthentication CollectionErrorKind = "authentication"
	CollectionErrorRateLimited    CollectionErrorKind = "rate_limited"
	CollectionErrorTemporary      CollectionErrorKind = "temporary"
	CollectionErrorParse          CollectionErrorKind = "parse"
	CollectionErrorPermanent      CollectionErrorKind = "permanent"
)

func (kind CollectionErrorKind) Valid() bool {
	switch kind {
	case CollectionErrorAuthentication, CollectionErrorRateLimited, CollectionErrorTemporary, CollectionErrorParse, CollectionErrorPermanent:
		return true
	default:
		return false
	}
}

type collectionError struct {
	kind  CollectionErrorKind
	cause error
}

func (err *collectionError) Error() string { return "collection " + string(err.kind) + " error" }
func (err *collectionError) Unwrap() error { return err.cause }

func NewCollectionError(kind CollectionErrorKind, cause error) error {
	if !kind.Valid() {
		kind = CollectionErrorPermanent
	}
	return &collectionError{kind: kind, cause: cause}
}

func ClassifyCollectionError(err error) CollectionErrorKind {
	var collection *collectionError
	if errors.As(err, &collection) {
		return collection.kind
	}
	return CollectionErrorPermanent
}

func IsCollectionRetryable(err error) bool {
	switch ClassifyCollectionError(err) {
	case CollectionErrorRateLimited, CollectionErrorTemporary:
		return true
	default:
		return false
	}
}

// SafeCollectionErrorCause returns a sanitized reason for operational
// diagnostics. Provider responses, query text and credentials are never
// included; only stable connector/validation diagnostics are surfaced.
// Collection wrappers are unwrapped until the innermost diagnostic is reached.
func SafeCollectionErrorCause(cause error) string {
	if cause == nil {
		return ""
	}
	innermost := cause
	for {
		var collection *collectionError
		if errors.As(innermost, &collection) && collection.cause != nil {
			innermost = collection.cause
			continue
		}
		break
	}
	// allowlist of stable, connector/validation-provided diagnostics that do
	// not carry third-party content, provider responses or query text
	switch innermost.Error() {
	case "invalid RSS source connection",
		"RSS connector requires an RSS source connection",
		"invalid RSS endpoint",
		"invalid RSS resource limit profile",
		"unsafe RSS destination",
		"RSS redirect limit exceeded",
		"invalid RSS request cursor",
		"RSS destination is not permitted",
		"RSS response exceeds body byte limit",
		"RSS response exceeds collection item limit",
		"RSS daily request quota exceeded",
		"read RSS response",
		"parse RSS response",
		"invalid RSS pagination link",
		"Hacker News destination is not permitted",
		"X destination is not permitted",
		"Foundry destination is not permitted",
		"source connection is unavailable",
		"resolved collection identity changed":
		return innermost.Error()
	default:
		// Unknown errors may carry query text, HTTP status details or other
		// derived content; only surface a generic hint.
		return "collection failed (see error_kind)"
	}
}
