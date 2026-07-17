package queue

import (
	"errors"
	"fmt"
)

// P0 job kinds are deliberately finite. Each handler consumes only this
// package's bounded ID/version envelope and rereads business facts in the DB.
const (
	MaxPayloadBytes   = 4096
	MaxUniqueKeyBytes = 256

	KindCollectSource        = "collect_source"
	KindNormalizeContent     = "normalize_content"
	KindEvaluateRelevance    = "evaluate_relevance"
	KindClusterContent       = "cluster_content"
	KindRecomputeEventHeat   = "recompute_event_heat"
	KindGenerateEventSummary = "generate_event_summary"
	KindBuildReport          = "build_report"
	KindDeliverEmail         = "deliver_email"
	KindProjectKnowledge     = "project_knowledge"
	KindReconcileKnowledge   = "reconcile_knowledge"
	KindRunRetention         = "run_retention"
)

func IsKnownKind(kind string) bool {
	switch kind {
	case KindCollectSource, KindNormalizeContent, KindEvaluateRelevance, KindClusterContent,
		KindRecomputeEventHeat, KindGenerateEventSummary, KindBuildReport, KindDeliverEmail,
		KindProjectKnowledge, KindReconcileKnowledge, KindRunRetention:
		return true
	default:
		return false
	}
}

var (
	ErrRetryable = errors.New("retryable job failure")
	ErrPermanent = errors.New("permanent job failure")
	ErrCancelled = errors.New("cancelled job")
)

type classifiedError struct {
	kind  error
	cause error
}

func (err *classifiedError) Error() string {
	if err == nil || err.cause == nil {
		return ""
	}
	return fmt.Sprintf("%s: %v", err.kind, err.cause)
}

func (err *classifiedError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *classifiedError) Is(target error) bool {
	if err == nil {
		return false
	}
	return target == err.kind || errors.Is(err.cause, target)
}

func NewRetryableError(cause error) error { return newClassifiedError(ErrRetryable, cause) }
func NewPermanentError(cause error) error { return newClassifiedError(ErrPermanent, cause) }
func NewCancelledError(cause error) error { return newClassifiedError(ErrCancelled, cause) }

func newClassifiedError(kind, cause error) error {
	if cause == nil {
		return nil
	}
	return &classifiedError{kind: kind, cause: cause}
}

func IsRetryable(err error) bool { return errors.Is(err, ErrRetryable) }
func IsPermanent(err error) bool { return errors.Is(err, ErrPermanent) }
func IsCancelled(err error) bool { return errors.Is(err, ErrCancelled) }
