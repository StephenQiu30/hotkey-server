package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type relevanceHTTPService interface {
	ListMatches(context.Context, int64, ingestiondomain.RelevanceSnapshotListQuery) (ingestiondomain.RelevanceSnapshotPage, error)
	GetMatch(context.Context, int64, int64) (ingestionapplication.RelevanceMatchDetail, error)
	Preview(context.Context, int64) ([]ingestionapplication.RelevancePreviewItem, error)
	UpsertMatchFeedback(context.Context, int64, int64, int64, ingestiondomain.FeedbackType, *int64) (ingestiondomain.RelevanceFeedback, error)
	UpsertFalseNegativeContentFeedback(context.Context, int64, int64, int64, *int64) (ingestiondomain.RelevanceFeedback, error)
	Evaluations(context.Context, int64) ([]ingestiondomain.RelevanceEvaluation, error)
	RefreshSuggestions(context.Context, int64) (int, error)
	ListSuggestions(context.Context, int64, ingestiondomain.RelevanceSuggestionListQuery) (ingestiondomain.RelevanceSuggestionPage, error)
	ReviewSuggestion(context.Context, int64, int64, int64, int64, ingestiondomain.SuggestionStatus) (ingestiondomain.RelevanceSuggestion, error)
}

type RelevanceHandler struct{ service relevanceHTTPService }

func NewRelevanceHandler(service relevanceHTTPService) *RelevanceHandler {
	return &RelevanceHandler{service: service}
}

// ListMatches returns the current active Content match for each configuration
// version, ordered by score. It returns only the safe explanation allowlist.
// @Summary List relevance matches
// @Tags relevance
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param decision query string false "accepted, rejected, or review"
// @Param cursor query string false "opaque cursor"
// @Param limit query int false "page size, 1-100"
// @Success 200 {object} ContentResult[RelevanceMatchPageResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/matches [get]
func (handler *RelevanceHandler) ListMatches(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	monitorID, err := relevancePathID(c, "id")
	if err != nil {
		return err
	}
	query, err := relevanceMatchListQuery(c)
	if err != nil {
		return err
	}
	page, err := handler.service.ListMatches(c.Request.Context(), monitorID, query)
	if err != nil {
		return err
	}
	response := RelevanceMatchPageResponse{Items: make([]RelevanceMatchResponse, 0, len(page.Items))}
	for _, snapshot := range page.Items {
		response.Items = append(response.Items, relevanceMatchResponse(snapshot))
	}
	response.NextCursor = encodeMatchCursor(page.Next)
	httptransport.OK(c, response)
	return nil
}

// GetMatch returns one monitor-owned match and a compact safe Content summary.
// @Summary Get a relevance explanation
// @Tags relevance
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param match_id path int true "match ID"
// @Success 200 {object} ContentResult[RelevanceMatchDetailResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/matches/{match_id} [get]
func (handler *RelevanceHandler) GetMatch(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	monitorID, err := relevancePathID(c, "id")
	if err != nil {
		return err
	}
	matchID, err := relevancePathID(c, "match_id")
	if err != nil {
		return err
	}
	detail, err := handler.service.GetMatch(c.Request.Context(), monitorID, matchID)
	if err != nil {
		return err
	}
	httptransport.OK(c, RelevanceMatchDetailResponse{Match: relevanceMatchResponse(detail.Snapshot), Content: relevanceContentResponse(detail.Content)})
	return nil
}

