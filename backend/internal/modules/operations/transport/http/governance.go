package http

import (
	"context"
	"fmt"
	"strconv"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	domain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type governanceService interface {
	Usage(context.Context, identitydomain.Subject) (domain.UsageOverview, error)
	RetentionPolicies(context.Context, identitydomain.Subject) ([]domain.RetentionPolicy, error)
	RetentionRun(context.Context, identitydomain.Subject, int64) (domain.CleanupResult, error)
	PreviewRetention(context.Context, operationsapplication.RetentionInput) (domain.CleanupResult, error)
	ApproveRetention(context.Context, operationsapplication.RetentionRunInput) (domain.CleanupResult, error)
	ExecuteRetention(context.Context, operationsapplication.RetentionRunInput) (domain.CleanupResult, error)
	Audit(context.Context, identitydomain.Subject, domain.AuditQuery) (domain.AuditPage, error)
}

type GovernanceHandler struct{ service governanceService }

func NewGovernanceHandler(service governanceService) *GovernanceHandler {
	return &GovernanceHandler{service: service}
}

func RegisterGovernanceRoutes(router *gin.Engine, service *operationsapplication.GovernanceService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewGovernanceHandler(service)
	usage := router.Group("/api/v1/operations/usage", httptransport.RequireAuthentication(authenticator), httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	usage.GET("", httptransport.Wrap(handler.Usage))
	admin := router.Group("/api/v1/operations", httptransport.RequireAuthentication(authenticator), httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.GET("/retention-policies", httptransport.Wrap(handler.RetentionPolicies))
	admin.POST("/retention-policies/:id/preview", httptransport.Wrap(handler.PreviewRetention))
	admin.GET("/retention-runs/:id", httptransport.Wrap(handler.RetentionRun))
	admin.POST("/retention-runs/:id/approve", httptransport.Wrap(handler.ApproveRetention))
	admin.POST("/retention-runs/:id/execute", httptransport.Wrap(handler.ExecuteRetention))
	admin.GET("/audit-logs", httptransport.Wrap(handler.Audit))
}

type GovernanceResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type RetentionPolicyResponse struct {
	ID            int64  `json:"id"`
	Version       int64  `json:"version"`
	DataClass     string `json:"data_class"`
	RetentionDays int    `json:"retention_days"`
	Action        string `json:"action"`
	Enabled       bool   `json:"enabled"`
	Description   string `json:"description"`
	Protected     bool   `json:"protected"`
}

type RetentionPreviewRequest struct {
	ExpectedVersion int64 `json:"expected_version" binding:"required"`
	BatchSize       int   `json:"batch_size" binding:"required"`
}

type RetentionConfirmationRequest struct {
	CandidateHash string `json:"candidate_hash" binding:"required"`
}

// Usage returns current-user hard quotas and workspace observed usage.
// @Summary Get quota and usage overview
// @Tags operations
// @Produce json
// @Security BearerAuth
// @Success 200 {object} GovernanceResult[domain.UsageOverview]
// @Failure 401 {object} GovernanceResult[EmptyResponse]
// @Failure 403 {object} GovernanceResult[EmptyResponse]
// @Failure 503 {object} GovernanceResult[EmptyResponse]
// @Router /api/v1/operations/usage [get]
func (handler *GovernanceHandler) Usage(c *gin.Context) error {
	httptransport.SetModule(c, "operations")
	subject, err := governanceSubject(c)
	if err != nil {
		return err
	}
	result, err := handler.service.Usage(c.Request.Context(), subject)
	if err != nil {
		return operationsapplication.GovernanceHTTPError(err)
	}
	httptransport.OK(c, result)
	return nil
}

// RetentionPolicies lists the closed server-side retention policy set.
// @Summary List retention policies
// @Tags operations
// @Produce json
// @Security BearerAuth
// @Success 200 {object} GovernanceResult[[]RetentionPolicyResponse]
// @Failure 401 {object} GovernanceResult[EmptyResponse]
// @Failure 403 {object} GovernanceResult[EmptyResponse]
// @Failure 503 {object} GovernanceResult[EmptyResponse]
// @Router /api/v1/operations/retention-policies [get]
func (handler *GovernanceHandler) RetentionPolicies(c *gin.Context) error {
	httptransport.SetModule(c, "operations")
	subject, err := governanceSubject(c)
	if err != nil {
		return err
	}
	policies, err := handler.service.RetentionPolicies(c.Request.Context(), subject)
	if err != nil {
		return operationsapplication.GovernanceHTTPError(err)
	}
	response := make([]RetentionPolicyResponse, 0, len(policies))
	for _, policy := range policies {
		response = append(response, retentionPolicyResponse(policy))
	}
	httptransport.OK(c, response)
	return nil
}

// PreviewRetention counts a bounded candidate batch without changing data.
// @Summary Preview a retention batch
// @Tags operations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "retention policy ID"
// @Param request body RetentionPreviewRequest true "preview boundary"
// @Success 200 {object} GovernanceResult[domain.CleanupResult]
// @Failure 400 {object} GovernanceResult[EmptyResponse]
// @Failure 401 {object} GovernanceResult[EmptyResponse]
// @Failure 403 {object} GovernanceResult[EmptyResponse]
// @Failure 404 {object} GovernanceResult[EmptyResponse]
// @Failure 409 {object} GovernanceResult[EmptyResponse]
// @Failure 503 {object} GovernanceResult[EmptyResponse]
// @Router /api/v1/operations/retention-policies/{id}/preview [post]
func (handler *GovernanceHandler) PreviewRetention(c *gin.Context) error {
	httptransport.SetModule(c, "operations")
	subject, err := governanceSubject(c)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return operationsapplication.GovernanceHTTPError(fmt.Errorf("%w: invalid retention policy id", sharedrepository.ErrInvalidInput))
	}
	var request RetentionPreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return operationsapplication.GovernanceHTTPError(fmt.Errorf("%w: invalid retention preview", sharedrepository.ErrInvalidInput))
	}
	result, err := handler.service.PreviewRetention(c.Request.Context(), operationsapplication.RetentionInput{
		Subject: subject, PolicyID: id, ExpectedVersion: request.ExpectedVersion, BatchSize: request.BatchSize,
	})
	if err != nil {
		return operationsapplication.GovernanceHTTPError(err)
	}
	httptransport.OK(c, result)
	return nil
}

// RetentionRun returns one frozen run for independent Admin handoff.
// @Summary Get a frozen retention run
// @Tags operations
// @Produce json
// @Security BearerAuth
// @Param id path int true "retention run ID"
// @Success 200 {object} GovernanceResult[domain.CleanupResult]
// @Failure 400 {object} GovernanceResult[EmptyResponse]
// @Failure 401 {object} GovernanceResult[EmptyResponse]
// @Failure 403 {object} GovernanceResult[EmptyResponse]
// @Failure 404 {object} GovernanceResult[EmptyResponse]
// @Failure 503 {object} GovernanceResult[EmptyResponse]
// @Router /api/v1/operations/retention-runs/{id} [get]
func (handler *GovernanceHandler) RetentionRun(c *gin.Context) error {
	httptransport.SetModule(c, "operations")
	subject, err := governanceSubject(c)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return operationsapplication.GovernanceHTTPError(fmt.Errorf("%w: invalid retention run id", sharedrepository.ErrInvalidInput))
	}
	result, err := handler.service.RetentionRun(c.Request.Context(), subject, id)
	if err != nil {
		return operationsapplication.GovernanceHTTPError(err)
	}
	httptransport.OK(c, result)
	return nil
}

