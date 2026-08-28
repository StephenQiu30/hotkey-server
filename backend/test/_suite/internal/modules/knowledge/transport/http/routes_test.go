package http

import (
	"context"
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

type knowledgeRouteReader struct{}

func (knowledgeRouteReader) GetProposal(context.Context, int64) (domain.Proposal, error) {
	return domain.Proposal{}, nil
}

func (knowledgeRouteReader) ListDocuments(context.Context) ([]domain.Document, error) {
	return []domain.Document{}, nil
}

type knowledgeRouteVault struct{}

func (knowledgeRouteVault) ListFiles() ([]domain.VaultFile, error) { return []domain.VaultFile{}, nil }

func TestKnowledgeRoutesEnforcePublisherAndReconciliationRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := knowledgeRouteReader{}
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

func knowledgeRouteRequest(router *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
