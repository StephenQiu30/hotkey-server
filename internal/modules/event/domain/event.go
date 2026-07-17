// Package domain contains Event facts and the deterministic clustering rules.
// It deliberately has no database, HTTP or provider dependency.
package domain

import (
	"fmt"
	"strings"
	"time"
)

type LifecycleStatus string

const (
	LifecycleDetected LifecycleStatus = "detected"
	LifecycleActive   LifecycleStatus = "active"
	LifecycleCooling  LifecycleStatus = "cooling"
	LifecycleClosed   LifecycleStatus = "closed"
	LifecycleArchived LifecycleStatus = "archived"
	LifecycleRejected LifecycleStatus = "rejected"
	LifecycleMerged   LifecycleStatus = "merged"
)

func (status LifecycleStatus) Valid() bool {
	switch status {
	case LifecycleDetected, LifecycleActive, LifecycleCooling, LifecycleClosed, LifecycleArchived, LifecycleRejected, LifecycleMerged:
		return true
	default:
		return false
	}
}

// CanTransition is the complete V1 state machine. Repeated transitions are
// handled as idempotent commands by the application layer, not as new edges.
func CanTransition(from, to LifecycleStatus) bool {
	switch from {
	case LifecycleDetected:
		return to == LifecycleActive || to == LifecycleRejected || to == LifecycleMerged
	case LifecycleActive:
		return to == LifecycleCooling || to == LifecycleClosed || to == LifecycleRejected || to == LifecycleMerged
	case LifecycleCooling:
		return to == LifecycleActive || to == LifecycleClosed || to == LifecycleRejected || to == LifecycleMerged
	case LifecycleClosed:
		return to == LifecycleActive || to == LifecycleRejected || to == LifecycleMerged
	case LifecycleArchived:
		return false
	case LifecycleRejected:
		return false
	case LifecycleMerged:
		return false
	default:
		return false
	}
}

type Event struct {
	ID                      int64
	Version                 int64
	EventKey                string
	EventFingerprint        string
	FingerprintVersion      string
	TitleZH, TitleEN        string
	Summary                 string
	LifecycleStatus         LifecycleStatus
	FirstSeenAt, LastSeenAt time.Time
	RepresentativeContentID *int64
	MergedIntoID            *int64
	ManualLocked            bool
}

func (event Event) Validate() error {
	if event.ID <= 0 || event.Version <= 0 || strings.TrimSpace(event.EventKey) == "" || len(event.EventKey) > 96 || !event.LifecycleStatus.Valid() || event.FirstSeenAt.IsZero() || event.LastSeenAt.IsZero() {
		return fmt.Errorf("invalid event")
	}
	if event.LastSeenAt.Before(event.FirstSeenAt) {
		return fmt.Errorf("event last_seen_at precedes first_seen_at")
	}
	if event.LifecycleStatus == LifecycleMerged && (event.MergedIntoID == nil || *event.MergedIntoID <= 0 || *event.MergedIntoID == event.ID) {
		return fmt.Errorf("merged event requires a different target")
	}
	if event.LifecycleStatus != LifecycleMerged && event.MergedIntoID != nil {
		return fmt.Errorf("only merged event may have merged_into_id")
	}
	return nil
}

type MemberOrigin string

const (
	MemberOriginRule  MemberOrigin = "rule"
	MemberOriginModel MemberOrigin = "model"
	MemberOriginUser  MemberOrigin = "user"
)

func (origin MemberOrigin) Valid() bool {
	return origin == MemberOriginRule || origin == MemberOriginModel || origin == MemberOriginUser
}

type EvidenceRole string

const (
	EvidencePrimary    EvidenceRole = "primary"
	EvidenceSupporting EvidenceRole = "supporting"
	EvidenceContext    EvidenceRole = "context"
	EvidenceDuplicate  EvidenceRole = "duplicate"
)

func (role EvidenceRole) Valid() bool {
	return role == EvidencePrimary || role == EvidenceSupporting || role == EvidenceContext || role == EvidenceDuplicate
}

type EventMember struct {
	ID, Version, EventID, ContentID int64
	MembershipScore                 float64
	EvidenceRole                    EvidenceRole
	Representative                  bool
	Origin                          MemberOrigin
	ManualLocked                    bool
}

func (member EventMember) Validate() error {
	if member.ID <= 0 || member.Version <= 0 || member.EventID <= 0 || member.ContentID <= 0 || member.MembershipScore < 0 || member.MembershipScore > 100 || !member.EvidenceRole.Valid() || !member.Origin.Valid() {
		return fmt.Errorf("invalid event member")
	}
	return nil
}

type CandidateChannel string

const (
	ChannelLexical     CandidateChannel = "lexical"
	ChannelTemporal    CandidateChannel = "temporal"
	ChannelFingerprint CandidateChannel = "fingerprint"
	ChannelVector      CandidateChannel = "vector"
)

func (channel CandidateChannel) Valid() bool {
	return channel == ChannelLexical || channel == ChannelTemporal || channel == ChannelFingerprint || channel == ChannelVector
}
