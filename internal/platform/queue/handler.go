package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// ValidateHandlerJob keeps every P0 handler on the same small envelope
// contract. A handler must not inspect River metadata or accept arbitrary
// business JSON.
func ValidateHandlerJob(job Job, kind string) error {
	if job.Kind != kind {
		return fmt.Errorf("unexpected job kind %q", job.Kind)
	}
	if err := job.Validate(); err != nil {
		return err
	}
	return nil
}

// ClassifyHandlerError maps repository/application failures to the queue
// lifecycle. Context cancellation is terminal for this attempt; invalid,
// missing or conflicting facts are isolated; all other failures remain
// retryable.
func ClassifyHandlerError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return NewCancelledError(err)
	}
	if errors.Is(err, sharedrepository.ErrInvalidInput) || errors.Is(err, sharedrepository.ErrNotFound) ||
		errors.Is(err, sharedrepository.ErrConstraint) || errors.Is(err, sharedrepository.ErrConflict) {
		return NewPermanentError(err)
	}
	return NewRetryableError(err)
}

func StableJobHash(parts ...string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%q", parts)))
	return hex.EncodeToString(digest[:])
}

func StableJobKey(kind string, id, version int64, inputHash string) string {
	return StableJobHash(kind, fmt.Sprint(id), fmt.Sprint(version), inputHash)
}
