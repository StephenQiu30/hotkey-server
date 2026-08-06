package http

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
)

// EventResult mirrors the shared Result envelope only for swag's source
// parser. Runtime output always uses platform HTTP helpers.
type EventResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

type EventResponse struct {
	ID                       int64      `json:"id"`
	Version                  int64      `json:"version"`
	EventKey                 string     `json:"event_key"`
	TitleZH                  string     `json:"title_zh"`
	TitleEN                  string     `json:"title_en,omitempty"`
	Summary                  string     `json:"summary"`
	LifecycleStatus          string     `json:"lifecycle_status"`
	FirstSeenAt              time.Time  `json:"first_seen_at"`
	LastSeenAt               time.Time  `json:"last_seen_at"`
	RepresentativeContentID  *int64     `json:"representative_content_id,omitempty"`
	MergedIntoID             *int64     `json:"merged_into_id,omitempty"`
	ManualLocked             bool       `json:"manual_locked"`
	HeatScore                float64    `json:"heat_score"`
	TrendScore               float64    `json:"trend_score"`
	TrendStatus              string     `json:"trend_status"`
	WindowHours              int        `json:"window_hours"`
	HeatVersion              string     `json:"heat_version"`
	ReasonCodes              []string   `json:"reason_codes"`
	CapabilityProfileSetHash string     `json:"capability_profile_set_hash"`
	CalculatedAt             *time.Time `json:"calculated_at,omitempty"`
}

type EventPageResponse struct {
	Items      []EventResponse `json:"items"`
	NextCursor int64           `json:"next_cursor,omitempty"`
}

type EventUpdateResponse struct {
	ID              int64                     `json:"id"`
	Version         int64                     `json:"version"`
	EventID         int64                     `json:"event_id"`
	SequenceNo      int64                     `json:"sequence_no"`
	Kind            string                    `json:"kind"`
	Summary         string                    `json:"summary"`
	ObservedAt      time.Time                 `json:"observed_at"`
	ReasonCodes     []string                  `json:"reason_codes"`
	BeforeState     *EventUpdateStateResponse `json:"before_state"`
	AfterState      EventUpdateStateResponse  `json:"after_state"`
	EvidenceSetHash string                    `json:"evidence_set_hash"`
}

type EventUpdateStateResponse struct {
	HeatScore                float64   `json:"heat_score"`
	TrendScore               float64   `json:"trend_score"`
	TrendStatus              string    `json:"trend_status"`
	SourceCount              int       `json:"source_count"`
	ContentCount             int       `json:"content_count"`
	WindowEnd                time.Time `json:"window_end"`
	WindowHours              int       `json:"window_hours"`
	HeatVersion              string    `json:"heat_version"`
	EvidenceSetHash          string    `json:"evidence_set_hash"`
	CapabilityProfileSetHash string    `json:"capability_profile_set_hash"`
}

type EventUpdatePageResponse struct {
	Items      []EventUpdateResponse `json:"items"`
	NextCursor int64                 `json:"next_cursor,omitempty"`
}

type RadarEventResponse struct {
	EventID                int64                `json:"event_id"`
	Version                int64                `json:"version"`
	EventKey               string               `json:"event_key"`
	Title                  string               `json:"title"`
	TitleZH                string               `json:"title_zh"`
	TitleEN                string               `json:"title_en,omitempty"`
	Summary                string               `json:"summary"`
	LifecycleStatus        string               `json:"lifecycle_status"`
	FirstSeenAt            time.Time            `json:"first_seen_at"`
	LastSeenAt             time.Time            `json:"last_seen_at"`
	TrendScore             float64              `json:"trend_score"`
	TrendStatus            string               `json:"trend_status"`
	Attention              float64              `json:"attention"`
	Momentum               float64              `json:"momentum"`
	Breadth                float64              `json:"breadth"`
	IndependentSourceCount int                  `json:"independent_source_count"`
	Confirmation           string               `json:"confirmation"`
	ConfirmationScore      *float64             `json:"confirmation_score"`
	DataConfidence         float64              `json:"data_confidence"`
	WatchRelevance         *float64             `json:"watch_relevance,omitempty"`
	WatchFinalScore        *float64             `json:"watch_final_score,omitempty"`
	RankingScore           float64              `json:"ranking_score"`
	ReasonCodes            []string             `json:"reason_codes"`
	LatestUpdate           *EventUpdateResponse `json:"latest_update,omitempty"`
}

type RadarPageResponse struct {
	Items      []RadarEventResponse `json:"items"`
	NextCursor string               `json:"next_cursor,omitempty"`
	AsOf       time.Time            `json:"as_of"`
}

type EventMemberResponse struct {
	ID              int64   `json:"id"`
	Version         int64   `json:"version"`
	EventID         int64   `json:"event_id"`
	ContentID       int64   `json:"content_id"`
	MembershipScore float64 `json:"membership_score"`
	EvidenceRole    string  `json:"evidence_role"`
	Representative  bool    `json:"representative"`
	Origin          string  `json:"origin"`
	ManualLocked    bool    `json:"manual_locked"`
}