// ApproveRetention freezes an exact preview hash before execution.
// @Summary Approve a frozen retention run
// @Tags operations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "retention run ID"
// @Param request body RetentionConfirmationRequest true "frozen candidate hash"
// @Success 200 {object} GovernanceResult[domain.CleanupResult]
// @Failure 400 {object} GovernanceResult[EmptyResponse]
// @Failure 401 {object} GovernanceResult[EmptyResponse]
// @Failure 403 {object} GovernanceResult[EmptyResponse]
// @Failure 404 {object} GovernanceResult[EmptyResponse]
// @Failure 409 {object} GovernanceResult[EmptyResponse]
// @Failure 503 {object} GovernanceResult[EmptyResponse]
// @Router /api/v1/operations/retention-runs/{id}/approve [post]
func (handler *GovernanceHandler) ApproveRetention(c *gin.Context) error {
	return handler.retentionRun(c, true)
}

// ExecuteRetention revalidates policy and exact candidates before deleting.
// @Summary Execute an approved retention run
// @Tags operations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "retention run ID"
// @Param request body RetentionConfirmationRequest true "frozen candidate hash"
// @Success 200 {object} GovernanceResult[domain.CleanupResult]
// @Failure 400 {object} GovernanceResult[EmptyResponse]
// @Failure 401 {object} GovernanceResult[EmptyResponse]
// @Failure 403 {object} GovernanceResult[EmptyResponse]
// @Failure 404 {object} GovernanceResult[EmptyResponse]
// @Failure 409 {object} GovernanceResult[EmptyResponse]
// @Failure 503 {object} GovernanceResult[EmptyResponse]
// @Router /api/v1/operations/retention-runs/{id}/execute [post]
func (handler *GovernanceHandler) ExecuteRetention(c *gin.Context) error {
	return handler.retentionRun(c, false)
}

