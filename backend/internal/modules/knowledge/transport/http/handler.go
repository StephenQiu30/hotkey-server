package http

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	knowledgedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
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

// ListDocuments returns the current knowledge projections.
// @Summary List knowledge documents
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ProposalResult[[]DocumentResponse]
func (handler *Handler) ListDocuments(c *gin.Context) error {
	httptransport.SetModule(c, "knowledge")
	reader, ok := handler.reader.(interface {
		ListDocuments(context.Context) ([]knowledgedomain.Document, error)
	})
	if !ok {
		return knowledgeError(sharedrepository.ErrUnavailable)
	}
	items, err := reader.ListDocuments(c.Request.Context())
	if err != nil {
		return knowledgeError(err)
	}
	response := make([]DocumentResponse, 0, len(items))
	for _, item := range items {
		response = append(response, documentResponse(item))
	}
	httptransport.OK(c, response)
	return nil
}

// GetDocument returns a knowledge projection and its optimistic revision.
// @Summary Get knowledge document
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Param id path int true "document ID"
// @Success 200 {object} ProposalResult[DocumentResponse]
func (handler *Handler) GetDocument(c *gin.Context) error {
	httptransport.SetModule(c, "knowledge")
	id, err := positivePathID(c, "document")
	if err != nil {
		return err
	}
	reader, ok := handler.reader.(interface {
		GetDocumentContext(context.Context, int64) (knowledgedomain.Document, error)
	})
	if !ok {
		return knowledgeError(sharedrepository.ErrUnavailable)
	}
	document, err := reader.GetDocumentContext(c.Request.Context(), id)
	if err != nil {
		return knowledgeError(err)
	}
	httptransport.OK(c, documentResponse(document))
	return nil
}

// ListProposals returns up to one hundred newest proposals.
// @Summary List knowledge proposals
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Param status query string false "proposal status"
// @Success 200 {object} ProposalResult[[]ProposalResponse]
func (handler *Handler) ListProposals(c *gin.Context) error {
	httptransport.SetModule(c, "knowledge")
	status := knowledgedomain.ProposalStatus(c.Query("status"))
	reader, ok := handler.reader.(interface {
		ListProposals(context.Context, knowledgedomain.ProposalStatus) ([]knowledgedomain.Proposal, error)
	})
	if !ok {
		return knowledgeError(sharedrepository.ErrUnavailable)
	}
	items, err := reader.ListProposals(c.Request.Context(), status)
	if err != nil {
		return knowledgeError(err)
	}
	response := make([]ProposalResponse, 0, len(items))
	for _, item := range items {
		response = append(response, proposalResponse(item))
	}
	httptransport.OK(c, response)
	return nil
}

// GetProposal returns one proposal including its proposed automatic body.
// @Summary Get knowledge proposal
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Param id path int true "proposal ID"
// @Success 200 {object} ProposalResult[ProposalResponse]
func (handler *Handler) GetProposal(c *gin.Context) error {
	httptransport.SetModule(c, "knowledge")
	id, err := positivePathID(c, "proposal")
	if err != nil {
		return err
	}
	proposal, err := handler.reader.GetProposal(c.Request.Context(), id)
	if err != nil {
		return knowledgeError(err)
	}
	httptransport.OK(c, proposalResponse(proposal))
	return nil
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
// @Success 200 {object} ProposalResult[ProposalResponse]
// @Failure 400 {object} ProposalResult[EmptyResponse]
// @Failure 401 {object} ProposalResult[EmptyResponse]
// @Failure 403 {object} ProposalResult[EmptyResponse]
// @Failure 409 {object} ProposalResult[EmptyResponse]
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
	httptransport.OK(c, proposalResponse(proposal))
	return nil
}

// Approve approves a pending proposal after optimistic-version validation.
// @Summary Approve knowledge proposal
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Param id path int true "proposal ID"
// @Param version query int true "proposal version"
// @Success 200 {object} ProposalResult[ProposalResponse]
// @Failure 400 {object} ProposalResult[EmptyResponse]
// @Failure 401 {object} ProposalResult[EmptyResponse]
// @Failure 403 {object} ProposalResult[EmptyResponse]
// @Failure 409 {object} ProposalResult[EmptyResponse]
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
// @Success 200 {object} ProposalResult[ProposalResponse]
// @Failure 400 {object} ProposalResult[EmptyResponse]
// @Failure 401 {object} ProposalResult[EmptyResponse]
// @Failure 403 {object} ProposalResult[EmptyResponse]
// @Failure 409 {object} ProposalResult[EmptyResponse]
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
	httptransport.OK(c, proposalResponse(proposal))
	return nil
}

// Apply writes an approved proposal using Vault atomic replacement.
// @Summary Apply knowledge proposal
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Param id path int true "proposal ID"
// @Param version query int true "proposal version"
// @Success 200 {object} ProposalResult[DocumentResponse]
// @Failure 400 {object} ProposalResult[EmptyResponse]
// @Failure 401 {object} ProposalResult[EmptyResponse]
// @Failure 403 {object} ProposalResult[EmptyResponse]
// @Failure 409 {object} ProposalResult[EmptyResponse]
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
	httptransport.OK(c, documentResponse(document))
	return nil
}

// Reconcile compares database projections with Vault files.
// @Summary Reconcile knowledge Vault
// @Tags knowledge
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ProposalResult[ReconciliationResponse]
// @Failure 401 {object} ProposalResult[EmptyResponse]
// @Failure 403 {object} ProposalResult[EmptyResponse]
// @Failure 503 {object} ProposalResult[EmptyResponse]
func (handler *Handler) Reconcile(c *gin.Context) error {
	httptransport.SetModule(c, "knowledge")
	report, err := handler.reconcile.Reconcile(c.Request.Context())
	if err != nil {
		return knowledgeError(err)
	}
	httptransport.OK(c, reconciliationResponse(report))
	return nil
}

func proposalPath(c *gin.Context) (int64, int64, error) {
	id, err := positivePathID(c, "proposal")
	if err != nil {
		return 0, 0, err
	}
	version, err := strconv.ParseInt(c.Query("version"), 10, 64)
	if err != nil || version <= 0 {
		return 0, 0, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "proposal version is required")
	}
	return id, version, nil
}

func positivePathID(c *gin.Context, resource string) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid "+resource+" id")
	}
	return id, nil
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
