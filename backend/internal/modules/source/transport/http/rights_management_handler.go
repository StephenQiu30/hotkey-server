package http

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"regexp"
	"strconv"
	"strings"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

var rightsPolicyETagPattern = regexp.MustCompile(`^"v([1-9][0-9]*)"$`)

type rightsManagementHTTPService interface {
	GetSourceEndpointCapability(context.Context, sourceapplication.GetSourceEndpointCapabilityQuery) (sourceapplication.SourceEndpointCapabilityDTO, error)
	ListRightsPolicies(context.Context, sourceapplication.ListRightsPoliciesQuery) (sourceapplication.ListRightsPoliciesResult, error)
	ListRightsDecisionBatches(context.Context, sourceapplication.ListRightsDecisionBatchesQuery) (sourceapplication.ListRightsDecisionBatchesResult, error)
	GetRightsDecision(context.Context, sourceapplication.GetRightsDecisionQuery) (sourceapplication.RightsDecisionReadDTO, error)
	EvaluateRightsActionMatrix(context.Context, sourceapplication.EvaluateRightsActionMatrixQuery) (sourceapplication.RightsActionMatrixDTO, error)
	CreatePolicy(context.Context, sourceapplication.CreateRightsPolicyCommand) (sourceapplication.CreateRightsPolicyResult, error)
	RecordDecisions(context.Context, sourceapplication.RecordRightsDecisionCommand) (sourceapplication.RecordRightsDecisionResult, error)
}

type RightsManagementHandler struct {
	service rightsManagementHTTPService
}

func NewRightsManagementHandler(service rightsManagementHTTPService) *RightsManagementHandler {
	return &RightsManagementHandler{service: service}
}

// GetCapability returns connector mechanics without projecting an action allow.
// @Summary Get safe source endpoint capability
// @Tags source rights
// @Produce json
// @Security BearerAuth
// @Param id path int true "source endpoint ID"
// @Success 200 {object} SourceResult[SourceEndpointCapabilityResponseDTO]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 404 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-endpoints/{id}/capabilities [get]
func (handler *RightsManagementHandler) GetCapability(c *gin.Context) error {
	prepareRightsManagementResponse(c)
	actorID, sourceEndpointID, err := rightsManagementRequestIdentity(c)
	if err != nil {
		return err
	}
	result, err := handler.serviceOrUnavailable().GetSourceEndpointCapability(c.Request.Context(), sourceapplication.GetSourceEndpointCapabilityQuery{
		ActorID: actorID, SourceEndpointID: sourceEndpointID,
	})
	if err != nil {
		return rightsManagementReadHTTPError(err)
	}
	httptransport.OK(c, sourceEndpointCapabilityResponse(result))
	return nil
}

// ListPolicies returns immutable policy history to current administrators.
// @Summary List source endpoint rights policies
// @Tags source rights
// @Produce json
// @Security BearerAuth
// @Param id path int true "source endpoint ID"
// @Param cursor query string false "opaque cursor"
// @Param limit query int false "page size"
// @Success 200 {object} SourceResult[RightsPolicyPageResponseDTO]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 404 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-endpoints/{id}/rights-policies [get]
func (handler *RightsManagementHandler) ListPolicies(c *gin.Context) error {
	prepareRightsManagementResponse(c)
	actorID, sourceEndpointID, err := rightsManagementRequestIdentity(c)
	if err != nil {
		return err
	}
	cursor, limit, err := rightsManagementPage(c)
	if err != nil {
		return err
	}
	result, err := handler.serviceOrUnavailable().ListRightsPolicies(c.Request.Context(), sourceapplication.ListRightsPoliciesQuery{
		ActorID: actorID, SourceEndpointID: sourceEndpointID, Cursor: cursor, Limit: limit,
	})
	if err != nil {
		return rightsManagementReadHTTPError(err)
	}
	response := RightsPolicyPageResponseDTO{Items: make([]RightsPolicyResponseDTO, 0, len(result.Items)), NextCursor: result.NextCursor}
	for _, policy := range result.Items {
		response.Items = append(response.Items, rightsPolicyReadResponse(policy))
	}
	httptransport.OK(c, response)
	return nil
}

