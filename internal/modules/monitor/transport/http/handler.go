package http

import (
	"context"
	"fmt"
	stdhttp "net/http"
	"strconv"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	monitorapplication "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type monitorService interface {
	Create(context.Context, monitorapplication.CreateInput) (*domain.Monitor, *domain.MonitorConfigVersion, error)
	ReplaceDraft(context.Context, monitorapplication.ReplaceDraftInput) (*domain.Monitor, *domain.MonitorConfigVersion, error)
	AddAICandidate(context.Context, monitorapplication.AICandidateInput) (*domain.MonitorConfigVersion, *domain.MonitorRule, error)
	ApproveAICandidate(context.Context, monitorapplication.ApprovalInput) (*domain.MonitorConfigVersion, error)
	Preview(context.Context, identitydomain.Subject, int64) (monitorapplication.PreviewResult, error)
	Publish(context.Context, monitorapplication.PublishInput) (*domain.Monitor, *domain.MonitorConfigVersion, error)
	Pause(context.Context, monitorapplication.LifecycleInput) (*domain.Monitor, error)
	Resume(context.Context, monitorapplication.LifecycleInput) (*domain.Monitor, error)
	Archive(context.Context, monitorapplication.LifecycleInput) (*domain.Monitor, error)
	Restore(context.Context, monitorapplication.LifecycleInput) (*domain.Monitor, error)
	Delete(context.Context, monitorapplication.LifecycleInput) (*domain.Monitor, error)
	Get(context.Context, identitydomain.Subject, int64) (monitorapplication.MonitorView, error)
	List(context.Context, monitorapplication.ListInput) (monitorapplication.MonitorPage, error)
}

type Handler struct{ service monitorService }

func NewHandler(service monitorService) *Handler { return &Handler{service: service} }