type EventMemberPageResponse struct {
	Items []EventMemberResponse `json:"items"`
}

type HeatResponse struct {
	EventID                  int64     `json:"event_id"`
	HeatScore                float64   `json:"heat_score"`
	TrendScore               float64   `json:"trend_score"`
	TrendStatus              string    `json:"trend_status"`
	SourceCount              int       `json:"source_count"`
	ContentCount             int       `json:"content_count"`
	WindowHours              int       `json:"window_hours"`
	HeatVersion              string    `json:"heat_version"`
	EvidenceSetHash          string    `json:"evidence_set_hash"`
	CapabilityProfileSetHash string    `json:"capability_profile_set_hash"`
	ReasonCodes              []string  `json:"reason_codes"`
	CapturedAt               time.Time `json:"captured_at"`
}

type ClaimEvidenceRequest struct {
	ContentID  int64   `json:"content_id" binding:"required"`
	Locator    string  `json:"locator" binding:"required"`
	Excerpt    string  `json:"excerpt"`
	Stance     string  `json:"stance" binding:"required"`
	Confidence float64 `json:"confidence"`
}

type ClaimRequest struct {
	ID              int64                  `json:"id" binding:"required"`
	Version         int64                  `json:"version" binding:"required"`
	NormalizedClaim string                 `json:"normalized_claim" binding:"required"`
	ClaimHash       string                 `json:"claim_hash" binding:"required"`
	Status          string                 `json:"status" binding:"required"`
	Confidence      float64                `json:"confidence"`
	ManualLocked    bool                   `json:"manual_locked"`
	Evidence        []ClaimEvidenceRequest `json:"evidence" binding:"required,min=1"`
}

type ClaimResponse struct {
	ID              int64   `json:"id"`
	Version         int64   `json:"version"`
	EventID         int64   `json:"event_id"`
	NormalizedClaim string  `json:"normalized_claim"`
	ClaimHash       string  `json:"claim_hash"`
	Status          string  `json:"status"`
	Confidence      float64 `json:"confidence"`
}

type IntelligenceEvidenceResponse struct {
	ContentID  int64   `json:"content_id"`
	Locator    string  `json:"locator"`
	Excerpt    string  `json:"excerpt"`
	Stance     string  `json:"stance"`
	Confidence float64 `json:"confidence"`
}

type IntelligenceClaimResponse struct {
	ID              int64                          `json:"id"`
	Version         int64                          `json:"version"`
	NormalizedClaim string                         `json:"normalized_claim"`
	ClaimHash       string                         `json:"claim_hash"`
	Status          string                         `json:"status"`
	Confidence      float64                        `json:"confidence"`
	ManualLocked    bool                           `json:"manual_locked"`
	Evidence        []IntelligenceEvidenceResponse `json:"evidence"`
}

type IntelligenceEntityResponse struct {
	EntityID        int64   `json:"entity_id"`
	EntityVersion   int64   `json:"entity_version"`
	EntityKey       string  `json:"entity_key"`
	EntityType      string  `json:"entity_type"`
	CanonicalName   string  `json:"canonical_name"`
	EntityLocked    bool    `json:"entity_locked"`
	RelationID      int64   `json:"relation_id"`
	RelationVersion int64   `json:"relation_version"`
	Role            string  `json:"role"`
	Confidence      float64 `json:"confidence"`
	Origin          string  `json:"origin"`
	Confirmed       bool    `json:"confirmed"`
}

type EventIntelligenceResponse struct {
	EventID  int64                        `json:"event_id"`
	Entities []IntelligenceEntityResponse `json:"entities"`
	Claims   []IntelligenceClaimResponse  `json:"claims"`
}

type SummarySentenceResponse struct {
	Text     string                         `json:"text"`
	Evidence []IntelligenceEvidenceResponse `json:"evidence"`
}

type EventSummaryResponse struct {
	Version   string                    `json:"version"`
	TitleZH   string                    `json:"title_zh"`
	TitleEN   string                    `json:"title_en,omitempty"`
	Degraded  bool                      `json:"degraded"`
	Sentences []SummarySentenceResponse `json:"sentences"`
}

type SummaryRegenerationResponse struct {
	EventID    int64                `json:"event_id"`
	Status     string               `json:"status"`
	ReasonCode string               `json:"reason_code,omitempty"`
	RunID      int64                `json:"run_id,omitempty"`
	Reused     bool                 `json:"reused"`
	Summary    EventSummaryResponse `json:"summary"`
}

type ExtractionRegenerationResponse struct {
	EventID     int64  `json:"event_id"`
	Status      string `json:"status"`
	ReasonCode  string `json:"reason_code,omitempty"`
	RunID       int64  `json:"run_id,omitempty"`
	Reused      bool   `json:"reused"`
	EntityCount int    `json:"entity_count"`
	ClaimCount  int    `json:"claim_count"`
}