// CreatePolicy records an immutable, approved policy with durable idempotency.
// @Summary Create an immutable source rights policy
// @Tags source rights
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "source endpoint ID"
// @Param Idempotency-Key header string true "durable idempotency key"
// @Param request body CreateRightsPolicyRequestDTO true "rights policy"
// @Success 201 {object} SourceResult[CreateRightsPolicyResponseDTO]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-endpoints/{id}/rights-policies [post]
func (handler *RightsManagementHandler) CreatePolicy(c *gin.Context) error {
	prepareRightsManagementResponse(c)
	actorID, sourceEndpointID, err := rightsManagementRequestIdentity(c)
	if err != nil {
		return err
	}
	idempotencyKey, err := rightsManagementIdempotencyKey(c)
	if err != nil {
		return err
	}
	var request CreateRightsPolicyRequestDTO
	if err := bindStrictJSON(c, &request); err != nil {
		return invalidRequest(err)
	}
	result, err := handler.serviceOrUnavailable().CreatePolicy(c.Request.Context(), sourceapplication.CreateRightsPolicyCommand{
		ActorID: actorID, IdempotencyKey: idempotencyKey, SourceConnectionID: &sourceEndpointID,
		ScopeType: request.ScopeType, ScopeSubject: request.ScopeSubject, Revision: request.Revision,
		Priority: request.Priority, BasisSummary: request.BasisSummary, TermsURL: request.TermsURL,
		LicenseURI: request.LicenseURI, EffectiveFrom: request.EffectiveFrom, ExpiresAt: copyRightsTransportTime(request.ExpiresAt),
		ParentPolicyID: copyRightsTransportInt64(request.ParentPolicyID), ApprovedByUserID: copyRightsTransportInt64(request.ApprovedByUserID),
	})
	if err != nil {
		return rightsManagementMutationHTTPError(err)
	}
	c.Header("ETag", rightsPolicyETag(result.Policy.Version))
	httptransport.Created(c, CreateRightsPolicyResponseDTO{Policy: rightsPolicyResponse(result.Policy), IdempotentReplay: result.IdempotentReplay})
	return nil
}

// ListDecisionBatches returns atomic single-action decision history.
// @Summary List source endpoint rights decision batches
// @Tags source rights
// @Produce json
// @Security BearerAuth
// @Param id path int true "source endpoint ID"
// @Param cursor query string false "opaque cursor"
// @Param limit query int false "page size"
// @Success 200 {object} SourceResult[RightsDecisionBatchPageResponseDTO]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 404 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-endpoints/{id}/rights-decision-batches [get]
func (handler *RightsManagementHandler) ListDecisionBatches(c *gin.Context) error {
	prepareRightsManagementResponse(c)
	actorID, sourceEndpointID, err := rightsManagementRequestIdentity(c)
	if err != nil {
		return err
	}
	cursor, limit, err := rightsManagementPage(c)
	if err != nil {
		return err
	}
	result, err := handler.serviceOrUnavailable().ListRightsDecisionBatches(c.Request.Context(), sourceapplication.ListRightsDecisionBatchesQuery{
		ActorID: actorID, SourceEndpointID: sourceEndpointID, Cursor: cursor, Limit: limit,
	})
	if err != nil {
		return rightsManagementReadHTTPError(err)
	}
	response := RightsDecisionBatchPageResponseDTO{Items: make([]RightsDecisionBatchResponseDTO, 0, len(result.Items)), NextCursor: result.NextCursor}
	for _, batch := range result.Items {
		response.Items = append(response.Items, rightsDecisionBatchResponse(batch))
	}
	httptransport.OK(c, response)
	return nil
}

// RecordDecisionBatch atomically records one immutable decision per action.
// @Summary Record an atomic source rights decision batch
// @Tags source rights
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "source endpoint ID"
// @Param Idempotency-Key header string true "durable idempotency key"
// @Param If-Match header string true "quoted policy row version, for example v1"
// @Param request body RecordRightsDecisionBatchRequestDTO true "single-action decisions"
// @Success 201 {object} SourceResult[RecordRightsDecisionBatchResponseDTO]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-endpoints/{id}/rights-decision-batches [post]
func (handler *RightsManagementHandler) RecordDecisionBatch(c *gin.Context) error {
	prepareRightsManagementResponse(c)
	actorID, sourceEndpointID, err := rightsManagementRequestIdentity(c)
	if err != nil {
		return err
	}
	idempotencyKey, err := rightsManagementIdempotencyKey(c)
	if err != nil {
		return err
	}
	expectedVersion, err := rightsPolicyExpectedVersion(c.GetHeader("If-Match"))
	if err != nil {
		return err
	}
	var request RecordRightsDecisionBatchRequestDTO
	if err := bindStrictJSON(c, &request); err != nil {
		return invalidRequest(err)
	}
	if request.ExpectedPolicyVersion != expectedVersion {
		return sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "")
	}
	decisions := make([]sourceapplication.RightsActionDecisionDTO, 0, len(request.Decisions))
	for _, decision := range request.Decisions {
		decisions = append(decisions, sourceapplication.RightsActionDecisionDTO{
			Action: decision.Action, Decision: decision.Decision, ReasonCodes: append([]string(nil), decision.ReasonCodes...),
			Evaluator: decision.Evaluator, EvaluatedAt: decision.EvaluatedAt, EffectiveFrom: decision.EffectiveFrom,
			ExpiresAt: copyRightsTransportTime(decision.ExpiresAt), RetentionDays: copyRightsTransportInt(decision.RetentionDays),
			SupersedesDecisionID: copyRightsTransportInt64(decision.SupersedesDecisionID),
		})
	}
	result, err := handler.serviceOrUnavailable().RecordDecisions(c.Request.Context(), sourceapplication.RecordRightsDecisionCommand{
		ActorID: actorID, IdempotencyKey: idempotencyKey, SourceConnectionID: sourceEndpointID,
		PolicyID: request.PolicyID, ExpectedPolicyVersion: request.ExpectedPolicyVersion,
		SubjectType: request.SubjectType, SubjectKey: request.SubjectKey, InputDigest: request.InputDigest, Decisions: decisions,
	})
	if err != nil {
		return rightsManagementMutationHTTPError(err)
	}
	response := RecordRightsDecisionBatchResponseDTO{
		DecisionBatchID: result.DecisionBatchID, Decisions: make([]RightsDecisionResponseDTO, 0, len(result.Decisions)),
		IdempotentReplay: result.IdempotentReplay,
	}
	for _, decision := range result.Decisions {
		response.Decisions = append(response.Decisions, rightsDecisionResponse(decision))
	}
	c.Header("ETag", rightsPolicyETag(expectedVersion))
	httptransport.Created(c, response)
	return nil
}

