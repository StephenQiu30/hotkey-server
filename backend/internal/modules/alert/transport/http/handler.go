package http

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"

	alertapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/alert/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/alert/domain"
	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type alertService interface {
	List(context.Context, domain.ListQuery) (domain.ThreadPage, error)
	Get(context.Context, int64) (domain.ThreadDetail, error)
	Acknowledge(context.Context, alertapplication.ActionInput) (domain.Thread, error)
	Resolve(context.Context, alertapplication.ActionInput) (domain.Thread, error)
	Suppress(context.Context, alertapplication.ActionInput) (domain.Thread, error)
}

var _ alertService = (*alertapplication.Service)(nil)

type Handler struct{ service alertService }

func NewHandler(service alertService) *Handler { return &Handler{service: service} }

// List godoc
//
// @Summary List alert threads
// @Tags alerts
// @Produce json
// @Security BearerAuth
// @Param state query string false "thread state" Enums(open,acknowledged,resolved,suppressed)
// @Param severity query string false "severity" Enums(info,warning,critical)
// @Param monitor_id query int false "monitor ID" minimum(1)
// @Param limit query int false "page size" minimum(1) maximum(100) default(20)
// @Param cursor query string false "opaque alert cursor"
// @Success 200 {object} AlertResult[AlertPageResponse]
// @Failure 400 {object} AlertResult[EmptyResponse]
// @Failure 401 {object} AlertResult[EmptyResponse]
// @Failure 503 {object} AlertResult[EmptyResponse]
func (handler *Handler) List(c *gin.Context) error {
	httptransport.SetModule(c, "alert")
	query, err := alertListQuery(c)
	if err != nil {
		return err
	}
	page, err := handler.service.List(c.Request.Context(), query)
	if err != nil {
		return alertError(err)
	}
	httptransport.OK(c, pageResponse(page))
	return nil
}

// Get godoc
//
// @Summary Get an alert thread
// @Tags alerts
// @Produce json
// @Security BearerAuth
// @Param id path int true "alert thread ID"
// @Success 200 {object} AlertResult[AlertDetailResponse]
// @Failure 400 {object} AlertResult[EmptyResponse]
// @Failure 401 {object} AlertResult[EmptyResponse]
// @Failure 404 {object} AlertResult[EmptyResponse]
// @Failure 503 {object} AlertResult[EmptyResponse]
func (handler *Handler) Get(c *gin.Context) error {
	httptransport.SetModule(c, "alert")
	threadID, err := alertThreadID(c)
	if err != nil {
		return err
	}
	detail, err := handler.service.Get(c.Request.Context(), threadID)
	if err != nil {
		return alertError(err)
	}
	httptransport.OK(c, detailResponse(detail))
	return nil
}

// Acknowledge godoc
//
// @Summary Acknowledge an alert thread
// @Tags alerts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "alert thread ID"
// @Param request body AlertActionRequest true "versioned alert action"
// @Success 200 {object} AlertResult[AlertThreadResponse]
// @Failure 400 {object} AlertResult[EmptyResponse]
// @Failure 401 {object} AlertResult[EmptyResponse]
// @Failure 404 {object} AlertResult[EmptyResponse]
// @Failure 409 {object} AlertResult[EmptyResponse]
// @Failure 503 {object} AlertResult[EmptyResponse]
func (handler *Handler) Acknowledge(c *gin.Context) error {
	return handler.action(c, handler.service.Acknowledge)
}

// Resolve godoc
//
// @Summary Resolve an alert thread
// @Tags alerts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "alert thread ID"
// @Param request body AlertActionRequest true "versioned alert action"
// @Success 200 {object} AlertResult[AlertThreadResponse]
// @Failure 400 {object} AlertResult[EmptyResponse]
// @Failure 401 {object} AlertResult[EmptyResponse]
// @Failure 404 {object} AlertResult[EmptyResponse]
// @Failure 409 {object} AlertResult[EmptyResponse]
// @Failure 503 {object} AlertResult[EmptyResponse]
func (handler *Handler) Resolve(c *gin.Context) error {
	return handler.action(c, handler.service.Resolve)
}

// Suppress godoc
//
// @Summary Suppress an alert thread
// @Tags alerts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "alert thread ID"
// @Param request body AlertActionRequest true "versioned alert action"
// @Success 200 {object} AlertResult[AlertThreadResponse]
// @Failure 400 {object} AlertResult[EmptyResponse]
// @Failure 401 {object} AlertResult[EmptyResponse]
// @Failure 403 {object} AlertResult[EmptyResponse]
// @Failure 404 {object} AlertResult[EmptyResponse]
// @Failure 409 {object} AlertResult[EmptyResponse]
// @Failure 503 {object} AlertResult[EmptyResponse]
func (handler *Handler) Suppress(c *gin.Context) error {
	return handler.action(c, handler.service.Suppress)
}

func (handler *Handler) action(c *gin.Context, execute func(context.Context, alertapplication.ActionInput) (domain.Thread, error)) error {
	httptransport.SetModule(c, "alert")
	threadID, err := alertThreadID(c)
	if err != nil {
		return err
	}
	subject, err := alertSubject(c)
	if err != nil {
		return err
	}
	var request AlertActionRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.ReasonCode) == "" || len(request.ReasonCode) > domain.MaxReasonCodeLength {
		return invalidAlertRequest()
	}
	thread, err := execute(c.Request.Context(), alertapplication.ActionInput{
		Subject: subject, ThreadID: threadID, ExpectedVersion: request.ExpectedVersion, ReasonCode: strings.TrimSpace(request.ReasonCode),
	})
	if err != nil {
		return alertError(err)
	}
	httptransport.OK(c, threadResponse(thread))
	return nil
}

func alertListQuery(c *gin.Context) (domain.ListQuery, error) {
	query := domain.ListQuery{Limit: 20, Cursor: c.Query("cursor")}
	if value := c.Query("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			return domain.ListQuery{}, invalidAlertRequest()
		}
		query.Limit = limit
	}
	if value := c.Query("state"); value != "" {
		state := domain.State(value)
		query.State = &state
	}
	if value := c.Query("severity"); value != "" {
		severity := domain.Severity(value)
		query.Severity = &severity
	}
	if value := c.Query("monitor_id"); value != "" {
		monitorID, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return domain.ListQuery{}, invalidAlertRequest()
		}
		query.MonitorID = &monitorID
	}
	if err := query.Validate(); err != nil {
		return domain.ListQuery{}, invalidAlertRequest()
	}
	return query, nil
}

func alertThreadID(c *gin.Context) (int64, error) {
	threadID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || threadID <= 0 {
		return 0, invalidAlertRequest()
	}
	return threadID, nil
}

func alertSubject(c *gin.Context) (identitydomain.Subject, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return identitydomain.Subject{}, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	return identitydomain.Subject{UserID: subject.UserID, SessionID: subject.SessionID, AgentTokenID: subject.AgentTokenID, Role: identitydomain.Role(subject.Role)}, nil
}

func invalidAlertRequest() error {
	return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid alert request")
}

func alertError(err error) error {
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	switch {
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		return invalidAlertRequest()
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.New(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "alert not found")
	case errors.Is(err, sharedrepository.ErrConflict), errors.Is(err, sharedrepository.ErrImmutable):
		return sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "alert state or version conflict")
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "alert service unavailable")
	default:
		return fmt.Errorf("alert operation: %w", err)
	}
}