// Preview deterministically scores up to twenty active Content items without
// persisting snapshots or invoking a relevance-review provider.
// @Summary Preview relevance scoring without writes
// @Tags relevance
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Success 200 {object} ContentResult[[]RelevancePreviewItemResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/relevance-preview [post]
func (handler *RelevanceHandler) Preview(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	monitorID, err := relevancePathID(c, "id")
	if err != nil {
		return err
	}
	items, err := handler.service.Preview(c.Request.Context(), monitorID)
	if err != nil {
		return err
	}
	response := make([]RelevancePreviewItemResponse, 0, len(items))
	for _, item := range items {
		response = append(response, relevancePreviewItemResponse(item))
	}
	httptransport.OK(c, response)
	return nil
}

// UpsertMatchFeedback records one feedback fact under explicit optimistic
// concurrency. The initial write must send expected_feedback_version: null.
// @Summary Upsert feedback for a relevance match
// @Tags relevance
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param match_id path int true "match ID"
// @Param request body RelevanceFeedbackRequest true "feedback and expected version"
// @Success 200 {object} ContentResult[RelevanceFeedbackResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 409 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/matches/{match_id}/feedback [put]
func (handler *RelevanceHandler) UpsertMatchFeedback(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	actorID, err := relevanceActorID(c)
	if err != nil {
		return err
	}
	monitorID, err := relevancePathID(c, "id")
	if err != nil {
		return err
	}
	matchID, err := relevancePathID(c, "match_id")
	if err != nil {
		return err
	}
	feedbackType, expectedVersion, err := relevanceFeedbackRequest(c)
	if err != nil {
		return err
	}
	feedback, err := handler.service.UpsertMatchFeedback(c.Request.Context(), actorID, monitorID, matchID, feedbackType, expectedVersion)
	if err != nil {
		return err
	}
	httptransport.OK(c, relevanceFeedbackResponse(feedback))
	return nil
}

// UpsertContentFeedback records a false-negative fact for an active Content
// that has no relevance snapshot in the monitor's current published config.
// @Summary Record unmatched-content false-negative feedback
// @Tags relevance
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param content_id path int true "content ID"
// @Param request body RelevanceFalseNegativeFeedbackRequest true "expected version; feedback type is fixed to false_negative"
// @Success 200 {object} ContentResult[RelevanceFeedbackResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 409 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/contents/{content_id}/feedback [put]
func (handler *RelevanceHandler) UpsertContentFeedback(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	actorID, err := relevanceActorID(c)
	if err != nil {
		return err
	}
	monitorID, err := relevancePathID(c, "id")
	if err != nil {
		return err
	}
	contentID, err := relevancePathID(c, "content_id")
	if err != nil {
		return err
	}
	expectedVersion, err := relevanceFalseNegativeFeedbackRequest(c)
	if err != nil {
		return err
	}
	feedback, err := handler.service.UpsertFalseNegativeContentFeedback(c.Request.Context(), actorID, monitorID, contentID, expectedVersion)
	if err != nil {
		return err
	}
	httptransport.OK(c, relevanceFeedbackResponse(feedback))
	return nil
}

// Evaluations returns feedback-derived quality metrics for administrators.
// @Summary Get relevance feedback evaluation
// @Tags relevance
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Success 200 {object} ContentResult[[]RelevanceEvaluationResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/feedback/evaluation [get]
func (handler *RelevanceHandler) Evaluations(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	monitorID, err := relevancePathID(c, "id")
	if err != nil {
		return err
	}
	values, err := handler.service.Evaluations(c.Request.Context(), monitorID)
	if err != nil {
		return err
	}
	response := make([]RelevanceEvaluationResponse, 0, len(values))
	for _, value := range values {
		response = append(response, relevanceEvaluationResponse(value))
	}
	httptransport.OK(c, response)
	return nil
}

// RefreshSuggestions derives reviewable suggestions from existing feedback.
// It never edits monitor rules or published configurations.
// @Summary Refresh feedback suggestions
// @Tags relevance
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Success 200 {object} ContentResult[RelevanceRefreshResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/feedback/suggestions/refresh [post]
func (handler *RelevanceHandler) RefreshSuggestions(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	monitorID, err := relevancePathID(c, "id")
	if err != nil {
		return err
	}
	count, err := handler.service.RefreshSuggestions(c.Request.Context(), monitorID)
	if err != nil {
		return err
	}
	httptransport.OK(c, RelevanceRefreshResponse{SuggestionCount: count})
	return nil
}

// ListSuggestions exposes administrator-only, explicitly reviewed suggestions.
// @Summary List relevance feedback suggestions
// @Tags relevance
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param status query string false "pending, approved, or rejected"
// @Param cursor query string false "opaque cursor"
// @Param limit query int false "page size, 1-100"
// @Success 200 {object} ContentResult[RelevanceSuggestionPageResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/feedback/suggestions [get]
func (handler *RelevanceHandler) ListSuggestions(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	monitorID, err := relevancePathID(c, "id")
	if err != nil {
		return err
	}
	query, err := relevanceSuggestionListQuery(c)
	if err != nil {
		return err
	}
	page, err := handler.service.ListSuggestions(c.Request.Context(), monitorID, query)
	if err != nil {
		return err
	}
	response := RelevanceSuggestionPageResponse{Items: make([]RelevanceSuggestionResponse, 0, len(page.Items)), NextCursor: encodeSuggestionCursor(page.Next)}
	for _, value := range page.Items {
		response.Items = append(response.Items, relevanceSuggestionResponse(value))
	}
	httptransport.OK(c, response)
	return nil
}

// ReviewSuggestion approves or rejects a pending suggestion with a version.
// @Summary Review a relevance feedback suggestion
// @Tags relevance
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param suggestion_id path int true "suggestion ID"
// @Param request body RelevanceSuggestionReviewRequest true "expected version and decision"
// @Success 200 {object} ContentResult[RelevanceSuggestionResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 409 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/feedback/suggestions/{suggestion_id}/review [post]
func (handler *RelevanceHandler) ReviewSuggestion(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	actorID, err := relevanceActorID(c)
	if err != nil {
		return err
	}
	monitorID, err := relevancePathID(c, "id")
	if err != nil {
		return err
	}
	suggestionID, err := relevancePathID(c, "suggestion_id")
	if err != nil {
		return err
	}
	expectedVersion, status, err := relevanceSuggestionReviewRequest(c)
	if err != nil {
		return err
	}
	suggestion, err := handler.service.ReviewSuggestion(c.Request.Context(), actorID, monitorID, suggestionID, expectedVersion, status)
	if err != nil {
		return err
	}
	httptransport.OK(c, relevanceSuggestionResponse(suggestion))
	return nil
}

// RelevanceFeedbackRequest requires expected_feedback_version to be present:
// null creates the first feedback fact; a positive value updates it.
type RelevanceFeedbackRequest struct {
	FeedbackType            string `json:"feedback_type"`
	ExpectedFeedbackVersion *int64 `json:"expected_feedback_version" extensions:"x-nullable"`
	expectedVersionSet      bool
}

type RelevanceSuggestionReviewRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	Status          string `json:"status"`
}

func relevanceFeedbackRequest(c *gin.Context) (ingestiondomain.FeedbackType, *int64, error) {
	var request RelevanceFeedbackRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return "", nil, invalidRequest(err)
	}
	feedbackType := ingestiondomain.FeedbackType(request.FeedbackType)
	if !feedbackType.Valid() {
		return "", nil, invalidRequest(fmt.Errorf("invalid feedback type"))
	}
	if !request.expectedVersionSet {
		return "", nil, invalidRequest(fmt.Errorf("expected feedback version is required"))
	}
	if request.ExpectedFeedbackVersion != nil && *request.ExpectedFeedbackVersion <= 0 {
		return "", nil, invalidRequest(fmt.Errorf("invalid expected feedback version"))
	}
	return feedbackType, request.ExpectedFeedbackVersion, nil
}

func relevanceFalseNegativeFeedbackRequest(c *gin.Context) (*int64, error) {
	var request RelevanceFalseNegativeFeedbackRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return nil, invalidRequest(err)
	}
	if !request.expectedVersionSet {
		return nil, invalidRequest(fmt.Errorf("expected feedback version is required"))
	}
	if request.ExpectedFeedbackVersion != nil && *request.ExpectedFeedbackVersion <= 0 {
		return nil, invalidRequest(fmt.Errorf("invalid expected feedback version"))
	}
	return request.ExpectedFeedbackVersion, nil
}