// GetDecision returns one exact immutable decision to administrators.
// @Summary Get one source rights decision
// @Tags source rights
// @Produce json
// @Security BearerAuth
// @Param id path int true "source endpoint ID"
// @Param decision_id path int true "rights decision ID"
// @Success 200 {object} SourceResult[RightsDecisionResponseDTO]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 404 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-endpoints/{id}/rights-decisions/{decision_id} [get]
func (handler *RightsManagementHandler) GetDecision(c *gin.Context) error {
	prepareRightsManagementResponse(c)
	actorID, sourceEndpointID, err := rightsManagementRequestIdentity(c)
	if err != nil {
		return err
	}
	decisionID, err := rightsManagementPositiveID(c.Param("decision_id"), "rights decision")
	if err != nil {
		return err
	}
	result, err := handler.serviceOrUnavailable().GetRightsDecision(c.Request.Context(), sourceapplication.GetRightsDecisionQuery{
		ActorID: actorID, SourceEndpointID: sourceEndpointID, DecisionID: decisionID,
	})
	if err != nil {
		return rightsManagementReadHTTPError(err)
	}
	httptransport.OK(c, rightsDecisionReadResponse(result))
	return nil
}

// EvaluateActions accepts exact identity only in a restricted POST body and
// never echoes that identity in its action-matrix response.
// @Summary Evaluate exact current source rights actions
// @Tags source rights
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "source endpoint ID"
// @Param request body EvaluateRightsActionsRequestDTO true "exact subject evaluation"
// @Success 200 {object} SourceResult[RightsActionMatrixResponseDTO]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 404 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-endpoints/{id}/rights-evaluations [post]
func (handler *RightsManagementHandler) EvaluateActions(c *gin.Context) error {
	prepareRightsManagementResponse(c)
	actorID, sourceEndpointID, err := rightsManagementRequestIdentity(c)
	if err != nil {
		return err
	}
	var request EvaluateRightsActionsRequestDTO
	if err := bindStrictJSON(c, &request); err != nil {
		return invalidRequest(err)
	}
	result, err := handler.serviceOrUnavailable().EvaluateRightsActionMatrix(c.Request.Context(), sourceapplication.EvaluateRightsActionMatrixQuery{
		ActorID: actorID, SourceEndpointID: sourceEndpointID, SubjectType: request.SubjectType,
		SubjectKey: request.SubjectKey, InputDigest: request.InputDigest, At: request.At,
	})
	if err != nil {
		return rightsManagementReadHTTPError(err)
	}
	httptransport.OK(c, rightsActionMatrixResponse(result))
	return nil
}

func (handler *RightsManagementHandler) serviceOrUnavailable() rightsManagementHTTPService {
	if handler == nil || handler.service == nil {
		return unavailableRightsManagementHTTPService{}
	}
	return handler.service
}

type unavailableRightsManagementHTTPService struct{}

func prepareRightsManagementResponse(c *gin.Context) {
	httptransport.SetModule(c, "source")
	c.Header("Cache-Control", "private, no-store")
	c.Header("Vary", "Authorization")
	c.Header("X-Content-Type-Options", "nosniff")
}

