package http

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type MicroEventHandler struct {
	queries    *eventapplication.MicroEventQueryService
	governance *eventapplication.MicroEventGovernanceService
	evidence   *eventapplication.ClaimEvidenceService
}

func NewMicroEventHandler(queries *eventapplication.MicroEventQueryService, governance *eventapplication.MicroEventGovernanceService, evidence *eventapplication.ClaimEvidenceService) *MicroEventHandler {
	return &MicroEventHandler{queries: queries, governance: governance, evidence: evidence}
}

// List returns only v2 MicroEvent projections; legacy confidence and
// verification fields are structurally absent.
// @Summary List semantic micro-events
// @Tags micro-events
// @Produce json
// @Security BearerAuth
// @Param cursor query string false "opaque frozen-ranking cursor"
// @Param sort query string false "server ranking" Enums(heat,relevance,latest) default(heat)
// @Param limit query int false "page size" minimum(1) maximum(100)
// @Param status query string false "comma-separated lifecycle states"
// @Param monitor_id query int false "monitor relevance filter" minimum(1)
// @Param source_type query string false "comma-separated source types"
// @Param evidence_state query string false "comma-separated evidence states"
// @Param started_from query string false "event start lower bound in RFC3339"
// @Param started_to query string false "event start upper bound in RFC3339"
// @Success 200 {object} MicroEventV2Result[MicroEventPageResponseDTO]
// @Failure 400 {object} MicroEventV2Result[EmptyResponse]
// @Failure 401 {object} MicroEventV2Result[EmptyResponse]
// @Failure 503 {object} MicroEventV2Result[EmptyResponse]
// @Router /api/v1/micro-events [get]
func (handler *MicroEventHandler) List(c *gin.Context) error {
	c.Header("Cache-Control", "private, no-store")
	limit, err := queryLimit(c.Query("limit"))
	if err != nil {
		return err
	}
	cursor := strings.TrimSpace(c.Query("cursor"))
	monitorID, err := optionalPositiveQueryID(c.Query("monitor_id"))
	if err != nil {
		return err
	}
	startedFrom, err := optionalRFC3339QueryTime(c.Query("started_from"))
	if err != nil {
		return err
	}
	startedTo, err := optionalRFC3339QueryTime(c.Query("started_to"))
	if err != nil {
		return err
	}
	page, err := handler.queries.List(c.Request.Context(), eventapplication.MicroEventListQuery{
		Cursor: cursor, Limit: limit, Sort: c.Query("sort"), MonitorID: monitorID,
		Statuses: commaSeparatedQueryValues(c.Query("status")), SourceTypes: commaSeparatedQueryValues(c.Query("source_type")),
		EvidenceStates: commaSeparatedQueryValues(c.Query("evidence_state")), StartedFrom: startedFrom, StartedTo: startedTo,
	})
	if err != nil {
		return eventError(err)
	}
	response := MicroEventPageResponseDTO{Items: make([]MicroEventResponseDTO, len(page.Items)), NextCursor: page.NextCursor}
	for index, item := range page.Items {
		response.Items[index] = microEventResponseDTO(item)
	}
	httptransport.OK(c, response)
	return nil
}

func commaSeparatedQueryValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	values := strings.Split(raw, ",")
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
	}
	return values
}

func optionalPositiveQueryID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, invalidRequest(fmt.Errorf("positive query id is required"))
	}
	return value, nil
}

func optionalRFC3339QueryTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, invalidRequest(fmt.Errorf("RFC3339 query time is required"))
	}
	value = value.UTC()
	return &value, nil
}

// Get returns a single MicroEvent with Storyline, Heat v2, and descriptive EvidenceState.
// @Summary Get semantic micro-event
// @Tags micro-events
// @Produce json
// @Security BearerAuth
// @Param id path int true "micro-event ID"
// @Success 200 {object} MicroEventV2Result[MicroEventResponseDTO]
// @Failure 400 {object} MicroEventV2Result[EmptyResponse]
// @Failure 401 {object} MicroEventV2Result[EmptyResponse]
// @Failure 404 {object} MicroEventV2Result[EmptyResponse]
// @Router /api/v1/micro-events/{id} [get]
func (handler *MicroEventHandler) Get(c *gin.Context) error {
	c.Header("Cache-Control", "private, no-store")
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	item, err := handler.queries.Get(c.Request.Context(), id)
	if err != nil {
		return eventError(err)
	}
	c.Header("ETag", fmt.Sprintf("\"v%d\"", item.Version))
	httptransport.OK(c, microEventResponseDTO(item))
	return nil
}

