package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	read         *application.ReadService
	lifecycle    *application.LifecycleService
	governance   *application.GovernanceService
	heat         *application.HeatService
	claims       *application.ClaimService
	intelligence eventIntelligenceReader
	summaries    eventSummaryGenerator
	extractions  eventClaimExtractor
}

type eventIntelligenceReader interface {
	Read(context.Context, int64) (application.EventIntelligenceReadResult, error)
}

type eventSummaryGenerator interface {
	Generate(context.Context, int64) (application.EventSummaryGenerationResult, error)
}

type eventClaimExtractor interface {
	Extract(context.Context, int64) (application.EventClaimExtractionResult, error)
}

func NewHandler(read *application.ReadService, lifecycle *application.LifecycleService, governance *application.GovernanceService) *Handler {
	return &Handler{read: read, lifecycle: lifecycle, governance: governance}
}

func NewHandlerWithHeat(read *application.ReadService, lifecycle *application.LifecycleService, governance *application.GovernanceService, heat *application.HeatService) *Handler {
	handler := NewHandler(read, lifecycle, governance)
	handler.heat = heat
	return handler
}

func NewHandlerWithHeatAndClaims(read *application.ReadService, lifecycle *application.LifecycleService, governance *application.GovernanceService, heat *application.HeatService, claims *application.ClaimService) *Handler {
	handler := NewHandlerWithHeat(read, lifecycle, governance, heat)
	handler.claims = claims
	return handler
}

func NewHandlerWithIntelligence(read *application.ReadService, lifecycle *application.LifecycleService, governance *application.GovernanceService, heat *application.HeatService, claims *application.ClaimService, intelligence eventIntelligenceReader, summaries eventSummaryGenerator, extractions eventClaimExtractor) *Handler {
	handler := NewHandlerWithHeatAndClaims(read, lifecycle, governance, heat, claims)
	handler.intelligence, handler.summaries, handler.extractions = intelligence, summaries, extractions
	return handler
}

// GetIntelligence returns only the Event-owned, evidence-backed entity and
// claim projection. It never exposes model prompts, provider payloads, or AI
// run implementation details.
// @Summary Get verified event intelligence
// @Tags events
// @Produce json
// @Security BearerAuth
// @Param id path int true "event ID"
// @Success 200 {object} EventResult[EventIntelligenceResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 404 {object} EventResult[EmptyResponse]
// @Failure 503 {object} EventResult[EmptyResponse]
// @Router /api/v1/events/{id}/intelligence [get]
func (handler *Handler) GetIntelligence(c *gin.Context) error {
	if handler == nil || handler.intelligence == nil {
		return sharederrors.New(sharederrors.CodeUnavailable, http.StatusServiceUnavailable, "")
	}
	eventID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	result, err := handler.intelligence.Read(c.Request.Context(), eventID)
	if err != nil {
		return eventError(err)
	}
	response := EventIntelligenceResponse{EventID: result.EventID, Entities: make([]IntelligenceEntityResponse, 0, len(result.Entities)), Claims: make([]IntelligenceClaimResponse, 0, len(result.Claims))}
	for _, item := range result.Entities {
		response.Entities = append(response.Entities, IntelligenceEntityResponse{EntityID: item.Entity.ID, EntityVersion: item.Entity.Version, EntityKey: item.Entity.Key, EntityType: string(item.Entity.Type), CanonicalName: item.Entity.Name, EntityLocked: item.Entity.ManualLocked, RelationID: item.EventEntity.ID, RelationVersion: item.EventEntity.Version, Role: item.EventEntity.Role, Confidence: item.EventEntity.Confidence, Origin: string(item.EventEntity.Origin), Confirmed: item.EventEntity.Confirmed})
	}
	for _, claim := range result.Claims {
		response.Claims = append(response.Claims, intelligenceClaimResponse(claim))
	}
	httptransport.OK(c, response)
	return nil
}