// List returns the viewer-safe active/paused published view or, for
// collaborators, the same view plus safe draft metadata where it exists.
// @Summary List monitors
// @Tags monitors
// @Produce json
// @Security BearerAuth
// @Param cursor query string false "cursor"
// @Param limit query int false "page size"
// @Success 200 {object} MonitorResult[MonitorPageResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors [get]
func (handler *Handler) List(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	subject, err := monitorSubject(c)
	if err != nil {
		return err
	}
	limit, err := monitorPageLimit(c)
	if err != nil {
		return err
	}
	page, err := handler.service.List(c.Request.Context(), monitorapplication.ListInput{Subject: subject, Cursor: c.Query("cursor"), Limit: limit})
	if err != nil {
		return err
	}
	response := MonitorPageResponse{Items: make([]MonitorResponse, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		response.Items = append(response.Items, monitorResponse(item))
	}
	httptransport.OK(c, response)
	return nil
}

// Get returns the role-appropriate safe Monitor projection.
// @Summary Get a monitor
// @Tags monitors
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Success 200 {object} MonitorResult[MonitorResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id} [get]
func (handler *Handler) Get(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	subject, err := monitorSubject(c)
	if err != nil {
		return err
	}
	id, err := monitorID(c)
	if err != nil {
		return err
	}
	view, err := handler.service.Get(c.Request.Context(), subject, id)
	if err != nil {
		return err
	}
	httptransport.OK(c, monitorResponse(view))
	return nil
}

// Create creates a Monitor and its first draft. The request cannot control
// rule origin or approval state; DTO conversion fixes both to user/approved.
// @Summary Create a monitor draft
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateMonitorRequest true "monitor draft"
// @Success 201 {object} MonitorResult[MonitorResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors [post]
func (handler *Handler) Create(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	subject, err := monitorSubject(c)
	if err != nil {
		return err
	}
	var request CreateMonitorRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	monitor, _, err := handler.service.Create(c.Request.Context(), monitorapplication.CreateInput{Subject: subject, Draft: monitorDraft(request)})
	if err != nil {
		return err
	}
	view, err := handler.service.Get(c.Request.Context(), subject, monitor.ID)
	if err != nil {
		return err
	}
	httptransport.Created(c, monitorResponse(view))
	return nil
}

// ReplaceDraft applies the strict expected-monitor/expected-draft protocol.
// @Summary Replace a monitor draft
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param request body ReplaceDraftRequest true "full draft replacement"
// @Success 200 {object} MonitorResult[MonitorResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/draft [put]
func (handler *Handler) ReplaceDraft(c *gin.Context) error { return handler.replaceDraft(c) }

func (handler *Handler) replaceDraft(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	subject, err := monitorSubject(c)
	if err != nil {
		return err
	}
	id, err := monitorID(c)
	if err != nil {
		return err
	}
	var request ReplaceDraftRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	expected, err := expectedVersions(request.ExpectedDraftRequest)
	if err != nil {
		return invalidRequest(err)
	}
	monitor, _, err := handler.service.ReplaceDraft(c.Request.Context(), monitorapplication.ReplaceDraftInput{Subject: subject, MonitorID: id, Expected: expected, Draft: replaceMonitorDraft(request)})
	if err != nil {
		return err
	}
	view, err := handler.service.Get(c.Request.Context(), subject, monitor.ID)
	if err != nil {
		return err
	}
	httptransport.OK(c, monitorResponse(view))
	return nil
}

// AddAICandidate adds a server-owned pending AI rule.
// @Summary Add a pending AI rule candidate
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param request body AICandidateRequest true "AI candidate"
// @Success 200 {object} MonitorResult[MonitorRuleResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/draft/ai-candidates [post]
func (handler *Handler) AddAICandidate(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	subject, err := monitorSubject(c)
	if err != nil {
		return err
	}
	id, err := monitorID(c)
	if err != nil {
		return err
	}
	var request AICandidateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	expected, err := expectedVersions(request.ExpectedDraftRequest)
	if err != nil {
		return invalidRequest(err)
	}
	_, rule, err := handler.service.AddAICandidate(c.Request.Context(), monitorapplication.AICandidateInput{Subject: subject, MonitorID: id, Expected: expected, Rule: aiCandidateRule(request)})
	if err != nil {
		return err
	}
	httptransport.OK(c, MonitorRuleResponse{ID: rule.ID, RuleType: string(rule.RuleType), Operator: string(rule.Operator), Value: rule.Value, Weight: rule.Weight, Priority: rule.Priority, Origin: string(rule.Origin), ApprovalStatus: string(rule.ApprovalStatus), Enabled: rule.Enabled})
	return nil
}

// ApproveAICandidate accepts either approved or rejected, never an arbitrary
// client-controlled state.
// @Summary Approve or reject an AI rule candidate
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param rule_id path int true "rule ID"
// @Param request body ApprovalRequest true "approval"
// @Success 200 {object} MonitorResult[EmptyResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/draft/rules/{rule_id}/approval [post]
func (handler *Handler) ApproveAICandidate(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	subject, err := monitorSubject(c)
	if err != nil {
		return err
	}
	id, err := monitorID(c)
	if err != nil {
		return err
	}
	ruleID, err := ruleID(c)
	if err != nil {
		return err
	}
	var request ApprovalRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	expected, err := expectedVersions(request.ExpectedDraftRequest)
	if err != nil {
		return invalidRequest(err)
	}
	if _, err = handler.service.ApproveAICandidate(c.Request.Context(), monitorapplication.ApprovalInput{Subject: subject, MonitorID: id, RuleID: ruleID, Expected: expected, Approval: domain.RuleApprovalStatus(request.Approval)}); err != nil {
		return err
	}
	httptransport.Empty(c)
	return nil
}

// Preview remains a read-only application call without expected versions.
// @Summary Preview a monitor draft without persistence
// @Tags monitors
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Success 200 {object} MonitorResult[PreviewResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/preview [post]
func (handler *Handler) Preview(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	subject, err := monitorSubject(c)
	if err != nil {
		return err
	}
	id, err := monitorID(c)
	if err != nil {
		return err
	}
	preview, err := handler.service.Preview(c.Request.Context(), subject, id)
	if err != nil {
		return err
	}
	httptransport.OK(c, previewResponse(preview))
	return nil
}

// Publish atomically publishes the expected draft.
// @Summary Publish a monitor draft
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param request body PublishRequest true "expected versions"
// @Success 200 {object} MonitorResult[MonitorResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/publish [post]
func (handler *Handler) Publish(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	subject, err := monitorSubject(c)
	if err != nil {
		return err
	}
	id, err := monitorID(c)
	if err != nil {
		return err
	}
	var request PublishRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	expected, err := expectedVersions(request.ExpectedDraftRequest)
	if err != nil {
		return invalidRequest(err)
	}
	monitor, _, err := handler.service.Publish(c.Request.Context(), monitorapplication.PublishInput{Subject: subject, MonitorID: id, Expected: expected})
	if err != nil {
		return err
	}
	view, err := handler.service.Get(c.Request.Context(), subject, monitor.ID)
	if err != nil {
		return err
	}
	httptransport.OK(c, monitorResponse(view))
	return nil
}

// Pause moves an active Monitor to paused with the expected aggregate version.
// @Summary Pause a monitor
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param request body LifecycleRequest true "expected monitor version"
// @Success 200 {object} MonitorResult[MonitorResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/pause [post]
func (handler *Handler) Pause(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.Pause)
}

// Resume moves a paused Monitor to active only when it remains schedulable.
// @Summary Resume a monitor
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param request body LifecycleRequest true "expected monitor version"
// @Success 200 {object} MonitorResult[MonitorResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/resume [post]
func (handler *Handler) Resume(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.Resume)
}

