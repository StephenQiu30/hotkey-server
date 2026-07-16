package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"strconv"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type sourceService interface {
	Create(context.Context, sourceapplication.CreateInput) (*domain.ManagementSourceConnection, error)
	Update(context.Context, sourceapplication.UpdateInput) (*domain.ManagementSourceConnection, error)
	Enable(context.Context, sourceapplication.LifecycleInput) (*domain.ManagementSourceConnection, error)
	Disable(context.Context, sourceapplication.LifecycleInput) (*domain.ManagementSourceConnection, error)
	Archive(context.Context, sourceapplication.LifecycleInput) (*domain.ManagementSourceConnection, error)
	Restore(context.Context, sourceapplication.LifecycleInput) (*domain.ManagementSourceConnection, error)
	GetPublic(context.Context, identitydomain.Subject, int64) (domain.PublicSourceConnection, error)
	GetManagement(context.Context, identitydomain.Subject, int64) (domain.ManagementSourceConnection, error)
	ListPublic(context.Context, sourceapplication.ListInput) (domain.PublicSourceConnectionPage, error)
	ListManagement(context.Context, sourceapplication.ListInput) (domain.ManagementSourceConnectionPage, error)
}

type Handler struct{ service sourceService }

func NewHandler(service sourceService) *Handler { return &Handler{service: service} }

// List exposes the public Source fields to viewers/editors. Administrators
// additionally receive endpoint and fixed allowlisted configuration fields,
// represented by SourceReadPageResponse's optional management branch.
// @Summary List source connections
// @Tags sources
// @Produce json
// @Security BearerAuth
// @Param cursor query string false "cursor"
// @Param limit query int false "page size"
// @Success 200 {object} SourceResult[SourceReadPageResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-connections [get]
func (handler *Handler) List(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	query, err := sourceListQuery(c)
	if err != nil {
		return err
	}
	if subject.Role == identitydomain.RoleAdmin {
		page, err := handler.service.ListManagement(c.Request.Context(), sourceapplication.ListInput{Subject: subject, Query: query})
		if err != nil {
			return err
		}
		response := SourceReadPageResponse{Items: make([]SourceReadResponse, 0, len(page.Items)), NextCursor: page.NextCursor}
		for _, item := range page.Items {
			response.Items = append(response.Items, managementReadResponse(item))
		}
		httptransport.OK(c, response)
		return nil
	}
	page, err := handler.service.ListPublic(c.Request.Context(), sourceapplication.ListInput{Subject: subject, Query: query})
	if err != nil {
		return err
	}
	response := SourceReadPageResponse{Items: make([]SourceReadResponse, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		response.Items = append(response.Items, sourceReadResponse(item))
	}
	httptransport.OK(c, response)
	return nil
}

// Get returns the public Source shape, with optional endpoint/config only for
// administrators as documented by SourceReadResponse.
// @Summary Get a source connection
// @Tags sources
// @Produce json
// @Security BearerAuth
// @Param id path int true "source connection ID"
// @Success 200 {object} SourceResult[SourceReadResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-connections/{id} [get]
func (handler *Handler) Get(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	id, err := sourceID(c)
	if err != nil {
		return err
	}
	if subject.Role == identitydomain.RoleAdmin {
		item, err := handler.service.GetManagement(c.Request.Context(), subject, id)
		if err != nil {
			return err
		}
		httptransport.OK(c, managementReadResponse(item))
		return nil
	}
	item, err := handler.service.GetPublic(c.Request.Context(), subject, id)
	if err != nil {
		return err
	}
	httptransport.OK(c, sourceReadResponse(item))
	return nil
}

// Create records a credential reference but never echoes it.
// @Summary Create a source connection
// @Tags sources
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateSourceRequest true "source connection"
// @Success 201 {object} SourceResult[ManagementSourceResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-connections [post]
func (handler *Handler) Create(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	var request CreateSourceRequest
	if err := bindStrictJSON(c, &request); err != nil {
		return invalidRequest(err)
	}
	connection, err := sourceCreateInput(request)
	if err != nil {
		return invalidRequest(err)
	}
	result, err := handler.service.Create(c.Request.Context(), sourceapplication.CreateInput{Subject: subject, Connection: connection})
	if err != nil {
		return err
	}
	httptransport.Created(c, managementResponse(*result))
	return nil
}

// Update accepts only explicit source fields and expected_source_version.
// @Summary Update a source connection
// @Tags sources
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "source connection ID"
// @Param request body UpdateSourceRequest true "source update"
// @Success 200 {object} SourceResult[ManagementSourceResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-connections/{id} [patch]
func (handler *Handler) Update(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	id, err := sourceID(c)
	if err != nil {
		return err
	}
	var request UpdateSourceRequest
	if err := bindStrictJSON(c, &request); err != nil {
		return invalidRequest(err)
	}
	input, err := sourceUpdateInput(request)
	if err != nil {
		return invalidRequest(err)
	}
	input.Subject, input.ID = subject, id
	result, err := handler.service.Update(c.Request.Context(), input)
	if err != nil {
		return err
	}
	httptransport.OK(c, managementResponse(*result))
	return nil
}

// Enable marks a restored or disabled source connection schedulable.
// @Summary Enable a source connection
// @Tags sources
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "source connection ID"
// @Param request body SourceLifecycleRequest true "expected source version"
// @Success 200 {object} SourceResult[ManagementSourceResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-connections/{id}/enable [post]
func (handler *Handler) Enable(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.Enable)
}