// RegenerateSummary creates an evidence-bound summary or returns a safe
// representative-content fallback when model execution is unavailable.
// @Summary Regenerate event summary
// @Tags events
// @Produce json
// @Security BearerAuth
// @Param id path int true "event ID"
// @Success 200 {object} EventResult[SummaryRegenerationResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 403 {object} EventResult[EmptyResponse]
// @Failure 404 {object} EventResult[EmptyResponse]
// @Failure 409 {object} EventResult[EmptyResponse]
// @Failure 503 {object} EventResult[EmptyResponse]
// @Router /api/v1/events/{id}/intelligence/summary/regenerate [post]
func (handler *Handler) RegenerateSummary(c *gin.Context) error {
	if handler == nil || handler.summaries == nil {
		return sharederrors.New(sharederrors.CodeUnavailable, http.StatusServiceUnavailable, "")
	}
	eventID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	result, err := handler.summaries.Generate(c.Request.Context(), eventID)
	if err != nil {
		return eventError(err)
	}
	httptransport.OK(c, SummaryRegenerationResponse{EventID: eventID, Status: regenerationStatus(result.Summary.Degraded), ReasonCode: result.ReasonCode, RunID: result.RunID, Reused: result.Reused, Summary: eventSummaryResponse(result.Summary)})
	return nil
}

// RegenerateExtraction creates only unconfirmed, evidence-backed Event facts.
// Provider unavailability returns a degradation result without mutating facts.
// @Summary Regenerate event entities and claims
// @Tags events
// @Produce json
// @Security BearerAuth
// @Param id path int true "event ID"
// @Success 200 {object} EventResult[ExtractionRegenerationResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 403 {object} EventResult[EmptyResponse]
// @Failure 404 {object} EventResult[EmptyResponse]
// @Failure 409 {object} EventResult[EmptyResponse]
// @Failure 503 {object} EventResult[EmptyResponse]
// @Router /api/v1/events/{id}/intelligence/extract [post]
func (handler *Handler) RegenerateExtraction(c *gin.Context) error {
	if handler == nil || handler.extractions == nil {
		return sharederrors.New(sharederrors.CodeUnavailable, http.StatusServiceUnavailable, "")
	}
	eventID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	result, err := handler.extractions.Extract(c.Request.Context(), eventID)
	if err != nil {
		return eventError(err)
	}
	httptransport.OK(c, ExtractionRegenerationResponse{EventID: eventID, Status: result.Status, ReasonCode: result.ReasonCode, RunID: result.RunID, Reused: result.Reused, EntityCount: len(result.Facts.Entities), ClaimCount: len(result.Facts.Claims)})
	return nil
}

// GetHeat returns the latest versioned heat snapshot and its evidence-set hash.
// @Summary Get latest event heat
// @Tags events
// @Produce json
// @Security BearerAuth
// @Param id path int true "event ID"
// @Success 200 {object} EventResult[HeatResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 404 {object} EventResult[EmptyResponse]
// @Failure 503 {object} EventResult[EmptyResponse]
// @Router /api/v1/events/{id}/heat [get]
func (handler *Handler) GetHeat(c *gin.Context) error {
	if handler == nil || handler.heat == nil {
		return sharederrors.New(sharederrors.CodeUnavailable, http.StatusServiceUnavailable, "")
	}
	eventID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	result, err := handler.heat.Latest(c.Request.Context(), eventID)
	if err != nil {
		return eventError(err)
	}
	httptransport.OK(c, HeatResponse{EventID: result.EventID, HeatScore: result.HeatScore, TrendScore: result.TrendScore, TrendStatus: string(result.TrendStatus), SourceCount: result.SourceCount, ContentCount: result.ContentCount, WindowHours: result.WindowHours, HeatVersion: result.HeatVersion, EvidenceSetHash: result.EvidenceSetHash, CapabilityProfileSetHash: result.CapabilityProfileSetHash, ReasonCodes: result.ReasonCodes, CapturedAt: result.WindowEnd})
	return nil
}

