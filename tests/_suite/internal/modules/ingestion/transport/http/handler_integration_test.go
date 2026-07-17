package http

import (
	"context"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/postgres"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/observability"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
	"github.com/gin-gonic/gin"
)

func TestContentRoutesPostgresIntegrationExposeOnlyActiveSafeContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runtime := openContentHTTPRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentHTTPSource(t, runtime)
	activeInput := contentHTTPInput(sourceID, "active-item", time.Date(2026, time.July, 17, 9, 0, 0, 0, time.UTC))
	active, _, err := repository.Upsert(context.Background(), activeInput, ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive})
	if err != nil {
		t.Fatalf("Upsert(active) error = %v", err)
	}
	const objectKey = "evidence/v1/1/aa/private-object.txt"
	if err := repository.CreateAsset(context.Background(), ingestiondomain.ContentAsset{
		ContentID: active.ID, AssetType: "text", ObjectKey: objectKey, OriginalURL: activeInput.CanonicalURL,
		MIMEType: "text/plain", SHA256: strings.Repeat("a", 64), SizeBytes: 1,
		CapturedAt: activeInput.FetchedAt, Status: ingestiondomain.AssetStatusAvailable,
	}); err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}
	deletedInput := contentHTTPInput(sourceID, "deleted-item", activeInput.FetchedAt.Add(time.Minute))
	deleted, _, err := repository.Upsert(context.Background(), deletedInput, ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive})
	if err != nil {
		t.Fatalf("Upsert(deleted) error = %v", err)
	}
	if _, changed, err := repository.MarkDeleted(context.Background(), sourceID, deletedInput.ExternalID); err != nil || !changed {
		t.Fatalf("MarkDeleted() changed/error = %v / %v", changed, err)
	}

	service, err := ingestionapplication.NewContentQueryService(ingestionapplication.ContentQueryDependencies{
		Contents: repository,
		Sources:  contentHTTPSourceReader{reference: sourcedomain.ContentSourceReference{Name: "Integration RSS", SourceType: sourcedomain.SourceTypeRSS}},
	})
	if err != nil {
		t.Fatalf("NewContentQueryService() error = %v", err)
	}
	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}
	router := gin.New()
	RegisterRoutes(router, service, contentAuthenticator{role: httptransport.RoleViewer}, metrics)

	list := performContentRequest(router, stdhttp.MethodGet, "/api/v1/contents?limit=10", "viewer")
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), objectKey) || strings.Contains(list.Body.String(), activeInput.Excerpt) || strings.Contains(list.Body.String(), "private-body") {
		t.Fatalf("list response leaked persistence/evidence field: %s", list.Body.String())
	}
	var listEnvelope struct {
		Data ContentPageResponse `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listEnvelope.Data.Items) != 1 || listEnvelope.Data.Items[0].ID != active.ID || listEnvelope.Data.Items[0].SourceName != "Integration RSS" {
		t.Fatalf("active safe list = %#v, want active source projection", listEnvelope.Data.Items)
	}

	getDeleted := performContentRequest(router, stdhttp.MethodGet, fmt.Sprintf("/api/v1/contents/%d", deleted.ID), "viewer")
	if getDeleted.Code != stdhttp.StatusNotFound {
		t.Fatalf("deleted get status = %d, want 404: %s", getDeleted.Code, getDeleted.Body.String())
	}
}

type contentHTTPSourceReader struct {
	reference sourcedomain.ContentSourceReference
}

func (reader contentHTTPSourceReader) FindForContent(context.Context, int64) (sourcedomain.ContentSourceReference, error) {
	return reader.reference, nil
}

func openContentHTTPRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}
	return runtime
}

func createContentHTTPSource(t *testing.T, runtime *database.Runtime) int64 {
	t.Helper()
	var sourceID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_connections (source_type, name, endpoint)
VALUES ('rss', $1, 'https://feeds.example.test/content-http')
RETURNING id`, fmt.Sprintf("content-http-%d", time.Now().UnixNano())).Scan(&sourceID); err != nil {
		t.Fatalf("create source: %v", err)
	}
	return sourceID
}

func contentHTTPInput(sourceID int64, externalID string, observedAt time.Time) ingestiondomain.NormalizedContent {
	return ingestiondomain.NormalizedContent{
		SourceConnectionID: sourceID, ExternalID: externalID, ContentType: "article", Title: "Safe " + externalID,
		Excerpt: "private excerpt " + externalID, Body: "private-body " + externalID,
		CanonicalURL: "https://example.test/contents/" + externalID, Language: "en",
		PublishedAt: observedAt, FetchedAt: observedAt, ContentHash: strings.Repeat("c", 64),
	}
}
