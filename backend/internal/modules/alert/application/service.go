// Package application coordinates Alert evaluation, reads and state actions
// through narrow Event, Monitor and Alert-owned ports.
package application

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/alert/domain"
	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	sharedclock "github.com/StephenQiu30/hotkey-server/backend/internal/shared/clock"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type EventUpdateRef = eventapplication.AlertUpdateRef
type EventAlertCandidate = eventapplication.AlertCandidate
type EventCandidateReader = eventapplication.AlertCandidateReader
type PublishedAlertPolicy = monitorapplication.PublishedAlertPolicy
type MonitorPolicyReader = monitorapplication.AlertPolicyReader

type OccurrenceWriter interface {
	RecordOccurrence(context.Context, domain.RecordOccurrenceCommand) (domain.RecordOccurrenceResult, error)
}

type alertReader interface {
	List(context.Context, domain.ListQuery) (domain.ThreadPage, error)
	Get(context.Context, int64) (domain.ThreadDetail, error)
}

type stateWriter interface {
	Transition(context.Context, domain.TransitionCommand) (domain.Thread, error)
}

type TransactionRunner interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

type AlertEmailPlan struct {
	OccurrenceID   int64
	IdempotencyKey string
	Recipient      string
	Severity       domain.Severity
	Title          string
	Reason         string
}

type AlertEmailPlanner interface {
	PlanAlertEmail(context.Context, AlertEmailPlan) error
}

type Dependencies struct {
	Candidates   EventCandidateReader
	Policies     MonitorPolicyReader
	Occurrences  OccurrenceWriter
	Clock        sharedclock.Clock
	Transactions TransactionRunner
	Emails       AlertEmailPlanner
}

type Service struct {
	candidates   EventCandidateReader
	policies     MonitorPolicyReader
	occurrences  OccurrenceWriter
	clock        sharedclock.Clock
	transactions TransactionRunner
	emails       AlertEmailPlanner
}

func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Candidates == nil || dependencies.Policies == nil || dependencies.Occurrences == nil {
		return nil, errors.New("alert application dependencies are required")
	}
	if dependencies.Clock == nil {
		dependencies.Clock = sharedclock.System{}
	}
	return &Service{candidates: dependencies.Candidates, policies: dependencies.Policies, occurrences: dependencies.Occurrences, clock: dependencies.Clock, transactions: dependencies.Transactions, emails: dependencies.Emails}, nil
}

type EvaluationResult struct {
	CandidateCount int
	EligibleCount  int
	CreatedCount   int
	DuplicateCount int
	ReopenedCount  int
}

