package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type knowledgeRouteAuthenticator struct{ role httptransport.Role }

func (auth knowledgeRouteAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 1, SessionID: 2, Role: auth.role}, nil
}

type knowledgeRouteReader struct {
	documentQuery domain.DocumentListQuery
	proposalQuery domain.ProposalListQuery
	documentCalls int
	proposalCalls int
}

func (*knowledgeRouteReader) GetProposal(context.Context, int64) (domain.Proposal, error) {
	return domain.Proposal{}, nil
}

func (*knowledgeRouteReader) ListDocuments(context.Context) ([]domain.Document, error) {
	return []domain.Document{}, nil
}

func (reader *knowledgeRouteReader) ListDocumentPage(_ context.Context, query domain.DocumentListQuery) (domain.DocumentPage, error) {
	reader.documentCalls++
	reader.documentQuery = query
	return domain.DocumentPage{Items: []domain.Document{}, NextCursor: "documents-next"}, nil
}

func (reader *knowledgeRouteReader) ListProposalPage(_ context.Context, query domain.ProposalListQuery) (domain.ProposalPage, error) {
	reader.proposalCalls++
	reader.proposalQuery = query
	return domain.ProposalPage{Items: []domain.Proposal{}, NextCursor: "proposals-next"}, nil
}

type knowledgeRouteVault struct{}

func (knowledgeRouteVault) ListFiles() ([]domain.VaultFile, error) { return []domain.VaultFile{}, nil }

func TestKnowledgeRoutesEnforcePublisherAndReconciliationRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &knowledgeRouteReader{}
	reconciler := knowledgeapplication.NewReconciler(reader, knowledgeRouteVault{})
	handler := NewHandler(nil, reader, reconciler, nil)

	unauthenticated := gin.New()
	RegisterRoutes(unauthenticated, handler, knowledgeRouteAuthenticator{role: httptransport.RoleEditor})
	if response := knowledgeRouteRequest(unauthenticated, stdhttp.MethodGet, "/api/v1/knowledge/documents", ""); response.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("unauthenticated documents = %d, want 401", response.Code)
	}

	for _, role := range []httptransport.Role{httptransport.RoleViewer, httptransport.RoleAnalyst} {
		router := gin.New()
		RegisterRoutes(router, handler, knowledgeRouteAuthenticator{role: role})
		if response := knowledgeRouteRequest(router, stdhttp.MethodGet, "/api/v1/knowledge/documents", string(role)); response.Code != stdhttp.StatusForbidden {
			t.Errorf("%s documents = %d, want 403", role, response.Code)
		}
	}

	editor := gin.New()
	RegisterRoutes(editor, handler, knowledgeRouteAuthenticator{role: httptransport.RoleEditor})
	if response := knowledgeRouteRequest(editor, stdhttp.MethodGet, "/api/v1/knowledge/documents", "editor"); response.Code != stdhttp.StatusOK {
		t.Fatalf("editor documents = %d: %s", response.Code, response.Body.String())
	}
	if response := knowledgeRouteRequest(editor, stdhttp.MethodPost, "/api/v1/knowledge/reconcile", "editor"); response.Code != stdhttp.StatusForbidden {
		t.Fatalf("editor reconcile = %d, want 403", response.Code)
	}

	admin := gin.New()
	RegisterRoutes(admin, handler, knowledgeRouteAuthenticator{role: httptransport.RoleAdmin})
	if response := knowledgeRouteRequest(admin, stdhttp.MethodPost, "/api/v1/knowledge/reconcile", "admin"); response.Code != stdhttp.StatusOK {
		t.Fatalf("admin reconcile = %d: %s", response.Code, response.Body.String())
	}
}

func TestKnowledgeListRoutesForwardOpaqueCursorsAndRejectInvalidQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &knowledgeRouteReader{}
	handler := NewHandler(nil, reader, knowledgeapplication.NewReconciler(reader, knowledgeRouteVault{}), nil)
	router := gin.New()
	RegisterRoutes(router, handler, knowledgeRouteAuthenticator{role: httptransport.RoleEditor})

	documents := knowledgeRouteRequest(router, stdhttp.MethodGet, "/api/v1/knowledge/documents?cursor=document-page-2&limit=2", "editor")
	if documents.Code != stdhttp.StatusOK || reader.documentQuery.Cursor != "document-page-2" || reader.documentQuery.Limit != 2 {
		t.Fatalf("documents response/query = %d/%#v: %s", documents.Code, reader.documentQuery, documents.Body.String())
	}
	var documentBody struct {
		Data struct {
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(documents.Body.Bytes(), &documentBody); err != nil || documentBody.Data.NextCursor != "documents-next" {
		t.Fatalf("document page body = %#v/%v", documentBody, err)
	}

	proposals := knowledgeRouteRequest(router, stdhttp.MethodGet, "/api/v1/knowledge/proposals?cursor=proposal-page-2&limit=3&status=pending", "editor")
	if proposals.Code != stdhttp.StatusOK || reader.proposalQuery.Cursor != "proposal-page-2" || reader.proposalQuery.Limit != 3 || reader.proposalQuery.Status != domain.ProposalPending {
		t.Fatalf("proposals response/query = %d/%#v: %s", proposals.Code, reader.proposalQuery, proposals.Body.String())
	}

	documentCalls, proposalCalls := reader.documentCalls, reader.proposalCalls
	for _, path := range []string{
		"/api/v1/knowledge/documents?limit=201",
		"/api/v1/knowledge/proposals?limit=0",
		"/api/v1/knowledge/proposals?status=unknown",
	} {
		response := knowledgeRouteRequest(router, stdhttp.MethodGet, path, "editor")
		if response.Code != stdhttp.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400: %s", path, response.Code, response.Body.String())
		}
	}
	if reader.documentCalls != documentCalls || reader.proposalCalls != proposalCalls {
		t.Fatalf("invalid queries reached reader: document=%d/%d proposal=%d/%d", reader.documentCalls, documentCalls, reader.proposalCalls, proposalCalls)
	}
}

func knowledgeRouteRequest(router *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