// Disable prevents scheduling while preserving immutable historical references.
// @Summary Disable a source connection
// @Tags sources
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "source connection ID"
// @Param request body SourceLifecycleRequest true "expected source version"
// @Success 200 {object} SourceResult[ManagementSourceResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-connections/{id}/disable [post]
func (handler *Handler) Disable(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.Disable)
}

// Archive soft-deletes a source connection after schedulability checks.
// @Summary Archive a source connection
// @Tags sources
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "source connection ID"
// @Param request body SourceLifecycleRequest true "expected source version"
// @Success 200 {object} SourceResult[ManagementSourceResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-connections/{id}/archive [post]
func (handler *Handler) Archive(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.Archive)
}

// Restore leaves a source disabled and unknown until an explicit enable.
// @Summary Restore a source connection
// @Tags sources
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "source connection ID"
// @Param request body SourceLifecycleRequest true "expected source version"
// @Success 200 {object} SourceResult[ManagementSourceResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-connections/{id}/restore [post]
func (handler *Handler) Restore(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.Restore)
}
func (handler *Handler) lifecycle(c *gin.Context, operation func(context.Context, sourceapplication.LifecycleInput) (*domain.ManagementSourceConnection, error)) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	id, err := sourceID(c)
	if err != nil {
		return err
	}
	var request SourceLifecycleRequest
	if err := bindStrictJSON(c, &request); err != nil {
		return invalidRequest(err)
	}
	result, err := operation(c.Request.Context(), sourceapplication.LifecycleInput{Subject: subject, ID: id, ExpectedVersion: request.ExpectedSourceVersion})
	if err != nil {
		return err
	}
	httptransport.OK(c, managementResponse(*result))
	return nil
}

func sourceSubject(c *gin.Context) (identitydomain.Subject, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return identitydomain.Subject{}, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	return identitydomain.Subject{UserID: subject.UserID, SessionID: subject.SessionID, Role: identitydomain.Role(subject.Role)}, nil
}
func sourceID(c *gin.Context) (int64, error) {
	value, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || value <= 0 {
		return 0, invalidRequest(fmt.Errorf("invalid source connection id"))
	}
	return value, nil
}
func sourceListQuery(c *gin.Context) (domain.SourceConnectionListQuery, error) {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return domain.SourceConnectionListQuery{}, invalidRequest(fmt.Errorf("invalid limit"))
		}
		limit = value
	}
	return domain.SourceConnectionListQuery{Cursor: c.Query("cursor"), Limit: limit}, nil
}
func bindStrictJSON(c *gin.Context, target any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain one JSON value")
		}
		return err
	}
	return nil
}
func invalidRequest(cause error) error {
	return sharederrors.Wrap(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "", cause)
}