func (request *RelevanceFeedbackRequest) UnmarshalJSON(data []byte) error {
	type payload struct {
		FeedbackType            string `json:"feedback_type"`
		ExpectedFeedbackVersion *int64 `json:"expected_feedback_version"`
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var decoded payload
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	_, request.expectedVersionSet = raw["expected_feedback_version"]
	request.FeedbackType = decoded.FeedbackType
	request.ExpectedFeedbackVersion = decoded.ExpectedFeedbackVersion
	return nil
}

func relevanceSuggestionReviewRequest(c *gin.Context) (int64, ingestiondomain.SuggestionStatus, error) {
	var request RelevanceSuggestionReviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return 0, "", invalidRequest(err)
	}
	status := ingestiondomain.SuggestionStatus(request.Status)
	if request.ExpectedVersion <= 0 || (status != ingestiondomain.SuggestionStatusApproved && status != ingestiondomain.SuggestionStatusRejected) {
		return 0, "", invalidRequest(fmt.Errorf("invalid suggestion review"))
	}
	return request.ExpectedVersion, status, nil
}

func relevanceMatchListQuery(c *gin.Context) (ingestiondomain.RelevanceSnapshotListQuery, error) {
	query := ingestiondomain.RelevanceSnapshotListQuery{Limit: 20}
	if raw := c.Query("limit"); raw != "" {
		limit, err := relevanceLimit(raw)
		if err != nil {
			return ingestiondomain.RelevanceSnapshotListQuery{}, err
		}
		query.Limit = limit
	}
	if raw := c.Query("decision"); raw != "" {
		decision := ingestiondomain.MatchDecision(raw)
		if !decision.Valid() {
			return ingestiondomain.RelevanceSnapshotListQuery{}, invalidRequest(fmt.Errorf("invalid match decision"))
		}
		query.Decision = &decision
	}
	if raw := c.Query("cursor"); raw != "" {
		cursor, err := decodeMatchCursor(raw)
		if err != nil {
			return ingestiondomain.RelevanceSnapshotListQuery{}, err
		}
		query.Cursor = cursor
	}
	return query, nil
}

