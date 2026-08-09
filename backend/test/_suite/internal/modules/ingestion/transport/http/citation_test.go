package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

func TestVersionedCitationRoutesReturnOnlySafeExactVersionFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	artifactSHA := strings.Repeat("a", 64)
	citation := transportCitationDTO(41, artifactSHA)
	service := &versionedCitationServiceStub{
		citation: ingestionapplication.CitationResult{Citation: citation},
		document: ingestionapplication.DocumentResult{
			Citation: citation, Markdown: "# verified\n", ETag: `"` + artifactSHA + `"`,
		},
	}
	router := newVersionedCitationRouter(service)

	unauthenticated := performVersionedCitationRequest(router, stdhttp.MethodGet, "/api/v1/document-versions/41/citation", "", "")
	if unauthenticated.Code != stdhttp.StatusUnauthorized || service.citationCalls != 0 {
		t.Fatalf("unauthenticated status/calls = %d/%d", unauthenticated.Code, service.citationCalls)
	}

	response := performVersionedCitationRequest(router, stdhttp.MethodGet, "/api/v1/document-versions/41/citation", "viewer", "")
	if response.Code != stdhttp.StatusOK || service.lastCitationQuery.DocumentVersionID != 41 {
		t.Fatalf("citation status/query = %d/%#v: %s", response.Code, service.lastCitationQuery, response.Body.String())
	}
	assertVersionedResponseDoesNotLeakInternals(t, response.Body.String())
	var envelope struct {
		Code int                 `json:"code"`
		Data CitationResponseDTO `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 0 || envelope.Data.DocumentVersionID != 41 || envelope.Data.Publisher != nil ||
		envelope.Data.PublisherAvailability != "unavailable" || envelope.Data.PublisherUnavailableReason == nil ||
		*envelope.Data.PublisherUnavailableReason != "publisher_unavailable" ||
		envelope.Data.LocatorAvailability != "unavailable" || envelope.Data.ExactQuote != nil ||
		envelope.Data.Artifact == nil || envelope.Data.Artifact.SHA256 != artifactSHA {
		t.Fatalf("citation response = %#v", envelope.Data)
	}

	document := performVersionedCitationRequest(router, stdhttp.MethodGet, "/api/v1/document-versions/41/document", "viewer", "")
	if document.Code != stdhttp.StatusOK || document.Header().Get("ETag") != `"`+artifactSHA+`"` ||
		!strings.Contains(document.Body.String(), `"markdown":"# verified\n"`) || service.lastDocumentQuery.DocumentVersionID != 41 {
		t.Fatalf("document response/query = %d %s %#v", document.Code, document.Body.String(), service.lastDocumentQuery)
	}
	assertVersionedResponseDoesNotLeakInternals(t, document.Body.String())
}

func TestVersionedDocumentRouteValidatesStrongETagAndEmitsVerified304(t *testing.T) {
	gin.SetMode(gin.TestMode)
	artifactSHA := strings.Repeat("a", 64)
	service := &versionedCitationServiceStub{document: ingestionapplication.DocumentResult{
		Citation: transportCitationDTO(41, artifactSHA), ETag: `"` + artifactSHA + `"`, NotModified: true,
	}}
	router := newVersionedCitationRouter(service)
	response := performVersionedCitationRequest(router, stdhttp.MethodGet, "/api/v1/document-versions/41/document", "viewer", `"`+artifactSHA+`"`)
	if response.Code != stdhttp.StatusNotModified || response.Body.Len() != 0 || response.Header().Get("ETag") != `"`+artifactSHA+`"` ||
		service.lastDocumentQuery.IfNoneMatch != `"`+artifactSHA+`"` {
		t.Fatalf("304 response/query = %d %q %q %#v", response.Code, response.Body.String(), response.Header().Get("ETag"), service.lastDocumentQuery)
	}

	for _, invalid := range []string{"W/\"" + artifactSHA + "\"", artifactSHA, `"abc"`, "*"} {
		service.documentCalls = 0
		response = performVersionedCitationRequest(router, stdhttp.MethodGet, "/api/v1/document-versions/41/document", "viewer", invalid)
		if response.Code != stdhttp.StatusBadRequest || service.documentCalls != 0 {
			t.Fatalf("invalid ETag %q status/calls = %d/%d: %s", invalid, response.Code, service.documentCalls, response.Body.String())
		}
		assertContentErrorResponse(t, response, sharederrors.CodeInvalidRequest)
	}
}

func TestVersionedDocumentRouteMapsSemanticFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		kind       ingestionapplication.DocumentReadFailureKind
		wantStatus int
		wantCode   int
		message    string
	}{
		{kind: ingestionapplication.DocumentReadFailureNotReadable, wantStatus: 409, wantCode: sharederrors.CodeConflict, message: "document is not readable"},
		{kind: ingestionapplication.DocumentReadFailurePolicy, wantStatus: 403, wantCode: sharederrors.CodeForbidden, message: "document policy blocked"},
		{kind: ingestionapplication.DocumentReadFailurePermission, wantStatus: 403, wantCode: sharederrors.CodeForbidden, message: "document permission denied"},
		{kind: ingestionapplication.DocumentReadFailureRetention, wantStatus: 404, wantCode: sharederrors.CodeNotFound, message: "document retention unavailable"},
		{kind: ingestionapplication.DocumentReadFailureMissing, wantStatus: 404, wantCode: sharederrors.CodeNotFound, message: "document artifact not found"},
		{kind: ingestionapplication.DocumentReadFailureIntegrity, wantStatus: 502, wantCode: sharederrors.CodeBadGateway, message: "document projection integrity failure"},
		{kind: ingestionapplication.DocumentReadFailureUnavailable, wantStatus: 503, wantCode: sharederrors.CodeUnavailable, message: "document projection unavailable"},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.kind), func(t *testing.T) {
			service := &versionedCitationServiceStub{documentErr: &ingestionapplication.DocumentReadError{Kind: test.kind}}
			response := performVersionedCitationRequest(newVersionedCitationRouter(service), stdhttp.MethodGet, "/api/v1/document-versions/41/document", "viewer", "")
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), `"message":"`+test.message+`"`) {
				t.Fatalf("response = %d %s, want %d/%q", response.Code, response.Body.String(), test.wantStatus, test.message)
			}
			assertContentErrorResponse(t, response, test.wantCode)
			assertVersionedResponseDoesNotLeakInternals(t, response.Body.String())
		})
	}
}

func TestUnavailableCitationOmitsContentDerivedMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	citation := transportCitationDTO(41, strings.Repeat("a", 64))
	citation.Availability = ingestionapplication.CitationPolicyBlocked
	citation.UnavailableReason = ingestionapplication.CitationReasonPermissionDenied
	citation.ContentSHA256 = nil
	citation.Artifact = nil
	service := &versionedCitationServiceStub{citation: ingestionapplication.CitationResult{Citation: citation}}
	response := performVersionedCitationRequest(newVersionedCitationRouter(service), stdhttp.MethodGet, "/api/v1/document-versions/41/citation", "viewer", "")
	if response.Code != stdhttp.StatusOK || !strings.Contains(response.Body.String(), `"content_sha256":null`) ||
		!strings.Contains(response.Body.String(), `"artifact":null`) || strings.Contains(response.Body.String(), `"etag"`) {
		t.Fatalf("unavailable citation leaked content-derived metadata: %d %s", response.Code, response.Body.String())
	}
}

func TestVersionedCitationRoutesRejectInvalidIDBeforeApplication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &versionedCitationServiceStub{}
	for _, suffix := range []string{"0", "-1", "not-a-number"} {
		response := performVersionedCitationRequest(newVersionedCitationRouter(service), stdhttp.MethodGet, "/api/v1/document-versions/"+suffix+"/citation", "viewer", "")
		if response.Code != stdhttp.StatusBadRequest || service.citationCalls != 0 {
			t.Fatalf("id %q response/calls = %d/%d", suffix, response.Code, service.citationCalls)
		}
	}
}

func transportCitationDTO(documentVersionID int64, artifactSHA string) ingestionapplication.CitationDTO {
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	author := "Ada"
	sourceRecordURL := "https://feed.example.test/items/41"
	canonicalURL := "https://publisher.example.test/articles/41"
	discussionURL := "https://forum.example.test/discussions/41"
	contentSHA := strings.Repeat("1", 64)
	return ingestionapplication.CitationDTO{
		DocumentID: 11, DocumentVersionID: documentVersionID, SourceType: "rss", SourceName: "Product feed",
		Title: "Verified title", Author: &author,
		PublisherAvailability:      ingestionapplication.CitationFactUnavailable,
		PublisherUnavailableReason: ingestionapplication.CitationReasonPublisherUnavailable,
		SourceRecordURL:            &sourceRecordURL, CanonicalURL: &canonicalURL, DiscussionURL: &discussionURL,
		BodyOrigin: ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
		Language: "en", PublishedAt: &now, CapturedAt: now, ContentSHA256: &contentSHA,
		Availability: ingestionapplication.CitationFullArchive,
		Artifact: &ingestionapplication.CitationArtifactDTO{
			ArtifactType: "markdown", TransformerProfileSHA256: strings.Repeat("b", 64),
			MIMEType: "text/markdown; charset=utf-8", SHA256: artifactSHA, SizeBytes: 11, ETag: `"` + artifactSHA + `"`,
		},
		LocatorAvailability:      ingestionapplication.CitationFactUnavailable,
		LocatorUnavailableReason: ingestionapplication.CitationReasonLocatorUnavailable,
	}
}

type versionedCitationServiceStub struct {
	citation          ingestionapplication.CitationResult
	document          ingestionapplication.DocumentResult
	citationErr       error
	documentErr       error
	lastCitationQuery ingestionapplication.CitationQuery
	lastDocumentQuery ingestionapplication.DocumentQuery
	citationCalls     int
	documentCalls     int
}

func (service *versionedCitationServiceStub) GetCitation(_ context.Context, query ingestionapplication.CitationQuery) (ingestionapplication.CitationResult, error) {
	service.citationCalls++
	service.lastCitationQuery = query
	return service.citation, service.citationErr
}

func (service *versionedCitationServiceStub) GetDocument(_ context.Context, query ingestionapplication.DocumentQuery) (ingestionapplication.DocumentResult, error) {
	service.documentCalls++
	service.lastDocumentQuery = query
	return service.document, service.documentErr
}

func newVersionedCitationRouter(service versionedCitationHTTPService) *gin.Engine {
	router := gin.New()
	RegisterCitationRoutes(router, service, contentAuthenticator{role: httptransport.RoleViewer})
	return router
}

func performVersionedCitationRequest(router *gin.Engine, method, path, token, ifNoneMatch string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if ifNoneMatch != "" {
		request.Header.Set("If-None-Match", ifNoneMatch)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertVersionedResponseDoesNotLeakInternals(t *testing.T, body string) {
	t.Helper()
	for _, forbidden := range []string{
		"object_key", "vault_relative_path", "minio", "credential", "raw_payload",
		"store_derived_rights_decision_id", "retain_rights_decision_id", "source_connection_id",
	} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
}