// Evidence lists exact-version ClaimEvidence projections. Revoked or expired
// excerpts return availability only and never leak quote text or hashes.
// @Summary List micro-event evidence
// @Tags micro-events
// @Produce json
// @Security BearerAuth
// @Param id path int true "micro-event ID"
// @Param cursor query string false "opaque evidence snapshot cursor"
// @Param limit query int false "page size" minimum(1) maximum(100)
// @Success 200 {object} MicroEventV2Result[MicroEventEvidencePageResponseDTO]
// @Failure 400 {object} MicroEventV2Result[EmptyResponse]
// @Failure 401 {object} MicroEventV2Result[EmptyResponse]
// @Failure 404 {object} MicroEventV2Result[EmptyResponse]
// @Router /api/v1/micro-events/{id}/evidence [get]
func (handler *MicroEventHandler) Evidence(c *gin.Context) error {
	c.Header("Cache-Control", "private, no-store")
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	limit, err := queryLimit(c.Query("limit"))
	if err != nil {
		return err
	}
	page, err := handler.queries.Evidence(c.Request.Context(), eventapplication.MicroEventEvidenceQuery{
		MicroEventID: id, Cursor: strings.TrimSpace(c.Query("cursor")), Limit: limit,
	})
	if err != nil {
		return eventError(err)
	}
	response := MicroEventEvidencePageResponseDTO{Items: make([]ClaimEvidenceResponseDTO, len(page.Items)), NextCursor: page.NextCursor}
	for index, item := range page.Items {
		response.Items[index] = claimEvidenceResponseDTO(item)
	}
	httptransport.OK(c, response)
	return nil
}

// RecordEvidence appends an editor-reviewed ClaimEvidence fact bound to a
// prevalidated TextQuoteSelector.
// @Summary Append manually reviewed claim evidence
// @Tags micro-events
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "micro-event ID"
// @Param If-Match header string true "strong event version ETag"
// @Param Idempotency-Key header string true "durable idempotency key"
// @Param request body RecordClaimEvidenceRequestDTO true "exact quoted claim relation"
// @Success 201 {object} MicroEventV2Result[ClaimEvidenceMutationResponseDTO]
// @Success 200 {object} MicroEventV2Result[ClaimEvidenceMutationResponseDTO]
// @Failure 400 {object} MicroEventV2Result[EmptyResponse]
// @Failure 401 {object} MicroEventV2Result[EmptyResponse]
// @Failure 403 {object} MicroEventV2Result[EmptyResponse]
// @Failure 409 {object} MicroEventV2Result[EmptyResponse]
// @Router /api/v1/micro-events/{id}/evidence [post]
func (handler *MicroEventHandler) RecordEvidence(c *gin.Context) error {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return sharederrors.New(sharederrors.CodeForbidden, 403, "")
	}
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var request RecordClaimEvidenceRequestDTO
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	if err := requireMicroEventVersion(c, request.ExpectedEventVersion); err != nil {
		return err
	}
	key, err := microEventIdempotencyKey(c)
	if err != nil {
		return err
	}
	qualifiers := make([]eventapplication.ClaimQualifierDTO, len(request.Qualifiers))
	for index, item := range request.Qualifiers {
		qualifiers[index] = eventapplication.ClaimQualifierDTO{Key: item.Key, Value: item.Value}
	}
	result, err := handler.evidence.Record(c.Request.Context(), eventapplication.RecordClaimEvidenceCommand{MicroEventID: id,
		ExpectedEventVersion: request.ExpectedEventVersion, DocumentVersionID: request.DocumentVersionID,
		TextQuoteSelectorID: request.TextQuoteSelectorID, Subject: request.Subject, Predicate: request.Predicate,
		Object: request.Object, Qualifiers: qualifiers, Relation: request.Relation,
		ExtractionSchemaVersion: eventapplication.CanonicalClaimExtractionSchemaVersion, Origin: "manual",
		ActorUserID: &subject.UserID, IdempotencyKey: key, DecisionAt: time.Now().UTC()})
	if err != nil {
		return eventError(err)
	}
	response := ClaimEvidenceMutationResponseDTO{ClaimID: result.Claim.ID, ClaimVersion: result.Claim.Version,
		EvidenceID: result.Evidence.ID, EvidenceVersion: result.Evidence.Version}
	response.EvidenceState = handler.recalculateEvidenceState(c, id, request.ExpectedEventVersion)
	if result.Created {
		httptransport.Created(c, response)
	} else {
		httptransport.OK(c, response)
	}
	return nil
}

