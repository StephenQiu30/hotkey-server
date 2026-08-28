package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	eventjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/infrastructure/jobs"
	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/jobs"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	knowledgejobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/jobs"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func TestAcceptedMatchProjectionAdapterSchedulesIndependentEvidenceAfterEventCommit(t *testing.T) {
	projector := &acceptedDocumentMatchEventProjectorFake{result: eventapplication.ProjectAcceptedDocumentMatchResult{
		MicroEvent: eventapplication.MicroEventDTO{ID: 7, Version: 3},
		Membership: eventapplication.MicroEventMembershipDecisionDTO{Action: "join"},
	}}
	scheduler := &automaticClaimEvidenceSchedulerFake{}
	refresh := &productEventRefreshSchedulerBootstrapFake{}
	adapter, err := newAcceptedDocumentMatchEventProjectionAdapter(projector, scheduler, refresh)
	if err != nil || adapter == nil {
		t.Fatalf("newAcceptedDocumentMatchEventProjectionAdapter() = %#v/%v", adapter, err)
	}
	consumed, err := adapter.ConsumeAcceptedDocumentMatch(t.Context(), ingestionapplication.ConsumeAcceptedDocumentMatchCommand{
		DocumentMatchDecisionID: 5, DocumentVersionID: 11,
	})
	if err != nil || consumed.DocumentMatchDecisionID != 5 || consumed.DocumentVersionID != 11 ||
		projector.command.DocumentMatchDecisionID != 5 || projector.command.DocumentVersionID != 11 ||
		scheduler.command != (eventapplication.ScheduleAutomaticClaimEvidenceCommand{MicroEventID: 7, DocumentVersionID: 11}) ||
		refresh.command != (eventapplication.ScheduleProductEventRefreshCommand{MicroEventID: 7, ExpectedEventVersion: 3}) {
		t.Fatalf("consumed/projected/scheduled = %#v / %#v / %#v / %#v / %v", consumed, projector.command, scheduler.command, refresh.command, err)
	}
}