func (service *Service) Evaluate(ctx context.Context, ref EventUpdateRef) (EvaluationResult, error) {
	if service == nil || service.candidates == nil || service.policies == nil || service.occurrences == nil {
		return EvaluationResult{}, sharedrepository.ErrUnavailable
	}
	if err := ref.Validate(); err != nil {
		return EvaluationResult{}, err
	}
	candidates, err := service.candidates.ListAlertCandidates(ctx, ref)
	if err != nil {
		return EvaluationResult{}, err
	}
	result := EvaluationResult{CandidateCount: len(candidates)}
	monitorIDs := make([]int64, 0, len(candidates))
	seen := make(map[int64]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, actionable := domain.TriggerTypeForEventUpdate(candidate.UpdateKind); !actionable {
			continue
		}
		if candidate.MonitorID <= 0 || candidate.EventID <= 0 {
			return EvaluationResult{}, fmt.Errorf("%w: invalid event alert candidate", sharedrepository.ErrInvalidInput)
		}
		if _, exists := seen[candidate.MonitorID]; !exists {
			seen[candidate.MonitorID] = struct{}{}
			monitorIDs = append(monitorIDs, candidate.MonitorID)
		}
	}
	if len(monitorIDs) == 0 {
		return result, nil
	}
	policies, err := service.policies.ListPublishedAlertPolicies(ctx, monitorIDs)
	if err != nil {
		return EvaluationResult{}, err
	}
	byMonitor := make(map[int64]PublishedAlertPolicy, len(policies))
	for _, policy := range policies {
		policy = normalizePolicy(policy)
		if policy.MonitorID <= 0 || policy.ConfigVersionID <= 0 || policy.Revision <= 0 || !validSHA256(policy.ConfigHash) || !validPolicy(policy) {
			return EvaluationResult{}, fmt.Errorf("%w: invalid published alert policy", sharedrepository.ErrInvalidInput)
		}
		if _, duplicate := byMonitor[policy.MonitorID]; duplicate {
			return EvaluationResult{}, fmt.Errorf("%w: duplicate published alert policy", sharedrepository.ErrConflict)
		}
		byMonitor[policy.MonitorID] = policy
	}
	for _, candidate := range candidates {
		trigger, actionable := domain.TriggerTypeForEventUpdate(candidate.UpdateKind)
		if !actionable {
			continue
		}
		policy, found := byMonitor[candidate.MonitorID]
		if !found {
			continue
		}
		severity, err := domain.SeverityForThresholds(candidate.FinalScore, policy.AlertWarningThreshold, policy.AlertCriticalThreshold)
		if err != nil || candidate.TriggeredAt.IsZero() {
			return EvaluationResult{}, fmt.Errorf("%w: invalid event alert score or time", sharedrepository.ErrInvalidInput)
		}
		if candidate.FinalScore < policy.EventThreshold || candidate.HeatScore < policy.AlertMinHeat || candidate.MomentumScore < policy.AlertMinMomentum || candidate.BreadthScore < policy.AlertMinBreadth {
			continue
		}
		result.EligibleCount++
		fingerprint, err := domain.OccurrenceFingerprint(domain.FingerprintInput{MonitorConfigVersionID: policy.ConfigVersionID, EventUpdateID: ref.ID, TriggerType: trigger, PolicyVersion: domain.PolicyVersionV2})
		if err != nil {
			return EvaluationResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
		}
		command := domain.RecordOccurrenceCommand{
			MonitorID: candidate.MonitorID, EventID: candidate.EventID, EventUpdateID: ref.ID,
			TriggerType: trigger, PolicyVersion: domain.PolicyVersionV2,
			MonitorConfigVersionID: policy.ConfigVersionID, MonitorRevision: policy.Revision, MonitorConfigHash: policy.ConfigHash,
			EventThresholdSnapshot: policy.EventThreshold, AlertMinHeatSnapshot: policy.AlertMinHeat, AlertMinMomentumSnapshot: policy.AlertMinMomentum, AlertMinBreadthSnapshot: policy.AlertMinBreadth,
			AlertWarningThresholdSnapshot: policy.AlertWarningThreshold, AlertCriticalThresholdSnapshot: policy.AlertCriticalThreshold, AlertCooldownMinutesSnapshot: policy.AlertCooldownMinutes,
			FinalScoreSnapshot: candidate.FinalScore, HeatScoreSnapshot: candidate.HeatScore, MomentumScoreSnapshot: candidate.MomentumScore, BreadthScoreSnapshot: candidate.BreadthScore, Severity: severity,
			TitleSnapshot: candidate.TitleSnapshot, ReasonSnapshot: candidate.ReasonSnapshot, ReasonCodes: append([]string(nil), candidate.ReasonCodes...),
			TriggeredAt: candidate.TriggeredAt.UTC(), Fingerprint: fingerprint,
		}
		var recorded domain.RecordOccurrenceResult
		record := func(writeContext context.Context) error {
			var writeErr error
			recorded, writeErr = service.occurrences.RecordOccurrence(writeContext, command)
			if writeErr != nil {
				return writeErr
			}
			if recorded.Created && recorded.Disturb && shouldEmail(policy, recorded.Thread.Severity) {
				if service.emails == nil {
					return sharedrepository.ErrUnavailable
				}
				return service.emails.PlanAlertEmail(writeContext, AlertEmailPlan{OccurrenceID: recorded.Occurrence.ID, IdempotencyKey: recorded.Occurrence.Fingerprint, Recipient: policy.RecipientEmail, Severity: recorded.Thread.Severity, Title: recorded.Thread.TitleSnapshot, Reason: recorded.Thread.ReasonSnapshot})
			}
			return nil
		}
		if policy.AlertEmailEnabled {
			if service.transactions == nil {
				return result, sharedrepository.ErrUnavailable
			}
			err = service.transactions.WithinTransaction(ctx, record)
		} else {
			err = record(ctx)
		}
		if err != nil {
			return result, err
		}
		if recorded.Created {
			result.CreatedCount++
		} else {
			result.DuplicateCount++
		}
		if recorded.Reopened {
			result.ReopenedCount++
		}
	}
	return result, nil
}

