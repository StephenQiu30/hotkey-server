// Package domain defines deterministic Alert facts and state rules.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	PolicyVersionV1              = "alert-policy-v1"
	PolicyVersionV2              = "alert-policy-v2"
	OccurrenceFingerprintVersion = "alert-occurrence-v1"
	CooldownV1                   = time.Hour
	MaxReasonCodeLength          = 64
)

type TriggerType string

const (
	TriggerNewEvent         TriggerType = "new_event"
	TriggerRising           TriggerType = "rising"
	TriggerReactivated      TriggerType = "reactivated"
	TriggerThresholdCrossed TriggerType = "threshold_crossed"
)

func (trigger TriggerType) Valid() bool {
	switch trigger {
	case TriggerNewEvent, TriggerRising, TriggerReactivated, TriggerThresholdCrossed:
		return true
	default:
		return false
	}
}

func TriggerTypeForEventUpdate(kind string) (TriggerType, bool) {
	switch kind {
	case "event_created":
		return TriggerNewEvent, true
	case "rising":
		return TriggerRising, true
	case "reactivated":
		return TriggerReactivated, true
	case "source_expansion", "metric_changed":
		return TriggerThresholdCrossed, true
	default:
		return "", false
	}
}

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

func (severity Severity) Valid() bool {
	return severity == SeverityInfo || severity == SeverityWarning || severity == SeverityCritical
}

func SeverityForScore(score float64) (Severity, error) {
	return SeverityForThresholds(score, 75, 90)
}

func SeverityForThresholds(score, warningThreshold, criticalThreshold float64) (Severity, error) {
	if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 100 {
		return "", fmt.Errorf("score must be finite and between 0 and 100")
	}
	if math.IsNaN(warningThreshold) || math.IsInf(warningThreshold, 0) || math.IsNaN(criticalThreshold) || math.IsInf(criticalThreshold, 0) || warningThreshold < 0 || criticalThreshold > 100 || warningThreshold > criticalThreshold {
		return "", fmt.Errorf("severity thresholds must be ordered from 0 to 100")
	}
	if score >= criticalThreshold {
		return SeverityCritical, nil
	}
	if score >= warningThreshold {
		return SeverityWarning, nil
	}
	return SeverityInfo, nil
}

type State string

const (
	StateOpen         State = "open"
	StateAcknowledged State = "acknowledged"
	StateResolved     State = "resolved"
	StateSuppressed   State = "suppressed"
)

func (state State) Valid() bool {
	return state == StateOpen || state == StateAcknowledged || state == StateResolved || state == StateSuppressed
}

func CanUserTransition(from, to State) bool {
	switch from {
	case StateOpen:
		return to == StateAcknowledged || to == StateResolved || to == StateSuppressed
	case StateAcknowledged:
		return to == StateResolved || to == StateSuppressed
	default:
		return false
	}
}

func CooldownUntil(triggeredAt time.Time) time.Time {
	return CooldownUntilMinutes(triggeredAt, 60)
}

func CooldownUntilMinutes(triggeredAt time.Time, minutes int) time.Time {
	if triggeredAt.IsZero() || minutes < 5 || minutes > 1440 {
		return time.Time{}
	}
	return triggeredAt.UTC().Add(time.Duration(minutes) * time.Minute)
}

func ShouldReopen(state State, cooldownUntil, triggeredAt time.Time) bool {
	if state != StateAcknowledged && state != StateResolved || cooldownUntil.IsZero() || triggeredAt.IsZero() {
		return false
	}
	return !triggeredAt.UTC().Before(cooldownUntil.UTC())
}

type FingerprintInput struct {
	MonitorConfigVersionID int64
	EventUpdateID          int64
	TriggerType            TriggerType
	PolicyVersion          string
}