// CorrectEvidence appends a relation/locator correction and retains both versions.
// @Summary Correct a claim evidence relation or locator
// @Tags micro-events
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "micro-event ID"
// @Param evidence_id path int true "original claim evidence version ID"
// @Param If-Match header string true "strong claim version ETag"
// @Param Idempotency-Key header string true "durable idempotency key"
// @Param request body CorrectClaimEvidenceRequestDTO true "correction facts"
// @Success 201 {object} MicroEventV2Result[ClaimEvidenceCorrectionResponseDTO]
// @Success 200 {object} MicroEventV2Result[ClaimEvidenceCorrectionResponseDTO]
// @Failure 400 {object} MicroEventV2Result[EmptyResponse]
// @Failure 401 {object} MicroEventV2Result[EmptyResponse]
// @Failure 403 {object} MicroEventV2Result[EmptyResponse]
// @Failure 409 {object} MicroEventV2Result[EmptyResponse]
// @Router /api/v1/micro-events/{id}/evidence/{evidence_id}/feedback [post]
func (handler *MicroEventHandler) CorrectEvidence(c *gin.Context) error {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return sharederrors.New(sharederrors.CodeForbidden, 403, "")
	}
	eventID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	evidenceID, err := pathID(c, "evidence_id")
	if err != nil {
		return err
	}
	var request CorrectClaimEvidenceRequestDTO
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	if err := requireMicroEventVersion(c, request.ExpectedClaimVersion); err != nil {
		return err
	}
	key, err := microEventIdempotencyKey(c)
	if err != nil {
		return err
	}
	result, err := handler.evidence.Correct(c.Request.Context(), eventapplication.CorrectClaimEvidenceCommand{
		OriginalClaimEvidenceVersionID: evidenceID, ExpectedClaimVersion: request.ExpectedClaimVersion,
		ResultTextQuoteSelectorID: request.ResultTextQuoteSelectorID, ResultRelation: request.ResultRelation,
		ActorUserID: subject.UserID, ReasonCode: request.ReasonCode, Note: request.Note,
		IdempotencyKey: key, DecisionAt: time.Now().UTC()})
	if err != nil {
		return eventError(err)
	}
	detail, err := handler.queries.Get(c.Request.Context(), eventID)
	if err != nil {
		return eventError(err)
	}
	response := ClaimEvidenceCorrectionResponseDTO{FeedbackID: result.Feedback.ID, EvidenceID: result.Evidence.ID, EvidenceVersion: result.Evidence.Version}
	response.EvidenceState = handler.recalculateEvidenceState(c, eventID, detail.Version)
	if result.Created {
		httptransport.Created(c, response)
	} else {
		httptransport.OK(c, response)
	}
	return nil
}

// Govern appends merge/split/move/close/reopen feedback against exact versions.
// @Summary Apply micro-event governance feedback
// @Tags micro-events
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "micro-event ID"
// @Param If-Match header string true "strong event version ETag"
// @Param Idempotency-Key header string true "durable idempotency key"
// @Param request body MicroEventGovernanceRequestDTO true "governance action"
// @Success 200 {object} MicroEventV2Result[MicroEventGovernanceResponseDTO]
// @Failure 400 {object} MicroEventV2Result[EmptyResponse]
// @Failure 401 {object} MicroEventV2Result[EmptyResponse]
// @Failure 403 {object} MicroEventV2Result[EmptyResponse]
// @Failure 409 {object} MicroEventV2Result[EmptyResponse]
// @Router /api/v1/micro-events/{id}/feedback [post]
func (handler *MicroEventHandler) Govern(c *gin.Context) error {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return sharederrors.New(sharederrors.CodeForbidden, 403, "")
	}
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var request MicroEventGovernanceRequestDTO
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	if err := requireMicroEventVersion(c, request.ExpectedEventVersion); err != nil {
		return err
	}
	key, err := microEventIdempotencyKey(c)
	if err != nil {
		return err
	}
	result, err := handler.governance.Govern(c.Request.Context(), eventapplication.GovernMicroEventCommand{ActorUserID: subject.UserID,
		Action: request.Action, MicroEventID: id, ExpectedEventVersion: request.ExpectedEventVersion,
		MembershipDecisionID: request.MembershipDecisionID, ContentFamilyID: request.ContentFamilyID,
		ExpectedMemberVersion: request.ExpectedMemberVersion, TargetMicroEventID: request.TargetMicroEventID,
		ExpectedTargetEventVersion: request.ExpectedTargetEventVersion, ReasonCode: request.ReasonCode, Note: request.Note,
		GovernanceProfileVersion: eventapplication.CanonicalMicroEventGovernanceProfileVersion, IdempotencyKey: key})
	if err != nil {
		return eventError(err)
	}
	response := MicroEventGovernanceResponseDTO{FeedbackID: result.Feedback.ID,
		SourceEvent: MicroEventGovernanceResultDTO{ID: result.SourceEvent.ID, Version: result.SourceEvent.Version, Status: result.SourceEvent.Status}}
	if result.TargetEvent != nil {
		response.TargetEvent = &MicroEventGovernanceResultDTO{ID: result.TargetEvent.ID, Version: result.TargetEvent.Version, Status: result.TargetEvent.Status}
	}
	httptransport.OK(c, response)
	return nil
}

