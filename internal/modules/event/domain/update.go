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

const EventUpdateAlgorithmVersionV1 = "metric-update-v1"

type EventUpdateKind string

const (
	EventUpdateEventCreated    EventUpdateKind = "event_created"
	EventUpdateRising          EventUpdateKind = "rising"
	EventUpdateCooling         EventUpdateKind = "cooling"
	EventUpdateReactivated     EventUpdateKind = "reactivated"
	EventUpdateSourceExpansion EventUpdateKind = "source_expansion"
	EventUpdateMetricChanged   EventUpdateKind = "metric_changed"
)

func (kind EventUpdateKind) Valid() bool {
	switch kind {
	case EventUpdateEventCreated, EventUpdateRising, EventUpdateCooling, EventUpdateReactivated, EventUpdateSourceExpansion, EventUpdateMetricChanged:
		return true
	default:
		return false
	}
}

func (kind EventUpdateKind) Actionable() bool { return kind.Valid() && kind != EventUpdateCooling }

func (status TrendStatus) Valid() bool {
	switch status {
	case TrendEmerging, TrendRising, TrendStable, TrendFalling, TrendDormant:
		return true
	default:
		return false
	}
}

// EventUpdateState keeps the public update contract compatible with the
// versioned metric snapshot while repositories persist only its bounded
// metric fields in before_state/after_state.
type EventUpdateState = HeatResult

type EventUpdateCandidate struct {
	EventID         int64
	Kind            EventUpdateKind
	Summary         string
	ObservedAt      time.Time
	ReasonCodes     []string
	BeforeState     *EventUpdateState
	AfterState      EventUpdateState
	EvidenceSetHash string
	IdempotencyKey  string
}

func (candidate EventUpdateCandidate) Validate() error {
	if candidate.EventID <= 0 || !candidate.Kind.Valid() || strings.TrimSpace(candidate.Summary) == "" || candidate.ObservedAt.IsZero() ||
		len(candidate.Summary) > 2000 || len(candidate.ReasonCodes) == 0 || !validHash(candidate.EvidenceSetHash) || !validHash(candidate.IdempotencyKey) {
		return fmt.Errorf("invalid event update candidate")
	}
	if err := validateEventUpdateHeat(candidate.AfterState); err != nil {
		return err
	}
	if candidate.AfterState.EventID != candidate.EventID || !candidate.ObservedAt.Equal(candidate.AfterState.WindowEnd) || candidate.EvidenceSetHash != candidate.AfterState.EvidenceSetHash {
		return fmt.Errorf("invalid event update after state")
	}
	for _, reason := range candidate.ReasonCodes {
		if !ValidReasonCode(reason) {
			return fmt.Errorf("invalid event update reason")
		}
	}
	if candidate.BeforeState != nil {
		if err := validateEventUpdateHeat(*candidate.BeforeState); err != nil {
			return err
		}
		if candidate.BeforeState.EventID != candidate.EventID {
			return fmt.Errorf("invalid event update before state")
		}
		if !candidate.BeforeState.WindowEnd.Before(candidate.AfterState.WindowEnd) {
			return fmt.Errorf("event update snapshots are not ordered")
		}
	}
	return nil
}

type EventUpdate struct {
	ID              int64
	Version         int64
	EventID         int64
	SequenceNo      int64
	Kind            EventUpdateKind
	Summary         string
	ObservedAt      time.Time
	ReasonCodes     []string
	BeforeState     *EventUpdateState
	AfterState      EventUpdateState
	EvidenceSetHash string
	IdempotencyKey  string
	CreatedAt       time.Time
}

type EventUpdateListQuery struct {
	EventID int64
	Limit   int
	Cursor  int64
}

type EventUpdatePage struct {
	Items      []EventUpdate
	NextCursor int64
}

