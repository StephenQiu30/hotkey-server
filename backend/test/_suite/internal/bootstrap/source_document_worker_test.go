package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/jobs"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func TestSourceEvidenceReaderAdapterCopiesEveryApplicationFactDefensively(t *testing.T) {
	t.Parallel()

	publishedAt := time.Date(2026, time.August, 9, 10, 0, 0, 0, time.FixedZone("source", 8*60*60))
	payload := []byte(`<rssItem><encoded>body</encoded></rssItem>`)
	selected := sourceapplication.SelectedEvidenceDTO{
		EvidenceReferenceID: 71, SourceObservationID: 41, EvidenceSnapshotID: 51, SourceConnectionID: 7,
		ExternalID: "article-41", UpstreamIdentity: strings.Repeat("b", 64), SourceCode: "rss", ContentType: "article",
		Title: "Launch", Language: "en", Author: "Reporter", SourceRecordURL: "https://feed.example.test/rss.xml",
		CanonicalURL: "https://news.example.test/launch", DiscussionURL: "https://forum.example.test/launch",
		BodyOrigin: "feed_content", Completeness: "full", PublishedAt: &publishedAt,
		DiscoveredAt: publishedAt.Add(time.Minute), CapturedAt: publishedAt.Add(2 * time.Minute),
		SelectedPayload: payload, SelectedPayloadSHA256: strings.Repeat("a", 64),
		PayloadMIMEType: "application/rss+xml", SelectorVersion: "rss2-go-xml-v1",
	}
	reader := &sourceEvidenceSelectionReaderFake{result: sourceapplication.EvidenceSelectionResult{Evidence: selected}}
	adapter, err := newSourceEvidenceReaderAdapter(reader)
	if err != nil {
		t.Fatalf("newSourceEvidenceReaderAdapter() error = %v", err)
	}

	result, err := adapter.ReadSelectedSourceEvidence(context.Background(), ingestionapplication.SourceEvidenceQuery{EvidenceReferenceID: 71})
	if err != nil {
		t.Fatalf("ReadSelectedSourceEvidence() error = %v", err)
	}
	if reader.query.EvidenceReferenceID != 71 || result.EvidenceReferenceID != 71 || result.SourceObservationID != 41 ||
		result.EvidenceSnapshotID != 51 || result.SourceConnectionID != 7 || result.ExternalWorkID != "article-41" ||
		result.UpstreamIdentity != selected.UpstreamIdentity ||
		result.SourceCode != "rss" || result.ContentType != "article" || result.Title != "Launch" || result.Language != "en" ||
		result.Author != "Reporter" || result.SourceRecordURL != selected.SourceRecordURL || result.CanonicalURL != selected.CanonicalURL ||
		result.DiscussionURL != selected.DiscussionURL || result.BodyOrigin != ingestionapplication.BodyOriginFeedContent ||
		result.Completeness != ingestionapplication.BodyCompletenessFull || result.PublishedAt == nil ||
		*result.PublishedAt != publishedAt || result.DiscoveredAt != selected.DiscoveredAt || result.CapturedAt != selected.CapturedAt ||
		string(result.SelectedPayload) != string(payload) || result.SelectedPayloadSHA256 != selected.SelectedPayloadSHA256 ||
		result.PayloadMIMEType != selected.PayloadMIMEType || result.SelectorVersion != selected.SelectorVersion {
		t.Fatalf("mapped selected evidence = %#v", result)
	}
	reader.result.Evidence.SelectedPayload[0] = 'X'
	*reader.result.Evidence.PublishedAt = time.Time{}
	if result.SelectedPayload[0] == 'X' || result.PublishedAt.IsZero() {
		t.Fatal("adapter result aliases Source Application bytes or time pointers")
	}
	result.SelectedPayload[1] = 'Y'
	if reader.result.Evidence.SelectedPayload[1] == 'Y' {
		t.Fatal("Source Application payload aliases adapter result")
	}
}