func (unavailableRightsManagementHTTPService) GetSourceEndpointCapability(context.Context, sourceapplication.GetSourceEndpointCapabilityQuery) (sourceapplication.SourceEndpointCapabilityDTO, error) {
	return sourceapplication.SourceEndpointCapabilityDTO{}, sharedrepository.ErrUnavailable
}
func (unavailableRightsManagementHTTPService) ListRightsPolicies(context.Context, sourceapplication.ListRightsPoliciesQuery) (sourceapplication.ListRightsPoliciesResult, error) {
	return sourceapplication.ListRightsPoliciesResult{}, sharedrepository.ErrUnavailable
}
func (unavailableRightsManagementHTTPService) ListRightsDecisionBatches(context.Context, sourceapplication.ListRightsDecisionBatchesQuery) (sourceapplication.ListRightsDecisionBatchesResult, error) {
	return sourceapplication.ListRightsDecisionBatchesResult{}, sharedrepository.ErrUnavailable
}
func (unavailableRightsManagementHTTPService) GetRightsDecision(context.Context, sourceapplication.GetRightsDecisionQuery) (sourceapplication.RightsDecisionReadDTO, error) {
	return sourceapplication.RightsDecisionReadDTO{}, sharedrepository.ErrUnavailable
}
func (unavailableRightsManagementHTTPService) EvaluateRightsActionMatrix(context.Context, sourceapplication.EvaluateRightsActionMatrixQuery) (sourceapplication.RightsActionMatrixDTO, error) {
	return sourceapplication.RightsActionMatrixDTO{}, sharedrepository.ErrUnavailable
}
func (unavailableRightsManagementHTTPService) CreatePolicy(context.Context, sourceapplication.CreateRightsPolicyCommand) (sourceapplication.CreateRightsPolicyResult, error) {
	return sourceapplication.CreateRightsPolicyResult{}, sharedrepository.ErrUnavailable
}
func (unavailableRightsManagementHTTPService) RecordDecisions(context.Context, sourceapplication.RecordRightsDecisionCommand) (sourceapplication.RecordRightsDecisionResult, error) {
	return sourceapplication.RecordRightsDecisionResult{}, sharedrepository.ErrUnavailable
}

func rightsManagementRequestIdentity(c *gin.Context) (int64, int64, error) {
	subject, found := httptransport.SubjectFromContext(c)
	if !found {
		return 0, 0, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	sourceEndpointID, err := rightsManagementPositiveID(c.Param("id"), "source endpoint")
	if err != nil {
		return 0, 0, err
	}
	return subject.UserID, sourceEndpointID, nil
}

func rightsManagementPositiveID(raw, name string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, invalidRequest(fmt.Errorf("invalid %s id", name))
	}
	return value, nil
}

func rightsManagementPage(c *gin.Context) (string, int, error) {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return "", 0, invalidRequest(fmt.Errorf("invalid rights page limit"))
		}
		limit = value
	}
	return c.Query("cursor"), limit, nil
}

func rightsManagementIdempotencyKey(c *gin.Context) (string, error) {
	value := c.GetHeader("Idempotency-Key")
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return "", invalidRequest(fmt.Errorf("invalid Idempotency-Key"))
	}
	for index, character := range value {
		alphanumeric := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
		if alphanumeric || index > 0 && (character == '.' || character == '_' || character == '-' || character == ':') {
			continue
		}
		return "", invalidRequest(fmt.Errorf("invalid Idempotency-Key"))
	}
	return value, nil
}

func rightsPolicyExpectedVersion(value string) (int64, error) {
	match := rightsPolicyETagPattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return 0, invalidRequest(fmt.Errorf("invalid If-Match policy version"))
	}
	version, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || version <= 0 {
		return 0, invalidRequest(fmt.Errorf("invalid If-Match policy version"))
	}
	return version, nil
}

func rightsPolicyETag(version int64) string {
	return fmt.Sprintf(`"v%d"`, version)
}

func rightsManagementReadHTTPError(err error) error {
	return rightsManagementHTTPError(err, false)
}

func rightsManagementMutationHTTPError(err error) error {
	return rightsManagementHTTPError(err, true)
}

func rightsManagementHTTPError(err error, mutation bool) error {
	if err == nil {
		return nil
	}
	var applicationError *sharederrors.AppError
	if errors.As(err, &applicationError) {
		return applicationError
	}
	switch {
	case errors.Is(err, sharedrepository.ErrInvalidInput):
		return sharederrors.Wrap(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "", err)
	case errors.Is(err, sharedrepository.ErrNotFound) && !mutation:
		return sharederrors.Wrap(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "", err)
	case errors.Is(err, sharedrepository.ErrNotFound), errors.Is(err, sharedrepository.ErrConflict),
		errors.Is(err, sharedrepository.ErrConstraint), errors.Is(err, sharedrepository.ErrImmutable):
		return sharederrors.Wrap(sharederrors.CodeConflict, stdhttp.StatusConflict, "", err)
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.Wrap(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "", err)
	default:
		return err
	}
}