func relevanceSuggestionListQuery(c *gin.Context) (ingestiondomain.RelevanceSuggestionListQuery, error) {
	query := ingestiondomain.RelevanceSuggestionListQuery{Limit: 20}
	if raw := c.Query("limit"); raw != "" {
		limit, err := relevanceLimit(raw)
		if err != nil {
			return ingestiondomain.RelevanceSuggestionListQuery{}, err
		}
		query.Limit = limit
	}
	if raw := c.Query("status"); raw != "" {
		status := ingestiondomain.SuggestionStatus(raw)
		if !status.Valid() {
			return ingestiondomain.RelevanceSuggestionListQuery{}, invalidRequest(fmt.Errorf("invalid suggestion status"))
		}
		query.Status = &status
	}
	if raw := c.Query("cursor"); raw != "" {
		cursor, err := decodeSuggestionCursor(raw)
		if err != nil {
			return ingestiondomain.RelevanceSuggestionListQuery{}, err
		}
		query.Cursor = cursor
	}
	return query, nil
}

func relevanceLimit(raw string) (int, error) {
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		return 0, invalidRequest(fmt.Errorf("limit must be 1-100"))
	}
	return limit, nil
}

func relevancePathID(c *gin.Context, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, invalidRequest(fmt.Errorf("invalid %s", name))
	}
	return id, nil
}

