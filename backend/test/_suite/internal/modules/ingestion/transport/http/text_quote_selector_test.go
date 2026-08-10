package http

import (
	"context"
	"crypto/sha256"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type quoteSelectorHTTPRepositoryFake struct {
	plaintext string
	digest    string
}

func (fake *quoteSelectorHTTPRepositoryFake) ReadTextQuoteSelectorTarget(_ context.Context, query ingestionapplication.TextQuoteSelectorTargetQuery) (ingestionapplication.TextQuoteSelectorTargetDTO, error) {
	return ingestionapplication.TextQuoteSelectorTargetDTO{
		SourceConnectionID: 3, DocumentID: 5, DocumentVersionID: query.DocumentVersionID,
		ContentSHA256: fake.digest, DocumentLifecycleState: ingestionapplication.DocumentReadable,
		PlaintextArtifact: ingestionapplication.TextQuoteProjectionArtifactDTO{ID: 7,
			ArtifactType: ingestionapplication.DocumentProjectionPlaintext, TransformerProfileSHA256: strings.Repeat("a", 64),
			MIMEType: "text/plain; charset=utf-8", SHA256: fake.digest, SizeBytes: int64(len(fake.plaintext)),
			RetentionUntil: query.DecisionAt.Add(time.Hour)},
		MarkdownArtifactID: 9, AnchorMapSHA256: strings.Repeat("b", 64),
		AnchorBlocks: []ingestionapplication.TextQuoteAnchorBlockDTO{{Ordinal: 0, PlaintextUTF8ByteStart: 0,
			PlaintextUTF8ByteEnd: int64(len(fake.plaintext)), MarkdownAnchor: "body-0000-111111111111"}},
		QuoteRightsDecisionID: 11, RetainRightsDecisionID: 13,
		RetentionUntil: query.DecisionAt.Add(time.Hour), DecisionAt: query.DecisionAt,
	}, nil
}

func (fake *quoteSelectorHTTPRepositoryFake) PersistTextQuoteSelector(_ context.Context, command ingestionapplication.PersistTextQuoteSelectorCommand) (ingestionapplication.TextQuoteSelectorDTO, error) {
	return ingestionapplication.TextQuoteSelectorDTO{ID: 17, Version: 1, SourceConnectionID: command.SourceConnectionID,
		DocumentVersionID: command.DocumentVersionID, PlaintextArtifactID: command.PlaintextArtifactID,
		MarkdownArtifactID: command.MarkdownArtifactID, QuoteRightsDecisionID: command.QuoteRightsDecisionID,
		RetainRightsDecisionID: command.RetainRightsDecisionID, ExactQuote: command.ExactQuote, Prefix: command.Prefix,
		Suffix: command.Suffix, UTF8ByteStart: command.UTF8ByteStart, UTF8ByteEnd: command.UTF8ByteEnd,
		QuoteSHA256: command.QuoteSHA256, PlaintextSHA256: command.PlaintextSHA256,
		NormalizationVersion: command.NormalizationVersion, SelectorVersion: command.SelectorVersion,
		AnchorMapSHA256: command.AnchorMapSHA256, MarkdownAnchor: command.MarkdownAnchor,
		RetentionUntil: command.RetentionUntil, CreatedAt: command.DecisionAt,
	}, nil
}

type quoteSelectorProjectionReaderFake struct{ plaintext, digest string }

func (fake quoteSelectorProjectionReaderFake) ReadDocumentProjection(context.Context, knowledgeapplication.DocumentProjectionQueryDTO) (knowledgeapplication.DocumentProjectionResultDTO, error) {
	return knowledgeapplication.DocumentProjectionResultDTO{Content: fake.plaintext, MIMEType: "text/plain; charset=utf-8",
		SHA256: fake.digest, SizeBytes: int64(len(fake.plaintext))}, nil
}

type quoteSelectorAuthenticator struct{ role httptransport.Role }

func (auth quoteSelectorAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 7, SessionID: 8, Role: auth.role}, nil
}

func TestTextQuoteSelectorRouteLocatesUniqueQuoteAndReturnsSafeProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	plaintext := "首段。\n\nCafé 发布新模型。"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext)))
	repository := &quoteSelectorHTTPRepositoryFake{plaintext: plaintext, digest: digest}
	service, err := ingestionapplication.NewTextQuoteSelectorService(ingestionapplication.TextQuoteSelectorDependencies{
		Repository: repository, Projections: quoteSelectorProjectionReaderFake{plaintext: plaintext, digest: digest},
	})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterTextQuoteSelectorRoutes(router, service, quoteSelectorAuthenticator{role: httptransport.RoleEditor})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/document-versions/19/text-quote-selectors",
		strings.NewReader(fmt.Sprintf(`{"exact_quote":"Café 发布新模型","plaintext_sha256":"%s","normalization_version":"nfc-lf-collapse-space-v1"}`, digest)))
	request.Header.Set("Authorization", "Bearer editor")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", fmt.Sprintf("\"%s\"", digest))
	request.Header.Set("Idempotency-Key", "quote-selector-fixture")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{`"id":17`, `"document_version_id":19`, `"exact_quote":"Café 发布新模型"`, `"markdown_anchor":"body-0000-111111111111"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response missing %s: %s", expected, body)
		}
	}
	for _, forbidden := range []string{"quote_rights_decision_id", "retain_rights_decision_id", "artifact_id", "object_key"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %s: %s", forbidden, body)
		}
	}
}

func TestTextQuoteSelectorRouteRequiresEditorAndStrongPlaintextETag(t *testing.T) {
	plaintext := "唯一引用"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext)))
	repository := &quoteSelectorHTTPRepositoryFake{plaintext: plaintext, digest: digest}
	service, _ := ingestionapplication.NewTextQuoteSelectorService(ingestionapplication.TextQuoteSelectorDependencies{
		Repository: repository, Projections: quoteSelectorProjectionReaderFake{plaintext: plaintext, digest: digest},
	})
	for _, test := range []struct {
		name string
		role httptransport.Role
		etag string
		want int
	}{
		{name: "viewer forbidden", role: httptransport.RoleViewer, etag: fmt.Sprintf("\"%s\"", digest), want: stdhttp.StatusForbidden},
		{name: "weak etag rejected", role: httptransport.RoleEditor, etag: fmt.Sprintf("W/\"%s\"", digest), want: stdhttp.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			RegisterTextQuoteSelectorRoutes(router, service, quoteSelectorAuthenticator{role: test.role})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/document-versions/19/text-quote-selectors",
				strings.NewReader(fmt.Sprintf(`{"exact_quote":"唯一引用","plaintext_sha256":"%s","normalization_version":"nfc-lf-collapse-space-v1"}`, digest)))
			request.Header.Set("Authorization", "Bearer actor")
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("If-Match", test.etag)
			request.Header.Set("Idempotency-Key", "quote-selector-fixture")
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}