// SaveClaim records an evidence-backed claim after the repository verifies
// that every referenced content item is still active in the event.
// @Summary Save an event claim
// @Tags events
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "event ID"
// @Param request body ClaimRequest true "claim request"
// @Success 200 {object} EventResult[ClaimResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 403 {object} EventResult[EmptyResponse]
// @Failure 404 {object} EventResult[EmptyResponse]
// @Failure 409 {object} EventResult[EmptyResponse]
// @Failure 503 {object} EventResult[EmptyResponse]
// @Router /api/v1/events/{id}/claims [post]
func (handler *Handler) SaveClaim(c *gin.Context) error {
	if handler == nil || handler.claims == nil {
		return sharederrors.New(sharederrors.CodeUnavailable, http.StatusServiceUnavailable, "")
	}
	eventID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var request ClaimRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	evidence := make([]domain.ClaimEvidence, 0, len(request.Evidence))
	for _, item := range request.Evidence {
		evidence = append(evidence, domain.ClaimEvidence{EvidenceRef: domain.EvidenceRef{ContentID: item.ContentID, Locator: item.Locator, Excerpt: item.Excerpt}, Stance: item.Stance, Confidence: item.Confidence})
	}
	claim, err := handler.claims.Save(c.Request.Context(), domain.Claim{ID: request.ID, Version: request.Version, EventID: eventID, NormalizedClaim: request.NormalizedClaim, ClaimHash: request.ClaimHash, Status: domain.ClaimStatus(request.Status), Confidence: request.Confidence, ManualLocked: request.ManualLocked, Evidence: evidence})
	if err != nil {
		return eventError(err)
	}
	httptransport.OK(c, ClaimResponse{ID: claim.ID, Version: claim.Version, EventID: claim.EventID, NormalizedClaim: claim.NormalizedClaim, ClaimHash: claim.ClaimHash, Status: string(claim.Status), Confidence: claim.Confidence})
	return nil
}

