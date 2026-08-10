package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"strconv"
	"strings"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

const maximumDocumentMatchRequestBytes = 16 * 1024

type documentMatchQueryHTTPService interface {
	List(context.Context, ingestionapplication.ListDocumentMatchesQuery) (ingestionapplication.DocumentMatchPageResult, error)
}

type documentMatchReviewHTTPService interface {
	Override(context.Context, ingestionapplication.OverrideDocumentMatchCommand) (ingestionapplication.OverrideDocumentMatchResult, error)
}

type DocumentMatchHandler struct {
	query  documentMatchQueryHTTPService
	review documentMatchReviewHTTPService
}

func NewDocumentMatchHandler(query documentMatchQueryHTTPService, review documentMatchReviewHTTPService) *DocumentMatchHandler {
	return &DocumentMatchHandler{query: query, review: review}
}

// List returns only v2 exact-version relevance facts. RRF and raw channel
// scores are explanatory retrieval signals, never probabilities or truth.
// @Summary List exact document matches for a monitor
// @Tags document-matches
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param decision query string false "effective accepted, review, or rejected decision"
// @Param cursor query string false "opaque cursor"
// @Param limit query int false "page size, 1-100"
// @Success 200 {object} ContentResult[DocumentMatchPageResponseDTO]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/document-matches [get]
func (handler *DocumentMatchHandler) List(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	actorUserID, monitorID, err := documentMatchRequestIdentity(c)
	if err != nil {
		return err
	}
	limit, err := documentMatchLimit(c.Query("limit"))
	if err != nil {
		return err
	}
	decision := strings.TrimSpace(c.Query("decision"))
	if decision != "" && decision != "accepted" && decision != "review" && decision != "rejected" {
		return documentMatchHTTPError(ingestionapplication.ErrInvalidDocumentMatchContract)
	}
	page, err := handler.query.List(c.Request.Context(), ingestionapplication.ListDocumentMatchesQuery{
		ActorUserID: actorUserID, MonitorID: monitorID, EffectiveDecision: decision,
		Cursor: strings.TrimSpace(c.Query("cursor")), Limit: limit,
	})
	if err != nil {
		return documentMatchHTTPError(err)
	}
	c.Header("Cache-Control", "private, no-store")
	httptransport.OK(c, documentMatchPageResponseDTO(page))
	return nil
}

// Override appends a manual accepted/rejected fact. If-Match is the current
// override sequence (v0 for an untouched automatic decision).
// @Summary Append a document match review decision
// @Tags document-matches
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param match_decision_id path int true "automatic match decision ID"
// @Param If-Match header string true "strong current resource ETag, e.g. v0"
// @Param Idempotency-Key header string true "bounded idempotency key"
// @Param request body OverrideDocumentMatchRequestDTO true "manual relevance decision"
// @Success 200 {object} ContentResult[OverrideDocumentMatchResponseDTO]
// @Success 201 {object} ContentResult[OverrideDocumentMatchResponseDTO]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 409 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/document-matches/{match_decision_id}/overrides [post]
func (handler *DocumentMatchHandler) Override(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	actorUserID, monitorID, err := documentMatchRequestIdentity(c)
	if err != nil {
		return err
	}
	matchDecisionID, err := documentMatchPathID(c, "match_decision_id")
	if err != nil {
		return err
	}
	expectedSequence, err := documentMatchExpectedSequence(c)
	if err != nil {
		return err
	}
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if !documentMatchIdempotencyKeyValid(idempotencyKey) {
		return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "")
	}
	var request OverrideDocumentMatchRequestDTO
	if err := bindDocumentMatchJSON(c, &request); err != nil {
		return err
	}
	result, err := handler.review.Override(c.Request.Context(), ingestionapplication.OverrideDocumentMatchCommand{
		ActorUserID: actorUserID, MonitorID: monitorID, MatchDecisionID: matchDecisionID,
		ExpectedSequence: expectedSequence, Decision: request.Decision, ReasonCode: request.ReasonCode,
		Note: request.Note, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return documentMatchHTTPError(err)
	}
	response := overrideDocumentMatchResponseDTO(result)
	c.Header("Cache-Control", "private, no-store")
	c.Header("ETag", fmt.Sprintf("\"v%d\"", response.ResourceVersion))
	if result.Reused {
		httptransport.OK(c, response)
	} else {
		httptransport.Created(c, response)
	}
	return nil
}

func documentMatchIdempotencyKeyValid(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len([]byte(value)) > 128 {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func RegisterDocumentMatchRoutes(router *gin.Engine, query documentMatchQueryHTTPService, review documentMatchReviewHTTPService, authenticator httptransport.Authenticator) {
	if router == nil || query == nil || review == nil {
		return
	}
	handler := NewDocumentMatchHandler(query, review)
	group := router.Group("/api/v1/monitors/:id/document-matches", httptransport.RequireAuthentication(authenticator))
	group.GET("", httptransport.Wrap(handler.List))
	edit := group.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	edit.POST("/:match_decision_id/overrides", httptransport.Wrap(handler.Override))
}

func documentMatchRequestIdentity(c *gin.Context) (int64, int64, error) {
	subject, found := httptransport.SubjectFromContext(c)
	if !found {
		return 0, 0, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	monitorID, err := documentMatchPathID(c, "id")
	return subject.UserID, monitorID, err
}

func documentMatchPathID(c *gin.Context, name string) (int64, error) {
	value, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || value <= 0 {
		return 0, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "")
	}
	return value, nil
}

func documentMatchLimit(raw string) (int, error) {
	if raw == "" {
		return ingestionapplication.DefaultDocumentMatchPageSize, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > ingestionapplication.MaximumDocumentMatchPageSize {
		return 0, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "")
	}
	return value, nil
}

func documentMatchExpectedSequence(c *gin.Context) (int64, error) {
	values := c.Request.Header.Values("If-Match")
	if len(values) != 1 || strings.Contains(values[0], ",") || !strings.HasPrefix(values[0], "\"v") || !strings.HasSuffix(values[0], "\"") {
		return 0, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "")
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(values[0], "\"v"), "\"")
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "")
	}
	return value, nil
}

func bindDocumentMatchJSON(c *gin.Context, target any) error {
	c.Request.Body = stdhttp.MaxBytesReader(c.Writer, c.Request.Body, maximumDocumentMatchRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return sharederrors.Wrap(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "")
	}
	return nil
}

func documentMatchHTTPError(err error) error {
	switch {
	case errors.Is(err, ingestionapplication.ErrInvalidDocumentMatchContract), errors.Is(err, sharedrepository.ErrInvalidInput):
		return sharederrors.Wrap(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "", err)
	case errors.Is(err, ingestionapplication.ErrDocumentMatchAuthorizationDenied):
		return sharederrors.Wrap(sharederrors.CodeForbidden, stdhttp.StatusForbidden, "", err)
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.Wrap(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "", err)
	case errors.Is(err, sharedrepository.ErrConflict):
		return sharederrors.Wrap(sharederrors.CodeConflict, stdhttp.StatusConflict, "", err)
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.Wrap(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "", err)
	default:
		return err
	}
}
