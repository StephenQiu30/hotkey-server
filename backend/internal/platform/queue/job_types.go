package queue

import (
	"errors"
	"fmt"
	"time"
)

// P0 job kinds are deliberately finite. Each handler consumes either the
// bounded ID/version envelope or one explicitly registered bounded semantic
// object and rereads all body-sized business facts from their owning module.
const (
	MaxPayloadBytes   = 4096
	MaxUniqueKeyBytes = 256

	KindCollectSource                    = "collect_source"
	KindRefreshXMetrics                  = "refresh_x_metrics"
	KindNormalizeContent                 = "normalize_content"
	KindEvaluateRelevance                = "evaluate_relevance"
	KindClusterContent                   = "cluster_content"
	KindRecomputeEventHeat               = "recompute_event_heat"
	KindEvaluateEventAlerts              = "evaluate_event_alerts"
	KindGenerateEventSummary             = "generate_event_summary"
	KindBuildReport                      = "build_report"
	KindDeliverEmail                     = "deliver_email"
	KindDeliverAlertEmail                = "deliver_alert_email"
	KindProjectUserNotification          = "project_user_notification"
	KindProjectKnowledge                 = "project_knowledge"
	KindReconcileKnowledge               = "reconcile_knowledge"
	KindRunRetention                     = "run_retention"
	KindGenerateSourceDocument           = "generate_source_document"
	KindAnalyzeMonitorIntent             = "analyze_monitor_intent"
	KindEvaluatePublishedDocumentMatches = "evaluate_published_document_matches"
	KindBackfillPublishedMonitorMatches  = "backfill_published_monitor_matches"
	KindProjectAcceptedDocumentMatch     = "project_accepted_document_match"
	KindExtractAutomaticClaimEvidence    = "extract_automatic_claim_evidence"
	KindRefreshProductEvent              = "refresh_product_event"
	KindRecomputeAIRun                   = "recompute_ai_run"
)

func IsKnownKind(kind string) bool {
	switch kind {
	case KindCollectSource, KindRefreshXMetrics, KindNormalizeContent, KindEvaluateRelevance, KindClusterContent,
		KindRecomputeEventHeat, KindEvaluateEventAlerts, KindGenerateEventSummary, KindBuildReport, KindDeliverEmail, KindDeliverAlertEmail,
		KindProjectUserNotification,
		KindProjectKnowledge, KindReconcileKnowledge, KindRunRetention, KindGenerateSourceDocument, KindAnalyzeMonitorIntent,
		KindEvaluatePublishedDocumentMatches, KindBackfillPublishedMonitorMatches, KindProjectAcceptedDocumentMatch:
		return true
	case KindExtractAutomaticClaimEvidence, KindRefreshProductEvent, KindRecomputeAIRun:
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
	kind    error
	cause   error
	retryAt *time.Time
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
func NewRetryableErrorAt(cause error, retryAt time.Time) error {
	err := newClassifiedError(ErrRetryable, cause)
	var classified *classifiedError
	_ = errors.As(err, &classified)
	if classified != nil && !retryAt.IsZero() {
		value := retryAt.UTC()
		classified.retryAt = &value
	}
	return err
}
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

func RetryAt(err error) (time.Time, bool) {
	var classified *classifiedError
	if !errors.As(err, &classified) || classified == nil || classified.retryAt == nil {
		return time.Time{}, false
	}
	return classified.retryAt.UTC(), true
}