func (handler *MicroEventHandler) recalculateEvidenceState(c *gin.Context, eventID, eventVersion int64) *EvidenceStateResponseDTO {
	result, err := handler.evidence.CalculateState(c.Request.Context(), eventapplication.CalculateEvidenceStateCommand{MicroEventID: eventID,
		ExpectedEventVersion: eventVersion, AlgorithmVersion: eventapplication.CanonicalEvidenceStateAlgorithmVersion, CalculatedAt: time.Now().UTC()})
	if err != nil {
		if errors.Is(err, sharedrepository.ErrNotFound) {
			return nil
		}
		return nil
	}
	return evidenceStateResponseDTO(result.Snapshot)
}

func requireMicroEventVersion(c *gin.Context, expected int64) error {
	values := c.Request.Header.Values("If-Match")
	if expected <= 0 || len(values) != 1 || values[0] != fmt.Sprintf("\"v%d\"", expected) {
		return invalidRequest(fmt.Errorf("one strong If-Match matching the expected version is required"))
	}
	return nil
}
func microEventIdempotencyKey(c *gin.Context) (string, error) {
	values := c.Request.Header.Values("Idempotency-Key")
	if len(values) != 1 {
		return "", invalidRequest(fmt.Errorf("one Idempotency-Key is required"))
	}
	value := strings.TrimSpace(values[0])
	if value == "" || len(value) > 96 || strings.ContainsAny(value, "\r\n") {
		return "", invalidRequest(fmt.Errorf("invalid Idempotency-Key"))
	}
	return value, nil
}