// List returns safe Event projections for authenticated users.
// @Summary List events
// @Tags events
// @Produce json
// @Security BearerAuth
// @Param cursor query int false "event id cursor"
// @Param limit query int false "page size"
// @Success 200 {object} EventResult[EventPageResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 503 {object} EventResult[EmptyResponse]
// @Router /api/v1/events [get]
func (handler *Handler) List(c *gin.Context) error {
	limit, err := queryLimit(c.Query("limit"))
	if err != nil {
		return err
	}
	cursor, err := queryCursor(c.Query("cursor"))
	if err != nil {
		return err
	}
	page, err := handler.read.List(c.Request.Context(), domain.EventListQuery{Limit: limit, Cursor: cursor})
	if err != nil {
		return eventError(err)
	}
	response := EventPageResponse{Items: make([]EventResponse, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, event := range page.Items {
		response.Items = append(response.Items, eventResponse(event))
	}
	httptransport.OK(c, response)
	return nil
}

// Get returns one safe Event projection.
// @Summary Get event
// @Tags events
// @Produce json
// @Security BearerAuth
// @Param id path int true "event ID"
// @Success 200 {object} EventResult[EventResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 404 {object} EventResult[EmptyResponse]
// @Router /api/v1/events/{id} [get]
func (handler *Handler) Get(c *gin.Context) error {
	eventID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	event, err := handler.read.Get(c.Request.Context(), eventID)
	if err != nil {
		return eventError(err)
	}
	httptransport.OK(c, eventResponse(event))
	return nil
}

// ListMembers returns safe Event evidence membership facts without raw content.
// @Summary List event contents
// @Tags events
// @Produce json
// @Security BearerAuth
// @Param id path int true "event ID"
// @Success 200 {object} EventResult[EventMemberPageResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 404 {object} EventResult[EmptyResponse]
// @Router /api/v1/events/{id}/contents [get]
func (handler *Handler) ListMembers(c *gin.Context) error {
	eventID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	page, err := handler.read.ListMembers(c.Request.Context(), eventID)
	if err != nil {
		return eventError(err)
	}
	response := EventMemberPageResponse{Items: make([]EventMemberResponse, 0, len(page.Items))}
	for _, member := range page.Items {
		response.Items = append(response.Items, memberResponse(member))
	}
	httptransport.OK(c, response)
	return nil
}

// SetMemberLock locks or unlocks one Event membership with its own version.
// @Summary Lock an event member
// @Tags events
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "event ID"
// @Param content_id path int true "content ID"
// @Param request body MemberLockRequest true "member lock command"
// @Success 200 {object} EventResult[EventMemberResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 403 {object} EventResult[EmptyResponse]
// @Failure 404 {object} EventResult[EmptyResponse]
// @Failure 409 {object} EventResult[EmptyResponse]
// @Router /api/v1/events/{id}/contents/{content_id}/lock [post]
func (handler *Handler) SetMemberLock(c *gin.Context) error {
	eventID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	contentID, err := pathID(c, "content_id")
	if err != nil {
		return err
	}
	var request MemberLockRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return sharederrors.New(sharederrors.CodeUnauthenticated, http.StatusUnauthorized, "")
	}
	member, err := handler.governance.SetMemberLock(c.Request.Context(), application.MemberLockCommand{EventID: eventID, ContentID: contentID, ExpectedVersion: request.ExpectedVersion, Locked: request.Locked, ActorUserID: &subject.UserID, ReasonCode: request.Reason})
	if err != nil {
		return eventError(err)
	}
	httptransport.OK(c, memberResponse(member))
	return nil
}

// Transition changes an Event lifecycle state with optimistic locking.
// @Summary Change event lifecycle
// @Tags events
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "event ID"
// @Param request body LifecycleRequest true "lifecycle command"
// @Success 200 {object} EventResult[EventResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 403 {object} EventResult[EmptyResponse]
// @Failure 404 {object} EventResult[EmptyResponse]
// @Failure 409 {object} EventResult[EmptyResponse]
// @Router /api/v1/events/{id}/lifecycle [post]
func (handler *Handler) Transition(c *gin.Context) error {
	eventID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var request LifecycleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return sharederrors.New(sharederrors.CodeUnauthenticated, http.StatusUnauthorized, "")
	}
	event, err := handler.lifecycle.Transition(c.Request.Context(), application.LifecycleInput{EventID: eventID, ExpectedVersion: request.ExpectedVersion, To: domain.LifecycleStatus(request.To), ActorUserID: &subject.UserID, ReasonCode: request.Reason})
	if err != nil {
		return eventError(err)
	}
	httptransport.OK(c, eventResponse(event))
	return nil
}

// Merge transactionally redirects one Event to another canonical Event.
// @Summary Merge events
// @Tags events
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "source event ID"
// @Param request body MergeRequest true "merge command"
// @Success 200 {object} EventResult[EventResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 403 {object} EventResult[EmptyResponse]
// @Failure 404 {object} EventResult[EmptyResponse]
// @Failure 409 {object} EventResult[EmptyResponse]
// @Router /api/v1/events/{id}/merge [post]
func (handler *Handler) Merge(c *gin.Context) error {
	sourceID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var request MergeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return sharederrors.New(sharederrors.CodeUnauthenticated, http.StatusUnauthorized, "")
	}
	event, err := handler.governance.Merge(c.Request.Context(), application.MergeCommand{SourceEventID: sourceID, TargetEventID: request.TargetEventID, SourceExpectedVersion: request.SourceExpectedVersion, TargetExpectedVersion: request.TargetExpectedVersion, ActorUserID: &subject.UserID, ReasonCode: request.Reason})
	if err != nil {
		return eventError(err)
	}
	httptransport.OK(c, eventResponse(event))
	return nil
}