type LifecycleRequest struct {
	ExpectedVersion int64  `json:"expected_version" binding:"required"`
	To              string `json:"to" binding:"required"`
	Reason          string `json:"reason" binding:"required,max=64"`
}

type MemberLockRequest struct {
	ExpectedVersion int64  `json:"expected_version" binding:"required"`
	Locked          bool   `json:"locked"`
	Reason          string `json:"reason" binding:"required,max=64"`
}

type MergeRequest struct {
	TargetEventID         int64  `json:"target_event_id" binding:"required"`
	SourceExpectedVersion int64  `json:"source_expected_version" binding:"required"`
	TargetExpectedVersion int64  `json:"target_expected_version" binding:"required"`
	Reason                string `json:"reason" binding:"required,max=64"`
}

type SplitMemberRequest struct {
	ContentID       int64 `json:"content_id" binding:"required"`
	ExpectedVersion int64 `json:"expected_version" binding:"required"`
}

type SplitRequest struct {
	SourceExpectedVersion int64                `json:"source_expected_version" binding:"required"`
	Members               []SplitMemberRequest `json:"members" binding:"required,min=1"`
	Reason                string               `json:"reason" binding:"required,max=64"`
}

func eventResponse(event domain.Event) EventResponse {
	return EventResponse{ID: event.ID, Version: event.Version, EventKey: event.EventKey, TitleZH: event.TitleZH, TitleEN: event.TitleEN, Summary: event.Summary, LifecycleStatus: string(event.LifecycleStatus), FirstSeenAt: event.FirstSeenAt, LastSeenAt: event.LastSeenAt, RepresentativeContentID: event.RepresentativeContentID, MergedIntoID: event.MergedIntoID, ManualLocked: event.ManualLocked, HeatScore: event.HeatScore, TrendScore: event.TrendScore, TrendStatus: string(event.TrendStatus), WindowHours: event.HeatWindowHours, HeatVersion: event.HeatVersion, ReasonCodes: event.HeatReasonCodes, CapabilityProfileSetHash: event.MetricCapabilityProfileSetHash, CalculatedAt: event.HeatCalculatedAt}
}

func eventUpdateResponse(update domain.EventUpdate) EventUpdateResponse {
	response := EventUpdateResponse{
		ID: update.ID, Version: update.Version, EventID: update.EventID, SequenceNo: update.SequenceNo,
		Kind: string(update.Kind), Summary: update.Summary, ObservedAt: update.ObservedAt,
		ReasonCodes: append([]string(nil), update.ReasonCodes...), AfterState: eventUpdateStateResponse(update.AfterState),
		EvidenceSetHash: update.EvidenceSetHash,
	}
	if update.BeforeState != nil {
		before := eventUpdateStateResponse(*update.BeforeState)
		response.BeforeState = &before
	}
	return response
}

func eventUpdateStateResponse(state domain.HeatResult) EventUpdateStateResponse {
	return EventUpdateStateResponse{
		HeatScore: state.HeatScore, TrendScore: state.TrendScore, TrendStatus: string(state.TrendStatus),
		SourceCount: state.SourceCount, ContentCount: state.ContentCount, WindowEnd: state.WindowEnd,
		WindowHours: state.WindowHours, HeatVersion: state.HeatVersion, EvidenceSetHash: state.EvidenceSetHash,
		CapabilityProfileSetHash: state.CapabilityProfileSetHash,
	}
}

func radarEventResponse(event domain.RadarEvent) RadarEventResponse {
	response := RadarEventResponse{
		EventID: event.EventID, Version: event.Version, EventKey: event.EventKey,
		Title: event.TitleZH, TitleZH: event.TitleZH, TitleEN: event.TitleEN, Summary: event.Summary,
		LifecycleStatus: string(event.LifecycleStatus), FirstSeenAt: event.FirstSeenAt, LastSeenAt: event.LastSeenAt,
		TrendScore: event.TrendScore, TrendStatus: string(event.TrendStatus),
		Attention: event.Attention, Momentum: event.Momentum, Breadth: event.Breadth,
		IndependentSourceCount: event.IndependentSourceCount,
		Confirmation:           string(event.Confirmation), ConfirmationScore: event.ConfirmationScore,
		DataConfidence: event.DataConfidence, WatchRelevance: event.WatchRelevance,
		WatchFinalScore: event.WatchFinalScore, RankingScore: event.RankingScore,
		ReasonCodes: append([]string(nil), event.ReasonCodes...),
	}
	if event.LatestUpdate != nil {
		latest := eventUpdateResponse(*event.LatestUpdate)
		response.LatestUpdate = &latest
	}
	return response
}

func memberResponse(member domain.EventMember) EventMemberResponse {
	return EventMemberResponse{ID: member.ID, Version: member.Version, EventID: member.EventID, ContentID: member.ContentID, MembershipScore: member.MembershipScore, EvidenceRole: string(member.EvidenceRole), Representative: member.Representative, Origin: string(member.Origin), ManualLocked: member.ManualLocked}
}