func microEventResponseDTO(item eventapplication.MicroEventProjectionDTO) MicroEventResponseDTO {
	value := MicroEventResponseDTO{ID: item.ID, Version: item.Version, EventKey: item.EventKey, Status: item.Status,
		PrimarySubjectKey: item.PrimarySubjectKey, PrimaryActionKey: item.PrimaryActionKey,
		LocationKeys: item.LocationKeys, IdentifierKeys: item.IdentifierKeys, EventStartedAt: item.EventStartedAt,
		EventEndedAt: item.EventEndedAt, ClusteringProfileVersion: item.ClusteringProfileVersion,
		RelevanceScore: item.RelevanceScore, ContentFamilyCount: item.ContentFamilyCount, DocumentCount: item.DocumentCount}
	if len(item.Members) > 0 {
		value.Members = make([]MicroEventMemberResponseDTO, len(item.Members))
		for index, member := range item.Members {
			value.Members[index] = MicroEventMemberResponseDTO{ID: member.ID, Version: member.Version,
				ContentFamilyID: member.ContentFamilyID, MembershipDecisionID: member.MembershipDecisionID,
				ClusteringProfileVersion: member.ClusteringProfileVersion}
		}
	}
	if item.Storyline != nil {
		value.Storyline = &StorylineResponseDTO{ID: item.Storyline.ID, Version: item.Storyline.Version,
			StorylineKey: item.Storyline.StorylineKey, Title: item.Storyline.Title, Summary: item.Storyline.Summary,
			Status: item.Storyline.Status, RelationProfileVersion: item.Storyline.RelationProfileVersion}
	}
	if item.LatestHeat != nil {
		value.LatestHeat = &EventHeatV2ResponseDTO{ID: item.LatestHeat.ID,
			MicroEventVersion: item.LatestHeat.MicroEventVersion, HeatProfileVersion: item.LatestHeat.HeatProfileVersion,
			WindowStartedAt: item.LatestHeat.WindowStartedAt,
			WindowEndedAt:   item.LatestHeat.WindowEndedAt, IndependentLineageRootCount: item.LatestHeat.IndependentLineageRoots,
			Velocity: item.LatestHeat.Velocity, Acceleration: item.LatestHeat.Acceleration, Coverage: item.LatestHeat.Coverage,
			NormalizedEngagement: item.LatestHeat.NormalizedEngagement, Recency: item.LatestHeat.Recency,
			AvailableWeight: item.LatestHeat.AvailableWeight, HeatScore: item.LatestHeat.HeatScore,
			WarmingUp: item.LatestHeat.WarmingUp, ReasonCodes: item.LatestHeat.ReasonCodes}
	}
	if item.LatestEvidenceState != nil {
		value.EvidenceState = evidenceStateResponseDTO(*item.LatestEvidenceState)
	}
	if item.LatestSummary != nil {
		summary := &EvidenceSummaryResponseDTO{ID: item.LatestSummary.ID, EventVersion: item.LatestSummary.EventVersion,
			SummaryProfileVersion: item.LatestSummary.SummaryProfileVersion, CreatedAt: item.LatestSummary.CreatedAt,
			Sentences: make([]EvidenceSummarySentenceResponseDTO, len(item.LatestSummary.Sentences))}
		for index, sentence := range item.LatestSummary.Sentences {
			summary.Sentences[index] = EvidenceSummarySentenceResponseDTO{ID: sentence.ID, Ordinal: sentence.Ordinal,
				Text: sentence.Text, EditorialNote: sentence.EditorialNote,
				ClaimEvidenceVersionIDs: append([]int64(nil), sentence.ClaimEvidenceVersionIDs...),
				DecisionOrigin:          sentence.DecisionOrigin}
		}
		value.EvidenceSummary = summary
	}
	return value
}
func evidenceStateResponseDTO(item eventapplication.EvidenceStateSnapshotDTO) *EvidenceStateResponseDTO {
	return &EvidenceStateResponseDTO{ID: item.ID, EventVersion: item.EventVersion, AlgorithmVersion: item.AlgorithmVersion,
		State: item.State, IndependentOriginCount: item.IndependentOriginCount, ReasonCodes: item.ReasonCodes, CalculatedAt: item.CalculatedAt}
}
func claimEvidenceResponseDTO(item eventapplication.ClaimEvidenceProjectionDTO) ClaimEvidenceResponseDTO {
	return ClaimEvidenceResponseDTO{ID: item.ID, Version: item.Version, ClaimID: item.ClaimID, DocumentVersionID: item.DocumentVersionID,
		ClaimVersion:        item.ClaimVersion,
		TextQuoteSelectorID: item.TextQuoteSelectorID, ContentFamilyID: item.ContentFamilyID, LineageRootID: item.LineageRootID,
		LineageDecisionID: item.LineageDecisionID, ContentFamilyMemberVersion: item.ContentFamilyMemberVersion,
		Subject: item.ClaimSubject, Predicate: item.ClaimPredicate, Object: item.ClaimObject, Relation: item.Relation,
		Availability: item.Availability, ExactQuote: item.ExactQuote, Prefix: item.Prefix, Suffix: item.Suffix,
		UTF8ByteStart: item.UTF8ByteStart, UTF8ByteEnd: item.UTF8ByteEnd, QuoteSHA256: item.QuoteSHA256,
		PlaintextSHA256: item.PlaintextSHA256, SelectorVersion: item.SelectorVersion, MarkdownAnchor: item.MarkdownAnchor,
		SourceRecordURL: item.SourceRecordURL, CanonicalURL: item.CanonicalURL, PublisherName: item.PublisherName,
		ContentOriginName: item.ContentOriginName, PublishedAt: item.PublishedAt, CapturedAt: item.CapturedAt,
		ExtractionSchemaVersion: item.ExtractionSchemaVersion, DecisionOrigin: item.DecisionOrigin, CreatedAt: item.CreatedAt}
}