// Split creates a new Event and moves selected unlocked members atomically.
// @Summary Split event
// @Tags events
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "source event ID"
// @Param request body SplitRequest true "split command"
// @Success 200 {object} EventResult[EventResponse]
// @Failure 400 {object} EventResult[EmptyResponse]
// @Failure 401 {object} EventResult[EmptyResponse]
// @Failure 403 {object} EventResult[EmptyResponse]
// @Failure 404 {object} EventResult[EmptyResponse]
// @Failure 409 {object} EventResult[EmptyResponse]
// @Router /api/v1/events/{id}/split [post]
func (handler *Handler) Split(c *gin.Context) error {
	sourceID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var request SplitRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return sharederrors.New(sharederrors.CodeUnauthenticated, http.StatusUnauthorized, "")
	}
	members := make([]application.SplitMember, 0, len(request.Members))
	for _, member := range request.Members {
		members = append(members, application.SplitMember{ContentID: member.ContentID, ExpectedVersion: member.ExpectedVersion})
	}
	event, err := handler.governance.Split(c.Request.Context(), application.SplitCommand{SourceEventID: sourceID, SourceExpectedVersion: request.SourceExpectedVersion, Members: members, ActorUserID: &subject.UserID, ReasonCode: request.Reason})
	if err != nil {
		return eventError(err)
	}
	httptransport.OK(c, eventResponse(event))
	return nil
}

func pathID(c *gin.Context, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, invalidRequest(fmt.Errorf("invalid event identifier"))
	}
	return id, nil
}

func queryLimit(raw string) (int, error) {
	if raw == "" {
		return 50, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 100 {
		return 0, invalidRequest(fmt.Errorf("invalid event limit"))
	}
	return value, nil
}

func queryCursor(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, invalidRequest(fmt.Errorf("invalid event cursor"))
	}
	return value, nil
}

func invalidRequest(cause error) error {
	return sharederrors.Wrap(sharederrors.CodeInvalidRequest, http.StatusBadRequest, "", cause)
}

func eventError(err error) error {
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	switch {
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.Wrap(sharederrors.CodeNotFound, http.StatusNotFound, "", err)
	case errors.Is(err, sharedrepository.ErrConflict), errors.Is(err, sharedrepository.ErrImmutable):
		return sharederrors.Wrap(sharederrors.CodeConflict, http.StatusConflict, "", err)
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		return sharederrors.Wrap(sharederrors.CodeInvalidRequest, http.StatusBadRequest, "", err)
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.Wrap(sharederrors.CodeUnavailable, http.StatusServiceUnavailable, "", err)
	default:
		return sharederrors.Wrap(sharederrors.CodeInternal, http.StatusInternalServerError, "", err)
	}
}

func intelligenceClaimResponse(claim domain.Claim) IntelligenceClaimResponse {
	response := IntelligenceClaimResponse{ID: claim.ID, Version: claim.Version, NormalizedClaim: claim.NormalizedClaim, ClaimHash: claim.ClaimHash, Status: string(claim.Status), Confidence: claim.Confidence, ManualLocked: claim.ManualLocked, Evidence: make([]IntelligenceEvidenceResponse, 0, len(claim.Evidence))}
	for _, item := range claim.Evidence {
		response.Evidence = append(response.Evidence, IntelligenceEvidenceResponse{ContentID: item.ContentID, Locator: item.Locator, Excerpt: item.Excerpt, Stance: item.Stance, Confidence: item.Confidence})
	}
	return response
}

func eventSummaryResponse(summary domain.EventSummary) EventSummaryResponse {
	response := EventSummaryResponse{Version: summary.Version, TitleZH: summary.TitleZH, TitleEN: summary.TitleEN, Degraded: summary.Degraded, Sentences: make([]SummarySentenceResponse, 0, len(summary.Sentences))}
	for _, sentence := range summary.Sentences {
		item := SummarySentenceResponse{Text: sentence.Text, Evidence: make([]IntelligenceEvidenceResponse, 0, len(sentence.Evidence))}
		for _, evidence := range sentence.Evidence {
			item.Evidence = append(item.Evidence, IntelligenceEvidenceResponse{ContentID: evidence.ContentID, Locator: evidence.Locator, Excerpt: evidence.Excerpt})
		}
		response.Sentences = append(response.Sentences, item)
	}
	return response
}

func regenerationStatus(degraded bool) string {
	if degraded {
		return "degraded"
	}
	return "succeeded"
}