// DetectEventUpdate applies the ordered Design-019 v1 rules. Only one kind is
// selected, while every matching deterministic reason remains on the fact.
func DetectEventUpdate(previous *HeatResult, current HeatResult) (*EventUpdateCandidate, error) {
	if err := validateEventUpdateHeat(current); err != nil {
		return nil, err
	}
	key, err := EventUpdateIdempotencyKey(current)
	if err != nil {
		return nil, err
	}
	after := eventUpdateState(current)
	if previous == nil {
		return newEventUpdateCandidate(current, nil, after, EventUpdateEventCreated, []string{"first_snapshot"}, key), nil
	}
	if err := validateEventUpdateHeat(*previous); err != nil {
		return nil, err
	}
	if previous.EventID != current.EventID || !previous.WindowEnd.Before(current.WindowEnd) {
		return nil, fmt.Errorf("event update snapshots do not describe an ordered event history")
	}

	reactivated := (previous.TrendStatus == TrendDormant || previous.TrendStatus == TrendFalling) && risingTrend(current.TrendStatus)
	rising := !risingTrend(previous.TrendStatus) && risingTrend(current.TrendStatus)
	cooling := !coolingTrend(previous.TrendStatus) && coolingTrend(current.TrendStatus)
	sourceExpansion := current.SourceCount-previous.SourceCount >= 2 || previous.SourceCount > 0 && current.SourceCount >= previous.SourceCount*2
	heatDelta := math.Abs(current.HeatScore-previous.HeatScore) >= 10

	reasons := make([]string, 0, 4)
	if reactivated {
		reasons = append(reasons, "reactivated")
	}
	if rising {
		reasons = append(reasons, "rising")
	}
	if cooling {
		reasons = append(reasons, "cooling")
	}
	if sourceExpansion {
		reasons = append(reasons, "source_expansion")
	}
	if heatDelta {
		reasons = append(reasons, "heat_delta")
	}
	if len(reasons) == 0 {
		return nil, nil
	}

	kind := EventUpdateMetricChanged
	switch {
	case reactivated:
		kind = EventUpdateReactivated
	case rising:
		kind = EventUpdateRising
	case cooling:
		kind = EventUpdateCooling
	case sourceExpansion:
		kind = EventUpdateSourceExpansion
	}
	before := eventUpdateState(*previous)
	return newEventUpdateCandidate(current, &before, after, kind, reasons, key), nil
}

// EventUpdateIdempotencyKey is the exact metric-update-v1 ordered UTF-8 input
// from Design-019 v1.2, hashed as lowercase SHA-256 hexadecimal.
func EventUpdateIdempotencyKey(current HeatResult) (string, error) {
	if err := validateEventUpdateHeat(current); err != nil {
		return "", err
	}
	ordered := []string{
		EventUpdateAlgorithmVersionV1,
		strconv.FormatInt(current.EventID, 10),
		current.WindowEnd.UTC().Format(time.RFC3339Nano),
		strconv.Itoa(current.WindowHours),
		current.HeatVersion,
		current.EvidenceSetHash,
		current.CapabilityProfileSetHash,
		fmt.Sprintf("%.2f", current.HeatScore),
		fmt.Sprintf("%.2f", current.TrendScore),
		string(current.TrendStatus),
		strconv.Itoa(current.SourceCount),
		strconv.Itoa(current.ContentCount),
	}
	digest := sha256.Sum256([]byte(strings.Join(ordered, "\n")))
	return hex.EncodeToString(digest[:]), nil
}

func newEventUpdateCandidate(current HeatResult, before *EventUpdateState, after EventUpdateState, kind EventUpdateKind, reasons []string, key string) *EventUpdateCandidate {
	return &EventUpdateCandidate{
		EventID: current.EventID, Kind: kind, Summary: eventUpdateSummary(kind), ObservedAt: current.WindowEnd.UTC(),
		ReasonCodes: append([]string(nil), reasons...), BeforeState: before, AfterState: after,
		EvidenceSetHash: current.EvidenceSetHash, IdempotencyKey: key,
	}
}

func eventUpdateState(result HeatResult) EventUpdateState {
	return HeatResult{
		EventID:   result.EventID,
		HeatScore: result.HeatScore, TrendScore: result.TrendScore, TrendStatus: result.TrendStatus,
		SourceCount: result.SourceCount, ContentCount: result.ContentCount, WindowEnd: result.WindowEnd.UTC(), WindowHours: result.WindowHours,
		HeatVersion: result.HeatVersion, EvidenceSetHash: result.EvidenceSetHash, CapabilityProfileSetHash: result.CapabilityProfileSetHash,
	}
}

func validateEventUpdateHeat(result HeatResult) error {
	if result.EventID <= 0 || result.WindowHours != 24 || result.WindowEnd.IsZero() || strings.TrimSpace(result.HeatVersion) == "" ||
		!validHash(result.EvidenceSetHash) || !validHash(result.CapabilityProfileSetHash) ||
		!finiteInRange(result.HeatScore, 0, 100) || !finiteInRange(result.TrendScore, -100, 100) ||
		!result.TrendStatus.Valid() || result.SourceCount < 0 || result.ContentCount < 0 {
		return fmt.Errorf("invalid 24 hour event metric snapshot")
	}
	return nil
}

func risingTrend(status TrendStatus) bool  { return status == TrendEmerging || status == TrendRising }
func coolingTrend(status TrendStatus) bool { return status == TrendFalling || status == TrendDormant }

func eventUpdateSummary(kind EventUpdateKind) string {
	switch kind {
	case EventUpdateEventCreated:
		return "First valid 24-hour metric snapshot recorded."
	case EventUpdateReactivated:
		return "Event activity reactivated."
	case EventUpdateRising:
		return "Event activity entered a rising trend."
	case EventUpdateCooling:
		return "Event activity entered a cooling trend."
	case EventUpdateSourceExpansion:
		return "Independent source coverage expanded."
	case EventUpdateMetricChanged:
		return "Event heat changed materially."
	default:
		return "Event metrics changed."
	}
}
