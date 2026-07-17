package domain

import (
	"errors"
	"testing"
)

func TestCollectionErrorsKeepRetryClassificationWithoutTransportDetails(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		kind      CollectionErrorKind
		retryable bool
	}{
		{CollectionErrorAuthentication, false},
		{CollectionErrorRateLimited, true},
		{CollectionErrorTemporary, true},
		{CollectionErrorParse, false},
		{CollectionErrorPermanent, false},
	} {
		err := NewCollectionError(test.kind, errors.New("upstream detail"))
		if got := ClassifyCollectionError(err); got != test.kind {
			t.Errorf("ClassifyCollectionError(%v) = %q, want %q", err, got, test.kind)
		}
		if got := IsCollectionRetryable(err); got != test.retryable {
			t.Errorf("IsCollectionRetryable(%q) = %v, want %v", test.kind, got, test.retryable)
		}
	}
}