func validPolicy(policy PublishedAlertPolicy) bool {
	if policy.EventThreshold < 0 || policy.EventThreshold > 100 || policy.AlertMinHeat < 0 || policy.AlertMinHeat > 100 || policy.AlertMinMomentum < 0 || policy.AlertMinMomentum > 100 || policy.AlertMinBreadth < 0 || policy.AlertMinBreadth > 100 || policy.AlertWarningThreshold < 0 || policy.AlertWarningThreshold > policy.AlertCriticalThreshold || policy.AlertCriticalThreshold > 100 || policy.AlertCooldownMinutes < 5 || policy.AlertCooldownMinutes > 1440 {
		return false
	}
	if policy.AlertEmailMinSeverity != "warning" && policy.AlertEmailMinSeverity != "critical" {
		return false
	}
	return !policy.AlertEmailEnabled || strings.TrimSpace(policy.RecipientEmail) != ""
}

func normalizePolicy(policy PublishedAlertPolicy) PublishedAlertPolicy {
	if policy.AlertCooldownMinutes == 0 && policy.AlertEmailMinSeverity == "" && policy.AlertWarningThreshold == 0 && policy.AlertCriticalThreshold == 0 && policy.AlertMinHeat == 0 && policy.AlertMinMomentum == 0 && policy.AlertMinBreadth == 0 {
		policy.AlertMinHeat, policy.AlertMinMomentum, policy.AlertMinBreadth = 70, 55, 25
		policy.AlertWarningThreshold, policy.AlertCriticalThreshold, policy.AlertCooldownMinutes = 75, 90, 60
		policy.AlertEmailMinSeverity = "critical"
	}
	return policy
}

func shouldEmail(policy PublishedAlertPolicy, severity domain.Severity) bool {
	if !policy.AlertEmailEnabled || strings.TrimSpace(policy.RecipientEmail) == "" || severity == domain.SeverityInfo {
		return false
	}
	return policy.AlertEmailMinSeverity == "warning" || severity == domain.SeverityCritical
}

func (service *Service) List(ctx context.Context, query domain.ListQuery) (domain.ThreadPage, error) {
	reader, ok := serviceReader(service)
	if !ok {
		return domain.ThreadPage{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return domain.ThreadPage{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return reader.List(ctx, query)
}

func (service *Service) Get(ctx context.Context, threadID int64) (domain.ThreadDetail, error) {
	reader, ok := serviceReader(service)
	if !ok {
		return domain.ThreadDetail{}, sharedrepository.ErrUnavailable
	}
	if threadID <= 0 {
		return domain.ThreadDetail{}, fmt.Errorf("%w: alert thread id is required", sharedrepository.ErrInvalidInput)
	}
	return reader.Get(ctx, threadID)
}

type ActionInput struct {
	Subject         identitydomain.Subject
	ThreadID        int64
	ExpectedVersion int64
	ReasonCode      string
}

func (service *Service) Acknowledge(ctx context.Context, input ActionInput) (domain.Thread, error) {
	return service.transition(ctx, input, domain.StateAcknowledged, false)
}

func (service *Service) Resolve(ctx context.Context, input ActionInput) (domain.Thread, error) {
	return service.transition(ctx, input, domain.StateResolved, false)
}

func (service *Service) Suppress(ctx context.Context, input ActionInput) (domain.Thread, error) {
	return service.transition(ctx, input, domain.StateSuppressed, true)
}

func (service *Service) transition(ctx context.Context, input ActionInput, to domain.State, elevated bool) (domain.Thread, error) {
	if service == nil || service.clock == nil || !input.Subject.Authenticated() {
		return domain.Thread{}, fmt.Errorf("%w: invalid alert actor", sharedrepository.ErrInvalidInput)
	}
	if elevated && input.Subject.Role == identitydomain.RoleViewer {
		return domain.Thread{}, sharederrors.New(sharederrors.CodeForbidden, http.StatusForbidden, "")
	}
	writer, ok := service.occurrences.(stateWriter)
	if !ok {
		return domain.Thread{}, sharedrepository.ErrUnavailable
	}
	command := domain.TransitionCommand{ThreadID: input.ThreadID, ExpectedVersion: input.ExpectedVersion, To: to, ActorUserID: input.Subject.UserID, ReasonCode: input.ReasonCode, At: service.clock.Now().UTC()}
	if err := command.Validate(); err != nil {
		return domain.Thread{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return writer.Transition(ctx, command)
}

func serviceReader(service *Service) (alertReader, bool) {
	if service == nil || service.occurrences == nil {
		return nil, false
	}
	reader, ok := service.occurrences.(alertReader)
	return reader, ok
}

func validSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
