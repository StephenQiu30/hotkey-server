package http

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type ReviewContentLineageRequestDTO struct {
	ExpectedMemberVersion         int64  `json:"expected_member_version" binding:"required"`
	FeedbackType                  string `json:"feedback_type" binding:"required"`
	RelationOverride              string `json:"relation_override"`
	TargetParentDocumentVersionID int64  `json:"target_parent_document_version_id"`
	ExpectedTargetMemberVersion   int64  `json:"expected_target_member_version"`
	ReasonCode                    string `json:"reason_code" binding:"required,max=64"`
	Note                          string `json:"note" binding:"max=1000"`
}

type ContentLineageFeedbackResponseDTO struct {
	FeedbackID                      int64  `json:"feedback_id"`
	LineageDecisionID               int64  `json:"lineage_decision_id"`
	ResultLineageDecisionID         int64  `json:"result_lineage_decision_id"`
	DocumentVersionID               int64  `json:"document_version_id"`
	OriginalContentFamilyID         int64  `json:"original_content_family_id"`
	ResultContentFamilyID           int64  `json:"result_content_family_id"`
	ResultContentFamilyVersion      int64  `json:"result_content_family_version"`
	OriginalRelation                string `json:"original_relation"`
	ResultRelation                  string `json:"result_relation"`
	OriginalParentDocumentVersionID *int64 `json:"original_parent_document_version_id,omitempty"`
	ResultParentDocumentVersionID   *int64 `json:"result_parent_document_version_id,omitempty"`
	FeedbackType                    string `json:"feedback_type"`
	Reused                          bool   `json:"reused"`
}

type contentLineageFeedbackHTTPService interface {
	Review(context.Context, ingestionapplication.ReviewContentLineageCommand) (ingestionapplication.ReviewContentLineageResult, error)
}

type ContentLineageFeedbackHandler struct {
	service contentLineageFeedbackHTTPService
}

func NewContentLineageFeedbackHandler(service contentLineageFeedbackHTTPService) *ContentLineageFeedbackHandler {
	return &ContentLineageFeedbackHandler{service: service}
}

func RegisterContentLineageFeedbackRoutes(router *gin.Engine, service contentLineageFeedbackHTTPService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	group := router.Group("/api/v1/content-lineage-decisions", httptransport.RequireAuthentication(authenticator),
		httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	group.POST("/:id/feedback", httptransport.Wrap(NewContentLineageFeedbackHandler(service).Review))
}

// Review appends a manual, versioned lineage fact without rewriting the
// original automated decision.
// @Summary Review a content lineage decision
// @Tags content-lineage
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "lineage decision ID"
// @Param If-Match header string true "strong current member ETag, e.g. v1"
// @Param Idempotency-Key header string true "bounded idempotency key"
// @Param request body ReviewContentLineageRequestDTO true "lineage review"
// @Success 200 {object} ContentResult[ContentLineageFeedbackResponseDTO]
// @Success 201 {object} ContentResult[ContentLineageFeedbackResponseDTO]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 409 {object} ContentResult[EmptyResponse]
// @Router /api/v1/content-lineage-decisions/{id}/feedback [post]
func (handler *ContentLineageFeedbackHandler) Review(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	if handler == nil || handler.service == nil {
		return sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "")
	}
	subject, found := httptransport.SubjectFromContext(c)
	if !found {
		return sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	decisionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || decisionID <= 0 {
		return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "")
	}
	expectedVersion, err := documentMatchExpectedSequence(c)
	if err != nil || expectedVersion <= 0 {
		return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "")
	}
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if !contentLineageIdempotencyKeyValid(idempotencyKey) {
		return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "")
	}
	var request ReviewContentLineageRequestDTO
	if err := bindDocumentMatchJSON(c, &request); err != nil {
		return err
	}
	if request.ExpectedMemberVersion != expectedVersion {
		return sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "")
	}
	result, err := handler.service.Review(c.Request.Context(), ingestionapplication.ReviewContentLineageCommand{
		ActorUserID: subject.UserID, LineageDecisionID: decisionID, ExpectedMemberVersion: expectedVersion,
		FeedbackType: strings.TrimSpace(request.FeedbackType), RelationOverride: strings.TrimSpace(request.RelationOverride),
		TargetParentDocumentVersionID: request.TargetParentDocumentVersionID,
		ExpectedTargetMemberVersion:   request.ExpectedTargetMemberVersion,
		ReasonCode:                    strings.TrimSpace(request.ReasonCode), Note: request.Note, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return contentLineageFeedbackHTTPError(err)
	}
	feedback := result.Feedback
	response := ContentLineageFeedbackResponseDTO{FeedbackID: feedback.FeedbackID,
		LineageDecisionID: feedback.LineageDecisionID, ResultLineageDecisionID: feedback.ResultLineageDecisionID,
		DocumentVersionID: feedback.DocumentVersionID, OriginalContentFamilyID: feedback.OriginalFamilyID,
		ResultContentFamilyID: feedback.ResultFamilyID, ResultContentFamilyVersion: feedback.ResultFamilyVersion,
		OriginalRelation: feedback.OriginalRelation, ResultRelation: feedback.ResultRelation,
		OriginalParentDocumentVersionID: feedback.OriginalParentDocumentVersionID,
		ResultParentDocumentVersionID:   feedback.ResultParentDocumentVersionID,
		FeedbackType:                    feedback.FeedbackType, Reused: feedback.Reused}
	c.Header("Cache-Control", "private, no-store")
	if feedback.Reused {
		httptransport.OK(c, response)
	} else {
		httptransport.Created(c, response)
	}
	return nil
}

func contentLineageIdempotencyKeyValid(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len([]byte(value)) <= 96 && !strings.ContainsAny(value, "\r\n")
}

func contentLineageFeedbackHTTPError(err error) error {
	switch {
	case errors.Is(err, ingestionapplication.ErrInvalidContentFamilyContract):
		return sharederrors.Wrap(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "", err)
	case errors.Is(err, ingestionapplication.ErrContentLineageFeedbackDenied):
		return sharederrors.Wrap(sharederrors.CodeForbidden, stdhttp.StatusForbidden, "", err)
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.Wrap(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "", err)
	case errors.Is(err, sharedrepository.ErrConflict):
		return sharederrors.Wrap(sharederrors.CodeConflict, stdhttp.StatusConflict, "", err)
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.Wrap(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "", err)
	default:
		return fmt.Errorf("review content lineage: %w", err)
	}
}
