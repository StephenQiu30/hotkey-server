package http

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/application"
	knowledgedomain "github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type proposalReader interface {
	GetProposal(context.Context, int64) (knowledgedomain.Proposal, error)
}

type Handler struct {
	proposals *knowledgeapplication.ProposalService
	reader    proposalReader
	reconcile *knowledgeapplication.Reconciler
	vault     knowledgeapplication.Vault
}

func NewHandler(proposals *knowledgeapplication.ProposalService, reader proposalReader, reconcile *knowledgeapplication.Reconciler, vault knowledgeapplication.Vault) *Handler {
	return &Handler{proposals: proposals, reader: reader, reconcile: reconcile, vault: vault}
}

type ProposalRequest struct {
	DocumentID   int64  `json:"document_id"`
	BaseRevision int64  `json:"base_revision"`
	BaseHash     string `json:"base_hash"`
	Frontmatter  string `json:"frontmatter"`
	Body         string `json:"body"`
	Reason       string `json:"reason"`
}

type ProposalResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

// Create creates a pending knowledge proposal.
// @Summary Create knowledge proposal
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Param request body ProposalRequest true "proposal"
// @Success 200 {object} ProposalResult[domain.Proposal]
// @Failure 400 {object} ProposalResult[EmptyResponse]
// @Failure 401 {object} ProposalResult[EmptyResponse]
// @Failure 403 {object} ProposalResult[EmptyResponse]
// @Failure 409 {object} ProposalResult[EmptyResponse]
// @Router /api/v1/knowledge/proposals [post]
func (handler *Handler) Create(c *gin.Context) error {
	httptransport.SetModule(c, "knowledge")
	var request ProposalRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid knowledge proposal")
	}
	proposal, err := handler.proposals.CreateContext(c.Request.Context(), request.DocumentID, request.BaseRevision, request.BaseHash, request.Frontmatter, request.Body, request.Reason)
	if err != nil {
		return knowledgeError(err)
	}
	httptransport.OK(c, proposal)
	return nil
}

// Approve approves a pending proposal after optimistic-version validation.
// @Summary Approve knowledge proposal
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Param id path int true "proposal ID"
// @Param version query int true "proposal version"
// @Success 200 {object} ProposalResult[domain.Proposal]
// @Failure 400 {object} ProposalResult[EmptyResponse]
// @Failure 401 {object} ProposalResult[EmptyResponse]
// @Failure 403 {object} ProposalResult[EmptyResponse]
// @Failure 409 {object} ProposalResult[EmptyResponse]
// @Router /api/v1/knowledge/proposals/{id}/approve [post]
func (handler *Handler) Approve(c *gin.Context) error {
	return handler.change(c, knowledgedomain.ProposalApproved)
}

// Reject rejects a pending proposal.
// @Summary Reject knowledge proposal
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Param id path int true "proposal ID"
// @Param version query int true "proposal version"
// @Success 200 {object} ProposalResult[domain.Proposal]
// @Failure 400 {object} ProposalResult[EmptyResponse]
// @Failure 401 {object} ProposalResult[EmptyResponse]
// @Failure 403 {object} ProposalResult[EmptyResponse]
// @Failure 409 {object} ProposalResult[EmptyResponse]
// @Router /api/v1/knowledge/proposals/{id}/reject [post]
func (handler *Handler) Reject(c *gin.Context) error {
	return handler.change(c, knowledgedomain.ProposalRejected)
}

func (handler *Handler) change(c *gin.Context, status knowledgedomain.ProposalStatus) error {
	httptransport.SetModule(c, "knowledge")
	id, version, err := proposalPath(c)
	if err != nil {
		return err
	}
	var proposal knowledgedomain.Proposal
	if status == knowledgedomain.ProposalApproved {
		proposal, err = handler.proposals.Approve(c.Request.Context(), id, version)
	} else {
		proposal, err = handler.proposals.Reject(c.Request.Context(), id, version)
	}
	if err != nil {
		return knowledgeError(err)
	}
	httptransport.OK(c, proposal)
	return nil
}

// Apply writes an approved proposal using Vault atomic replacement.
// @Summary Apply knowledge proposal
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Param id path int true "proposal ID"
// @Param version query int true "proposal version"
// @Success 200 {object} ProposalResult[domain.Document]
// @Failure 400 {object} ProposalResult[EmptyResponse]
// @Failure 401 {object} ProposalResult[EmptyResponse]
// @Failure 403 {object} ProposalResult[EmptyResponse]
// @Failure 409 {object} ProposalResult[EmptyResponse]
// @Router /api/v1/knowledge/proposals/{id}/apply [post]
func (handler *Handler) Apply(c *gin.Context) error {
	httptransport.SetModule(c, "knowledge")
	id, _, err := proposalPath(c)
	if err != nil {
		return err
	}
	proposal, err := handler.reader.GetProposal(c.Request.Context(), id)
	if err != nil {
		return knowledgeError(err)
	}
	document, err := handler.proposals.Apply(c.Request.Context(), proposal, handler.vault)
	if err != nil {
		return knowledgeError(err)
	}
	httptransport.OK(c, document)
	return nil
}

// Reconcile compares database projections with Vault files.
// @Summary Reconcile knowledge Vault
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ProposalResult[domain.ReconciliationReport]
// @Failure 401 {object} ProposalResult[EmptyResponse]
// @Failure 403 {object} ProposalResult[EmptyResponse]
// @Failure 503 {object} ProposalResult[EmptyResponse]
// @Router /api/v1/knowledge/reconcile [post]
func (handler *Handler) Reconcile(c *gin.Context) error {
	httptransport.SetModule(c, "knowledge")
	report, err := handler.reconcile.Reconcile(c.Request.Context())
	if err != nil {
		return knowledgeError(err)
	}
	httptransport.OK(c, report)
	return nil
}

func proposalPath(c *gin.Context) (int64, int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, 0, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid proposal id")
	}
	version, err := strconv.ParseInt(c.Query("version"), 10, 64)
	if err != nil || version <= 0 {
		return 0, 0, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "proposal version is required")
	}
	return id, version, nil
}

func knowledgeError(err error) error {
	switch {
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.New(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "knowledge resource not found")
	case errors.Is(err, sharedrepository.ErrConflict), errors.Is(err, sharedrepository.ErrImmutable):
		return sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "knowledge resource changed")
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid knowledge request")
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "knowledge service unavailable")
	default:
		return fmt.Errorf("knowledge operation: %w", err)
	}
}
