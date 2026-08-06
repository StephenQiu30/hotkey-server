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