func TestAcceptedMatchProjectionAdapterSkipsEvidenceForReviewMembership(t *testing.T) {
	projector := &acceptedDocumentMatchEventProjectorFake{result: eventapplication.ProjectAcceptedDocumentMatchResult{
		MicroEvent: eventapplication.MicroEventDTO{ID: 7, Version: 3},
		Membership: eventapplication.MicroEventMembershipDecisionDTO{Action: "review"},
	}}
	scheduler := &automaticClaimEvidenceSchedulerFake{}
	refresh := &productEventRefreshSchedulerBootstrapFake{}
	adapter, err := newAcceptedDocumentMatchEventProjectionAdapter(projector, scheduler, refresh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ConsumeAcceptedDocumentMatch(t.Context(), ingestionapplication.ConsumeAcceptedDocumentMatchCommand{
		DocumentMatchDecisionID: 5, DocumentVersionID: 11,
	}); err != nil || scheduler.calls != 0 || refresh.calls != 0 {
		t.Fatalf("review consume error/calls = %v/%d/%d", err, scheduler.calls, refresh.calls)
	}
}

type productEventRefreshSchedulerBootstrapFake struct {
	command eventapplication.ScheduleProductEventRefreshCommand
	calls   int
}

func (fake *productEventRefreshSchedulerBootstrapFake) ScheduleProductEventRefresh(_ context.Context,
	command eventapplication.ScheduleProductEventRefreshCommand) (eventapplication.ScheduleProductEventRefreshResult, error) {
	fake.calls++
	fake.command = command
	return eventapplication.ScheduleProductEventRefreshResult{MicroEventID: command.MicroEventID,
		MicroEventVersion: command.ExpectedEventVersion, JobID: 2, Created: true, Available: true}, nil
}

type acceptedDocumentMatchEventProjectorFake struct {
	command eventapplication.ProjectAcceptedDocumentMatchCommand
	result  eventapplication.ProjectAcceptedDocumentMatchResult
	err     error
}

func (fake *acceptedDocumentMatchEventProjectorFake) Project(_ context.Context,
	command eventapplication.ProjectAcceptedDocumentMatchCommand) (eventapplication.ProjectAcceptedDocumentMatchResult, error) {
	fake.command = command
	return fake.result, fake.err
}

type automaticClaimEvidenceSchedulerFake struct {
	command eventapplication.ScheduleAutomaticClaimEvidenceCommand
	calls   int
}

func (fake *automaticClaimEvidenceSchedulerFake) ScheduleAutomaticClaimEvidence(_ context.Context,
	command eventapplication.ScheduleAutomaticClaimEvidenceCommand) (eventapplication.ScheduleAutomaticClaimEvidenceResult, error) {
	fake.calls++
	fake.command = command
	return eventapplication.ScheduleAutomaticClaimEvidenceResult{MicroEventID: command.MicroEventID,
		DocumentVersionID: command.DocumentVersionID, JobID: 1, Created: true}, nil
}

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
	var automaticEvidenceService *eventapplication.AutomaticClaimEvidenceService
	var automaticEvidenceScheduler *eventjobs.AutomaticClaimEvidenceScheduler
	var automaticEvidenceHandler *eventjobs.AutomaticClaimEvidenceHandler
	var productEventRefreshScheduler *eventjobs.ProductEventRefreshScheduler
	var productEventRefreshHandler *eventjobs.ProductEventRefreshHandler
	var knowledgeRecovery *knowledgeapplication.VaultRecoveryService
	var knowledgeRecoveryHandler *knowledgejobs.Handler
	var handlers map[string]queue.Handler
	app, err := NewAppWithReadiness(
		cfg,
		zap.NewNop(),
		httptransport.ReadinessFunc(func(context.Context) error { return nil }),
		fx.Populate(&handler, &recallProjections, &publishedMatches, &publishedMatchEvaluations, &matchReviews,
			&publishedMatchHandler, &acceptedMatchProjectionHandler, &automaticEvidenceService, &automaticEvidenceScheduler,
			&automaticEvidenceHandler, &productEventRefreshScheduler, &productEventRefreshHandler,
			&knowledgeRecovery, &knowledgeRecoveryHandler, &handlers),
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
		publishedMatchHandler == nil || acceptedMatchProjectionHandler == nil || automaticEvidenceService == nil ||
		automaticEvidenceScheduler == nil || automaticEvidenceHandler == nil || productEventRefreshScheduler == nil ||
		productEventRefreshHandler == nil || handlers[queue.KindGenerateSourceDocument] == nil ||
		handlers[queue.KindEvaluatePublishedDocumentMatches] == nil || handlers[queue.KindProjectAcceptedDocumentMatch] == nil ||
		knowledgeRecovery == nil || knowledgeRecoveryHandler == nil || handlers[queue.KindProjectKnowledge] == nil {
		t.Fatalf("source document/match/evidence services/registration = %#v/%#v/%#v/%#v/%#v/%#v/%#v/%#v/%#v/%#v/%#v/%#v/%#v",
			handler, recallProjections, publishedMatches, publishedMatchEvaluations, matchReviews, publishedMatchHandler,
			acceptedMatchProjectionHandler, automaticEvidenceService, automaticEvidenceScheduler, automaticEvidenceHandler,
			handlers[queue.KindGenerateSourceDocument], handlers[queue.KindEvaluatePublishedDocumentMatches],
			handlers[queue.KindProjectAcceptedDocumentMatch])
	}
	if handlers[queue.KindExtractAutomaticClaimEvidence] == nil {
		t.Fatal("automatic claim evidence handler is not registered")
	}
	if handlers[queue.KindRefreshProductEvent] == nil {
		t.Fatal("product event refresh handler is not registered")
	}
	if handlers[queue.KindProjectUserNotification] == nil {
		t.Fatal("user notification projection handler is not registered")
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
