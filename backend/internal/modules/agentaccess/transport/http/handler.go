package http

import (
	"context"
	stdhttp "net/http"
	"strconv"

	agentapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/agentaccess/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/agentaccess/domain"
	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type tokenService interface {
	Create(context.Context, agentapplication.CreateInput) (agentapplication.CreatedToken, error)
	List(context.Context, identitydomain.Subject) ([]domain.Token, error)
	Revoke(context.Context, agentapplication.RevokeInput) (*domain.Token, error)
}

type Handler struct{ service tokenService }

func NewHandler(service tokenService) *Handler { return &Handler{service: service} }

// List returns only safe token metadata.
// @Summary List current user's Agent Tokens
// @Tags agent-access
// @Produce json
// @Security BearerAuth
// @Success 200 {object} AgentAccessResult[[]TokenResponse]
// @Failure 401 {object} AgentAccessResult[EmptyResponse]
// @Failure 503 {object} AgentAccessResult[EmptyResponse]
func (handler *Handler) List(c *gin.Context) error {
	httptransport.SetModule(c, "agentaccess")
	subject, err := browserSubject(c)
	if err != nil {
		return err
	}
	tokens, err := handler.service.List(c.Request.Context(), subject)
	if err != nil {
		return err
	}
	httptransport.OK(c, tokenResponses(tokens))
	return nil
}

// Create returns the raw credential once; subsequent responses never include it.
// @Summary Create an Agent Token
// @Tags agent-access
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateTokenRequest true "Agent Token settings"
// @Success 201 {object} AgentAccessResult[CreatedTokenResponse]
// @Failure 400 {object} AgentAccessResult[EmptyResponse]
// @Failure 401 {object} AgentAccessResult[EmptyResponse]
// @Failure 403 {object} AgentAccessResult[EmptyResponse]
// @Failure 409 {object} AgentAccessResult[EmptyResponse]
// @Failure 503 {object} AgentAccessResult[EmptyResponse]
func (handler *Handler) Create(c *gin.Context) error {
	httptransport.SetModule(c, "agentaccess")
	var request CreateTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest()
	}
	subject, err := browserSubject(c)
	if err != nil {
		return err
	}
	scopes := make([]domain.Scope, len(request.Scopes))
	for index, scope := range request.Scopes {
		scopes[index] = domain.Scope(scope)
	}
	created, err := handler.service.Create(c.Request.Context(), agentapplication.CreateInput{Subject: subject, Name: request.Name, Scopes: scopes, LifetimeDays: request.LifetimeDays})
	if err != nil {
		return err
	}
	response := CreatedTokenResponse{TokenResponse: tokenResponse(created.Token), Token: created.Raw}
	httptransport.Created(c, response)
	return nil
}

// Revoke invalidates the token immediately using optimistic concurrency.
// @Summary Revoke an Agent Token
// @Tags agent-access
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Agent Token ID"
// @Param request body RevokeTokenRequest true "expected version"
// @Success 200 {object} AgentAccessResult[TokenResponse]
// @Failure 400 {object} AgentAccessResult[EmptyResponse]
// @Failure 401 {object} AgentAccessResult[EmptyResponse]
// @Failure 404 {object} AgentAccessResult[EmptyResponse]
// @Failure 409 {object} AgentAccessResult[EmptyResponse]
// @Failure 503 {object} AgentAccessResult[EmptyResponse]
func (handler *Handler) Revoke(c *gin.Context) error {
	httptransport.SetModule(c, "agentaccess")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return invalidRequest()
	}
	var request RevokeTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest()
	}
	subject, err := browserSubject(c)
	if err != nil {
		return err
	}
	token, err := handler.service.Revoke(c.Request.Context(), agentapplication.RevokeInput{Subject: subject, TokenID: id, ExpectedVersion: request.ExpectedVersion})
	if err != nil {
		return err
	}
	httptransport.OK(c, tokenResponse(*token))
	return nil
}

func browserSubject(c *gin.Context) (identitydomain.Subject, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok || subject.SessionID <= 0 || subject.AgentTokenID > 0 {
		return identitydomain.Subject{}, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	return identitydomain.Subject{UserID: subject.UserID, SessionID: subject.SessionID, Role: identitydomain.Role(subject.Role)}, nil
}

func invalidRequest() error {
	return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid agent token request")
}