func OccurrenceFingerprint(input FingerprintInput) (string, error) {
	if input.MonitorConfigVersionID <= 0 || input.EventUpdateID <= 0 || !input.TriggerType.Valid() || strings.TrimSpace(input.PolicyVersion) == "" {
		return "", fmt.Errorf("invalid occurrence fingerprint input")
	}
	payload := strings.Join([]string{
		OccurrenceFingerprintVersion,
		strconv.FormatInt(input.MonitorConfigVersionID, 10),
		strconv.FormatInt(input.EventUpdateID, 10),
		string(input.TriggerType),
		input.PolicyVersion,
	}, "\n")
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:]), nil
}

type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorSystem ActorType = "system"
)

type Thread struct {
	ID                             int64
	Version                        int64
	MonitorID                      int64
	EventID                        int64
	TriggerType                    TriggerType
	PolicyVersion                  string
	MonitorConfigVersionID         int64
	MonitorRevision                int64
	MonitorConfigHash              string
	EventThresholdSnapshot         float64
	AlertMinHeatSnapshot           float64
	AlertMinMomentumSnapshot       float64
	AlertMinBreadthSnapshot        float64
	AlertWarningThresholdSnapshot  float64
	AlertCriticalThresholdSnapshot float64
	AlertCooldownMinutesSnapshot   int
	State                          State
	Severity                       Severity
	TitleSnapshot                  string
	ReasonSnapshot                 string
	FirstTriggeredAt               time.Time
	LastTriggeredAt                time.Time
	OccurrenceCount                int64
	CooldownUntil                  time.Time
	AcknowledgedAt                 *time.Time
	AcknowledgedByUserID           *int64
	ResolvedAt                     *time.Time
	ResolvedByUserID               *int64
	SuppressedAt                   *time.Time
	SuppressedByUserID             *int64
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

type Occurrence struct {
	ID                     int64
	ThreadID               int64
	EventUpdateID          int64
	Severity               Severity
	FinalScoreSnapshot     float64
	HeatScoreSnapshot      float64
	MomentumScoreSnapshot  float64
	BreadthScoreSnapshot   float64
	EventThresholdSnapshot float64
	ReasonCodes            []string
	Fingerprint            string
	TriggeredAt            time.Time
	CreatedAt              time.Time
}

type StateAudit struct {
	ID              int64
	ThreadID        int64
	ActorType       ActorType
	ActorUserID     *int64
	FromState       State
	ToState         State
	ExpectedVersion int64
	ReasonCode      string
	CreatedAt       time.Time
}

type RecordOccurrenceCommand struct {
	MonitorID                      int64
	EventID                        int64
	EventUpdateID                  int64
	TriggerType                    TriggerType
	PolicyVersion                  string
	MonitorConfigVersionID         int64
	MonitorRevision                int64
	MonitorConfigHash              string
	EventThresholdSnapshot         float64
	AlertMinHeatSnapshot           float64
	AlertMinMomentumSnapshot       float64
	AlertMinBreadthSnapshot        float64
	AlertWarningThresholdSnapshot  float64
	AlertCriticalThresholdSnapshot float64
	AlertCooldownMinutesSnapshot   int
	FinalScoreSnapshot             float64
	HeatScoreSnapshot              float64
	MomentumScoreSnapshot          float64
	BreadthScoreSnapshot           float64
	Severity                       Severity
	TitleSnapshot                  string
	ReasonSnapshot                 string
	ReasonCodes                    []string
	TriggeredAt                    time.Time
	Fingerprint                    string
}

func (command RecordOccurrenceCommand) Validate() error {
	if command.MonitorID <= 0 || command.EventID <= 0 || command.EventUpdateID <= 0 || command.MonitorConfigVersionID <= 0 || command.MonitorRevision <= 0 {
		return fmt.Errorf("positive alert identities are required")
	}
	if !command.TriggerType.Valid() || command.PolicyVersion != PolicyVersionV1 && command.PolicyVersion != PolicyVersionV2 || !command.Severity.Valid() {
		return fmt.Errorf("invalid alert policy facts")
	}
	if !validHash(command.MonitorConfigHash) || !validHash(command.Fingerprint) {
		return fmt.Errorf("invalid alert hashes")
	}
	warning, critical := float64(75), float64(90)
	if command.PolicyVersion == PolicyVersionV2 {
		warning, critical = command.AlertWarningThresholdSnapshot, command.AlertCriticalThresholdSnapshot
		if command.AlertCooldownMinutesSnapshot < 5 || command.AlertCooldownMinutesSnapshot > 1440 || !validScore(command.AlertMinHeatSnapshot) || !validScore(command.AlertMinMomentumSnapshot) || !validScore(command.AlertMinBreadthSnapshot) || command.HeatScoreSnapshot < command.AlertMinHeatSnapshot || command.MomentumScoreSnapshot < command.AlertMinMomentumSnapshot || command.BreadthScoreSnapshot < command.AlertMinBreadthSnapshot {
			return fmt.Errorf("invalid alert dimension snapshot")
		}
	}
	wantSeverity, scoreErr := SeverityForThresholds(command.FinalScoreSnapshot, warning, critical)
	_, thresholdErr := SeverityForScore(command.EventThresholdSnapshot)
	if scoreErr != nil || thresholdErr != nil || command.FinalScoreSnapshot < command.EventThresholdSnapshot || command.Severity != wantSeverity {
		return fmt.Errorf("invalid alert score snapshot")
	}
	if strings.TrimSpace(command.TitleSnapshot) == "" || command.TriggeredAt.IsZero() {
		return fmt.Errorf("alert title snapshot and trigger time are required")
	}
	want, err := OccurrenceFingerprint(FingerprintInput{MonitorConfigVersionID: command.MonitorConfigVersionID, EventUpdateID: command.EventUpdateID, TriggerType: command.TriggerType, PolicyVersion: command.PolicyVersion})
	if err != nil || want != command.Fingerprint {
		return fmt.Errorf("occurrence fingerprint does not match frozen facts")
	}
	return nil
}

type RecordOccurrenceResult struct {
	Thread     Thread
	Occurrence Occurrence
	Created    bool
	Reopened   bool
	Disturb    bool
}

type TransitionCommand struct {
	ThreadID        int64
	ExpectedVersion int64
	To              State
	ActorUserID     int64
	ReasonCode      string
	At              time.Time
}

func (command TransitionCommand) Validate() error {
	reason := strings.TrimSpace(command.ReasonCode)
	if command.ThreadID <= 0 || command.ExpectedVersion <= 0 || command.ActorUserID <= 0 || !command.To.Valid() || command.To == StateOpen || command.At.IsZero() || reason == "" || len(reason) > MaxReasonCodeLength {
		return fmt.Errorf("invalid alert transition")
	}
	return nil
}

type ListQuery struct {
	State     *State
	Severity  *Severity
	MonitorID *int64
	Cursor    string
	Limit     int
}

func (query ListQuery) Validate() error {
	if query.Limit < 1 || query.Limit > 100 {
		return fmt.Errorf("limit must be between 1 and 100")
	}
	if query.State != nil && !query.State.Valid() || query.Severity != nil && !query.Severity.Valid() || query.MonitorID != nil && *query.MonitorID <= 0 {
		return fmt.Errorf("invalid alert filters")
	}
	return nil
}

type ThreadPage struct {
	Items      []Thread
	NextCursor string
}

type ThreadDetail struct {
	Thread          Thread
	Occurrences     []Occurrence
	Audits          []StateAudit
	EmailDeliveries []EmailDelivery
}

type EmailDelivery struct {
	ID            int64
	OccurrenceID  int64
	Severity      Severity
	Status        string
	AttemptCount  int
	NextAttemptAt *time.Time
	SucceededAt   *time.Time
	LastError     string
}

func validHash(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func validScore(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 100
}