func relevanceActorID(c *gin.Context) (int64, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return 0, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	return subject.UserID, nil
}

type relevanceMatchCursorPayload struct {
	FinalScore float64 `json:"final_score"`
	ID         int64   `json:"id"`
}

type relevanceSuggestionCursorPayload struct {
	UpdatedAt string `json:"updated_at"`
	ID        int64  `json:"id"`
}

func encodeMatchCursor(cursor *ingestiondomain.RelevanceSnapshotCursor) string {
	if cursor == nil {
		return ""
	}
	return encodeRelevanceCursor(relevanceMatchCursorPayload{FinalScore: cursor.FinalScore, ID: cursor.ID})
}

func decodeMatchCursor(raw string) (*ingestiondomain.RelevanceSnapshotCursor, error) {
	var payload relevanceMatchCursorPayload
	if err := decodeRelevanceCursor(raw, &payload); err != nil || payload.ID <= 0 || payload.FinalScore < 0 || payload.FinalScore > 100 {
		return nil, invalidRequest(fmt.Errorf("invalid relevance match cursor"))
	}
	return &ingestiondomain.RelevanceSnapshotCursor{FinalScore: payload.FinalScore, ID: payload.ID}, nil
}

func encodeSuggestionCursor(cursor *ingestiondomain.RelevanceSuggestionCursor) string {
	if cursor == nil {
		return ""
	}
	return encodeRelevanceCursor(relevanceSuggestionCursorPayload{UpdatedAt: cursor.UpdatedAt.UTC().Format(time.RFC3339Nano), ID: cursor.ID})
}

func decodeSuggestionCursor(raw string) (*ingestiondomain.RelevanceSuggestionCursor, error) {
	var payload relevanceSuggestionCursorPayload
	if err := decodeRelevanceCursor(raw, &payload); err != nil || payload.ID <= 0 {
		return nil, invalidRequest(fmt.Errorf("invalid relevance suggestion cursor"))
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, payload.UpdatedAt)
	if err != nil || updatedAt.IsZero() {
		return nil, invalidRequest(fmt.Errorf("invalid relevance suggestion cursor"))
	}
	return &ingestiondomain.RelevanceSuggestionCursor{UpdatedAt: updatedAt.UTC(), ID: payload.ID}, nil
}

func encodeRelevanceCursor(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeRelevanceCursor(raw string, target any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, target)
}

// RegisterRelevanceRoutes mounts the PLAN-009 public relevance contract. The
// route groups make the viewer/editor/admin role matrix structural rather than
// a conditional hidden inside a handler.
func RegisterRelevanceRoutes(router *gin.Engine, service relevanceHTTPService, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := NewRelevanceHandler(service)
	monitor := router.Group("/api/v1/monitors/:id", httptransport.RequireAuthentication(authenticator))
	read := monitor.Group("", httptransport.RequireRoles(httptransport.RoleViewer, httptransport.RoleEditor, httptransport.RoleAdmin))
	read.GET("/matches", httptransport.Wrap(handler.ListMatches))
	read.GET("/matches/:match_id", httptransport.Wrap(handler.GetMatch))

	edit := monitor.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	edit.POST("/relevance-preview", httptransport.Wrap(handler.Preview))
	edit.PUT("/matches/:match_id/feedback", httptransport.Wrap(handler.UpsertMatchFeedback))
	edit.PUT("/contents/:content_id/feedback", httptransport.Wrap(handler.UpsertContentFeedback))

	admin := monitor.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.GET("/feedback/evaluation", httptransport.Wrap(handler.Evaluations))
	admin.POST("/feedback/suggestions/refresh", httptransport.Wrap(handler.RefreshSuggestions))
	admin.GET("/feedback/suggestions", httptransport.Wrap(handler.ListSuggestions))
	admin.POST("/feedback/suggestions/:suggestion_id/review", httptransport.Wrap(handler.ReviewSuggestion))
}