// Archive soft-deletes the Monitor configuration identity.
// @Summary Archive a monitor
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param request body LifecycleRequest true "expected monitor version"
// @Success 200 {object} MonitorResult[MonitorResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/archive [post]
func (handler *Handler) Archive(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.Archive)
}

// Restore returns an archived Monitor to the paused state.
// @Summary Restore a monitor
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param request body LifecycleRequest true "expected monitor version"
// @Success 200 {object} MonitorResult[MonitorResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/restore [post]
func (handler *Handler) Restore(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.Restore)
}

// Delete soft-deletes an archived Monitor while retaining immutable evidence
// and report provenance.
// @Summary Delete an archived monitor
// @Tags monitors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param request body LifecycleRequest true "expected monitor version"
// @Success 200 {object} MonitorResult[MonitorResponse]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id} [delete]
func (handler *Handler) Delete(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	subject, err := monitorSubject(c)
	if err != nil {
		return err
	}
	id, err := monitorID(c)
	if err != nil {
		return err
	}
	var request LifecycleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	monitor, err := handler.service.Delete(c.Request.Context(), monitorapplication.LifecycleInput{Subject: subject, MonitorID: id, ExpectedMonitorVersion: request.ExpectedMonitorVersion})
	if err != nil {
		return err
	}
	httptransport.OK(c, monitorResponse(monitorapplication.MonitorView{Monitor: *monitor}))
	return nil
}

func (handler *Handler) lifecycle(c *gin.Context, operation func(context.Context, monitorapplication.LifecycleInput) (*domain.Monitor, error)) error {
	httptransport.SetModule(c, "monitor")
	subject, err := monitorSubject(c)
	if err != nil {
		return err
	}
	id, err := monitorID(c)
	if err != nil {
		return err
	}
	var request LifecycleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	monitor, err := operation(c.Request.Context(), monitorapplication.LifecycleInput{Subject: subject, MonitorID: id, ExpectedMonitorVersion: request.ExpectedMonitorVersion})
	if err != nil {
		return err
	}
	view, err := handler.service.Get(c.Request.Context(), subject, monitor.ID)
	if err != nil {
		return err
	}
	httptransport.OK(c, monitorResponse(view))
	return nil
}

func monitorSubject(c *gin.Context) (identitydomain.Subject, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return identitydomain.Subject{}, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	return identitydomain.Subject{UserID: subject.UserID, SessionID: subject.SessionID, Role: identitydomain.Role(subject.Role)}, nil
}
func monitorID(c *gin.Context) (int64, error) { return positivePathID(c, "id") }
func ruleID(c *gin.Context) (int64, error)    { return positivePathID(c, "rule_id") }
func positivePathID(c *gin.Context, name string) (int64, error) {
	value, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || value <= 0 {
		return 0, invalidRequest(fmt.Errorf("invalid %s", name))
	}
	return value, nil
}
func monitorPageLimit(c *gin.Context) (int, error) {
	const defaultLimit, maximumLimit = 50, 200
	limit := defaultLimit
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maximumLimit {
			return 0, invalidRequest(fmt.Errorf("limit must be 1-%d", maximumLimit))
		}
		limit = value
	}
	return limit, nil
}
func invalidRequest(cause error) error {
	return sharederrors.Wrap(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "", cause)
}