func TestSourceEvidenceReaderAdapterFailsClosedOnInvalidMappedSemantics(t *testing.T) {
	t.Parallel()

	reader := &sourceEvidenceSelectionReaderFake{result: sourceapplication.EvidenceSelectionResult{Evidence: sourceapplication.SelectedEvidenceDTO{
		EvidenceReferenceID: 71, BodyOrigin: "invented_fulltext", Completeness: "full",
	}}}
	adapter, err := newSourceEvidenceReaderAdapter(reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ReadSelectedSourceEvidence(context.Background(), ingestionapplication.SourceEvidenceQuery{EvidenceReferenceID: 71}); !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("invalid Source projection error = %v, want constraint", err)
	}
}

func TestMinIOWorkerFxGraphConstructsSourceDocumentAndMatchServices(t *testing.T) {
	dsn := initializedBootstrapDatabase(t)
	cfg := config.Default()
	cfg.Role, cfg.HTTPAddr, cfg.DatabaseURL = string(RoleWorker), "", dsn
	cfg.VaultPath = t.TempDir()
	cfg.MinIO = config.MinIOConfig{
		Endpoint: "127.0.0.1:9000", Bucket: "test-evidence", AccessKey: "test-access", SecretKey: "test-secret",
	}
	var handler *ingestionjobs.SourceDocumentGenerationHandler
	var recallProjections *ingestionapplication.DocumentRecallProjectionService
	var publishedMatches *ingestionapplication.PublishedDocumentMatchService
	var publishedMatchEvaluations *ingestionapplication.PublishedMatchEvaluationService
	var matchReviews *ingestionapplication.DocumentMatchReviewService
	var publishedMatchHandler *ingestionjobs.PublishedDocumentMatchEvaluationHandler
	var acceptedMatchProjectionHandler *ingestionjobs.AcceptedDocumentMatchProjectionHandler
	var handlers map[string]queue.Handler
	app, err := NewAppWithReadiness(
		cfg,
		zap.NewNop(),
		httptransport.ReadinessFunc(func(context.Context) error { return nil }),
		fx.Populate(&handler, &recallProjections, &publishedMatches, &publishedMatchEvaluations, &matchReviews,
			&publishedMatchHandler, &acceptedMatchProjectionHandler, &handlers),
	)
	if err != nil {
		t.Fatalf("NewAppWithReadiness() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("MinIO worker Start() error = %v", err)
	}
	defer func() { _ = app.Stop(ctx) }()
	if handler == nil || recallProjections == nil || publishedMatches == nil || publishedMatchEvaluations == nil || matchReviews == nil ||
		publishedMatchHandler == nil || acceptedMatchProjectionHandler == nil || handlers[queue.KindGenerateSourceDocument] == nil ||
		handlers[queue.KindEvaluatePublishedDocumentMatches] == nil || handlers[queue.KindProjectAcceptedDocumentMatch] == nil {
		t.Fatalf("source document/match services/registration = %#v/%#v/%#v/%#v/%#v/%#v/%#v/%#v/%#v/%#v",
			handler, recallProjections, publishedMatches, publishedMatchEvaluations, matchReviews, publishedMatchHandler,
			acceptedMatchProjectionHandler, handlers[queue.KindGenerateSourceDocument],
			handlers[queue.KindEvaluatePublishedDocumentMatches], handlers[queue.KindProjectAcceptedDocumentMatch])
	}
}

type sourceEvidenceSelectionReaderFake struct {
	query  sourceapplication.EvidenceSelectionQuery
	result sourceapplication.EvidenceSelectionResult
	err    error
}

func (reader *sourceEvidenceSelectionReaderFake) Read(_ context.Context, query sourceapplication.EvidenceSelectionQuery) (sourceapplication.EvidenceSelectionResult, error) {
	reader.query = query
	return reader.result, reader.err
}