func (handler *GovernanceHandler) retentionRun(c *gin.Context, approve bool) error {
	httptransport.SetModule(c, "operations")
	subject, err := governanceSubject(c)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return operationsapplication.GovernanceHTTPError(fmt.Errorf("%w: invalid retention run id", sharedrepository.ErrInvalidInput))
	}
	var request RetentionConfirmationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return operationsapplication.GovernanceHTTPError(fmt.Errorf("%w: invalid retention confirmation", sharedrepository.ErrInvalidInput))
	}
	input := operationsapplication.RetentionRunInput{Subject: subject, RunID: id, CandidateHash: request.CandidateHash}
	var result domain.CleanupResult
	if approve {
		result, err = handler.service.ApproveRetention(c.Request.Context(), input)
	} else {
		result, err = handler.service.ExecuteRetention(c.Request.Context(), input)
	}
	if err != nil {
		return operationsapplication.GovernanceHTTPError(err)
	}
	httptransport.OK(c, result)
	return nil
}

// Audit returns stable, safe audit facts without before/after payloads.
// @Summary List audit logs
// @Tags operations
// @Produce json
// @Security BearerAuth
// @Param cursor query string false "opaque signed audit cursor"
// @Param limit query int false "page size"
// @Param action query string false "exact action"
// @Param resource_type query string false "exact resource type"
// @Param result query string false "success, failure or denied"
// @Success 200 {object} GovernanceResult[domain.AuditPage]
// @Failure 400 {object} GovernanceResult[EmptyResponse]
// @Failure 401 {object} GovernanceResult[EmptyResponse]
// @Failure 403 {object} GovernanceResult[EmptyResponse]
// @Failure 503 {object} GovernanceResult[EmptyResponse]
// @Router /api/v1/operations/audit-logs [get]
func (handler *GovernanceHandler) Audit(c *gin.Context) error {
	httptransport.SetModule(c, "operations")
	subject, err := governanceSubject(c)
	if err != nil {
		return err
	}
	query := domain.AuditQuery{Action: c.Query("action"), ResourceType: c.Query("resource_type"), Result: c.Query("result"), Limit: 50}
	if raw := c.Query("cursor"); raw != "" {
		query.Cursor = raw
	}
	if err == nil {
		if raw := c.Query("limit"); raw != "" {
			query.Limit, err = strconv.Atoi(raw)
		}
	}
	if err != nil {
		return operationsapplication.GovernanceHTTPError(fmt.Errorf("%w: invalid audit query", sharedrepository.ErrInvalidInput))
	}
	query.SubjectUserID = subject.UserID
	page, err := handler.service.Audit(c.Request.Context(), subject, query)
	if err != nil {
		return operationsapplication.GovernanceHTTPError(err)
	}
	httptransport.OK(c, page)
	return nil
}

func governanceSubject(c *gin.Context) (identitydomain.Subject, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return identitydomain.Subject{}, operationsapplication.GovernanceHTTPError(sharedrepository.ErrUnavailable)
	}
	return identitydomain.Subject{UserID: subject.UserID, SessionID: subject.SessionID, Role: identitydomain.Role(subject.Role)}, nil
}

func retentionPolicyResponse(policy domain.RetentionPolicy) RetentionPolicyResponse {
	return RetentionPolicyResponse{ID: policy.ID, Version: policy.Version, DataClass: policy.DataClass, RetentionDays: policy.RetentionDays, Action: policy.Action, Enabled: policy.Enabled, Description: policy.Description, Protected: policy.Protected}
}
