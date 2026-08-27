package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitordomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/domain"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

type monitorFailingAudit struct{ err error }

type monitorIntentBackfillScheduler struct{}

func (monitorIntentBackfillScheduler) SchedulePublishedIntentBackfill(_ context.Context, command monitorapplication.SchedulePublishedIntentBackfillCommand) (monitorapplication.SchedulePublishedIntentBackfillResult, error) {
	return monitorapplication.SchedulePublishedIntentBackfillResult{
		MonitorID: command.MonitorID, MonitorVersionID: command.MonitorVersionID,
		CompiledProfileID: command.CompiledProfileID, JobID: 901, Created: true,
	}, nil
}

func (audit monitorFailingAudit) Write(context.Context, operationsdomain.AuditEntry) error {
	return audit.err
}

func TestMonitorServicePublishesImmutableConfigurationAndCoordinatesSourceLifecycle(t *testing.T) {
	runtime := monitorRuntime(t)
	defer func() { _ = runtime.Close() }()
	admin := monitorAdmin(t, runtime)
	ctx := context.Background()
	usage := monitorpostgres.NewSourceUsageReader(runtime)
	sources, err := sourceapplication.NewService(sourceapplication.Dependencies{Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), MonitorUsage: usage, PublishedReferences: monitorpostgres.NewPublishedReferenceReader(runtime), Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewSourceService(): %v", err)
	}
	monitors, err := monitorapplication.NewService(monitorapplication.Dependencies{Runtime: runtime, Monitors: monitorpostgres.NewRepository(runtime), Sources: sources, Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewMonitorService(): %v", err)
	}
	connection, err := sources.Create(ctx, sourceapplication.CreateInput{Subject: admin, Connection: monitorSourceConnection("monitor-service-source")})
	if err != nil {
		t.Fatalf("Create source: %v", err)
	}

	created, draft, err := monitors.Create(ctx, monitorapplication.CreateInput{Subject: monitorEditor(admin.UserID), Draft: monitorDraft(connection.ID)})
	if err != nil {
		t.Fatalf("Create monitor: %v", err)
	}
	if created.Status != monitordomain.MonitorStatusDraft || created.Version != 1 || created.DraftConfigVersionID == nil || draft.Version != 1 {
		t.Fatalf("created monitor/draft = %#v %#v", created, draft)
	}
	if _, _, err := monitors.Create(ctx, monitorapplication.CreateInput{Subject: identitydomain.Subject{UserID: admin.UserID, Role: identitydomain.RoleViewer}, Draft: monitorDraft(connection.ID)}); appCode(err) != sharederrors.CodeForbidden {
		t.Fatalf("viewer Create code=%d", appCode(err))
	}

	publishedMonitor, publishedConfig, err := monitors.Publish(ctx, monitorapplication.PublishInput{Subject: admin, MonitorID: created.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: created.Version, DraftVersion: int64Value(draft.Version)}})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if publishedMonitor.Status != monitordomain.MonitorStatusActive || publishedMonitor.DraftConfigVersionID != nil || publishedConfig.State != monitordomain.ConfigVersionPublished || publishedConfig.ConfigHash == "" || publishedConfig.PublishedAt == nil {
		t.Fatalf("published facts = %#v %#v", publishedMonitor, publishedConfig)
	}
	var checkpointCount int
	if err := runtime.SQL.QueryRow(`
SELECT count(*)
FROM source_checkpoints AS checkpoint
JOIN monitor_sources AS source ON source.id = checkpoint.monitor_source_id
WHERE source.config_version_id = $1`, publishedConfig.ID).Scan(&checkpointCount); err != nil {
		t.Fatalf("read published checkpoints: %v", err)
	}
	if checkpointCount != 1 {
		t.Fatalf("published checkpoint count = %d, want 1", checkpointCount)
	}
	var checkpointQueryHash string
	var checkpointNextPollAt time.Time
	if err := runtime.SQL.QueryRow(`
SELECT checkpoint.query_hash, checkpoint.next_poll_at
FROM source_checkpoints AS checkpoint
JOIN monitor_sources AS source ON source.id = checkpoint.monitor_source_id
WHERE source.config_version_id = $1`, publishedConfig.ID).Scan(&checkpointQueryHash, &checkpointNextPollAt); err != nil {
		t.Fatalf("read published checkpoint facts: %v", err)
	}
	if checkpointQueryHash == "" || !checkpointNextPollAt.Equal(publishedConfig.PublishedAt.UTC()) {
		t.Fatalf("published checkpoints = count %d, hash %q, next poll %s; published at %s", checkpointCount, checkpointQueryHash, checkpointNextPollAt, publishedConfig.PublishedAt.UTC())
	}
	if _, err := sources.Disable(ctx, sourceapplication.LifecycleInput{Subject: admin, ID: connection.ID, ExpectedVersion: connection.Version}); appCode(err) != sharederrors.CodeSourceConnectionRequired {
		t.Fatalf("active sole-source disable code=%d", appCode(err))
	}

	paused, err := monitors.Pause(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: created.ID, ExpectedMonitorVersion: publishedMonitor.Version})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	idempotentPause, err := monitors.Pause(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: created.ID, ExpectedMonitorVersion: paused.Version})
	if err != nil || idempotentPause.Version != paused.Version || idempotentPause.Status != monitordomain.MonitorStatusPaused {
		t.Fatalf("idempotent Pause = %#v/%v", idempotentPause, err)
	}
	if _, err := monitors.Pause(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: created.ID, ExpectedMonitorVersion: publishedMonitor.Version}); appCode(err) != sharederrors.CodeMonitorVersionConflict {
		t.Fatalf("stale Pause code=%d", appCode(err))
	}
	dueWhilePaused, err := monitorpostgres.NewPublishedCollectionTargetReader(runtime).ListDue(ctx, time.Now().UTC().Add(time.Minute))
	if err != nil || len(dueWhilePaused) != 0 {
		t.Fatalf("paused due targets = %#v/%v, want none", dueWhilePaused, err)
	}
	disabled, err := sources.Disable(ctx, sourceapplication.LifecycleInput{Subject: admin, ID: connection.ID, ExpectedVersion: connection.Version})
	if err != nil {
		t.Fatalf("paused historical source Disable: %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("disabled source = %#v", disabled)
	}
	if _, err := monitors.Resume(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: created.ID, ExpectedMonitorVersion: paused.Version}); appCode(err) != sharederrors.CodeSourceConnectionRequired {
		t.Fatalf("zero-source resume code=%d", appCode(err))
	}
	_, err = sources.Enable(ctx, sourceapplication.LifecycleInput{Subject: admin, ID: connection.ID, ExpectedVersion: disabled.Version})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	resumed, err := monitors.Resume(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: created.ID, ExpectedMonitorVersion: paused.Version})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.Status != monitordomain.MonitorStatusActive {
		t.Fatalf("resumed status=%s", resumed.Status)
	}
	idempotentResume, err := monitors.Resume(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: created.ID, ExpectedMonitorVersion: resumed.Version})
	if err != nil || idempotentResume.Version != resumed.Version || idempotentResume.Status != monitordomain.MonitorStatusActive {
		t.Fatalf("idempotent Resume = %#v/%v", idempotentResume, err)
	}
	if _, err := monitors.Resume(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: created.ID, ExpectedMonitorVersion: paused.Version}); appCode(err) != sharederrors.CodeMonitorVersionConflict {
		t.Fatalf("stale Resume code=%d", appCode(err))
	}
	dueAfterResume, err := monitorpostgres.NewPublishedCollectionTargetReader(runtime).ListDue(ctx, time.Now().UTC().Add(time.Minute))
	if err != nil || len(dueAfterResume) != 0 {
		t.Fatalf("legacy publication without ready compiled profile became due after resume: %#v/%v", dueAfterResume, err)
	}

	firstDraftMonitor, secondDraft, err := monitors.ReplaceDraft(ctx, monitorapplication.ReplaceDraftInput{Subject: monitorEditor(admin.UserID), MonitorID: created.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: resumed.Version, DraftVersion: nil}, Draft: monitorDraft(connection.ID)})
	if err != nil {
		t.Fatalf("first replacement draft: %v", err)
	}
	if secondDraft.Revision != publishedConfig.Revision+1 || firstDraftMonitor.DraftConfigVersionID == nil {
		t.Fatalf("first draft facts = %#v %#v", firstDraftMonitor, secondDraft)
	}
	if _, _, err := monitors.ReplaceDraft(ctx, monitorapplication.ReplaceDraftInput{Subject: monitorEditor(admin.UserID), MonitorID: created.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: resumed.Version, DraftVersion: nil}, Draft: monitorDraft(connection.ID)}); appCode(err) != sharederrors.CodeMonitorVersionConflict {
		t.Fatalf("stale first draft code=%d", appCode(err))
	}

	before := tableCounts(t, runtime)
	preview, err := monitors.Preview(ctx, monitorEditor(admin.UserID), created.ID)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !preview.Eligible || len(preview.Sources) != 1 || preview.Sources[0].EstimatedRequests != 1 || preview.Sources[0].QuerySignature == "" {
		t.Fatalf("preview=%#v", preview)
	}
	after := tableCounts(t, runtime)
	if before != after {
		t.Fatalf("preview wrote persistent facts: before=%v after=%v", before, after)
	}

	candidate, aiRule, err := monitors.AddAICandidate(ctx, monitorapplication.AICandidateInput{Subject: admin, MonitorID: created.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: firstDraftMonitor.Version, DraftVersion: int64Value(secondDraft.Version)}, Rule: monitordomain.MonitorRule{RuleType: monitordomain.RuleTypeKeyword, Operator: monitordomain.RuleOperatorContains, Value: "suggestion", Weight: 10, Priority: 1}})
	if err != nil {
		t.Fatalf("AddAICandidate: %v", err)
	}
	previewWithPending, err := monitors.Preview(ctx, monitorEditor(admin.UserID), created.ID)
	if err != nil {
		t.Fatalf("Preview pending AI: %v", err)
	}
	if previewWithPending.Sources[0].QuerySignature != preview.Sources[0].QuerySignature {
		t.Fatalf("pending AI changed query signature: before=%s after=%s", preview.Sources[0].QuerySignature, previewWithPending.Sources[0].QuerySignature)
	}
	approved, err := monitors.ApproveAICandidate(ctx, monitorapplication.ApprovalInput{Subject: admin, MonitorID: created.ID, RuleID: aiRule.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: firstDraftMonitor.Version + 1, DraftVersion: int64Value(candidate.Version)}, Approval: monitordomain.RuleApprovalApproved})
	if err != nil {
		t.Fatalf("ApproveAICandidate: %v", err)
	}
	previewApproved, err := monitors.Preview(ctx, monitorEditor(admin.UserID), created.ID)
	if err != nil {
		t.Fatalf("Preview approved AI: %v", err)
	}
	if previewApproved.Sources[0].QuerySignature == preview.Sources[0].QuerySignature {
		t.Fatalf("approved AI did not change query signature")
	}
	if _, _, err := monitors.Publish(ctx, monitorapplication.PublishInput{Subject: monitorEditor(admin.UserID), MonitorID: created.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: firstDraftMonitor.Version + 2, DraftVersion: int64Value(approved.Version)}}); appCode(err) != sharederrors.CodeForbidden {
		t.Fatalf("editor publish code=%d", appCode(err))
	}
	if _, _, err := monitors.Publish(ctx, monitorapplication.PublishInput{Subject: admin, MonitorID: created.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: firstDraftMonitor.Version + 2, DraftVersion: int64Value(approved.Version)}}); err != nil {
		t.Fatalf("second Publish: %v", err)
	}
	active, err := monitors.ActivePublished(ctx, identitydomain.Subject{UserID: admin.UserID, Role: identitydomain.RoleViewer})
	if err != nil || len(active) != 1 || active[0].Config.State != monitordomain.ConfigVersionPublished {
		t.Fatalf("ActivePublished() = %#v, %v", active, err)
	}
	archived, err := monitors.Archive(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: created.ID, ExpectedMonitorVersion: active[0].Monitor.Version})
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if _, err := monitors.Pause(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: created.ID, ExpectedMonitorVersion: archived.Version}); appCode(err) != sharederrors.CodeInvalidMonitorState {
		t.Fatalf("Pause archived monitor code=%d", appCode(err))
	}
	if _, _, err := monitors.Create(ctx, monitorapplication.CreateInput{Subject: monitorEditor(admin.UserID), Draft: monitorDraft(connection.ID)}); err != nil {
		t.Fatalf("Create same-name monitor after archive: %v", err)
	}
	if _, err := monitors.Restore(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: created.ID, ExpectedMonitorVersion: archived.Version}); appCode(err) != sharederrors.CodeMonitorNameConflict {
		t.Fatalf("Restore name conflict code=%d", appCode(err))
	}
	deleted, err := monitors.Delete(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: created.ID, ExpectedMonitorVersion: archived.Version})
	if err != nil || deleted.DeletedAt == nil || deleted.Version != archived.Version+1 {
		t.Fatalf("Delete archived monitor = %#v/%v", deleted, err)
	}
	page, err := monitors.List(ctx, monitorapplication.ListInput{Subject: admin})
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	for _, item := range page.Items {
		if item.Monitor.ID == created.ID {
			t.Fatalf("deleted monitor %d remained in list", created.ID)
		}
	}

	_, err = runtime.SQL.Exec(`UPDATE monitor_rules SET value = 'mutated' WHERE config_version_id = $1`, publishedConfig.ID)
	if err == nil {
		t.Fatal("mutating published child succeeded")
	}
}

func TestMonitorServiceFirstDraftAndPublishConcurrencyAndAuditRollback(t *testing.T) {
	runtime := monitorRuntime(t)
	defer func() { _ = runtime.Close() }()
	admin := monitorAdmin(t, runtime)
	ctx := context.Background()
	usage := monitorpostgres.NewSourceUsageReader(runtime)
	sources, err := sourceapplication.NewService(sourceapplication.Dependencies{Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), MonitorUsage: usage, PublishedReferences: monitorpostgres.NewPublishedReferenceReader(runtime), Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewSourceService(): %v", err)
	}
	connection, err := sources.Create(ctx, sourceapplication.CreateInput{Subject: admin, Connection: monitorSourceConnection("monitor-concurrency-source")})
	if err != nil {
		t.Fatalf("Create source: %v", err)
	}
	monitors, err := monitorapplication.NewService(monitorapplication.Dependencies{Runtime: runtime, Monitors: monitorpostgres.NewRepository(runtime), Sources: sources, Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewMonitorService(): %v", err)
	}
	monitor, draft, err := monitors.Create(ctx, monitorapplication.CreateInput{Subject: monitorEditor(admin.UserID), Draft: monitorDraft(connection.ID)})
	if err != nil {
		t.Fatalf("Create monitor: %v", err)
	}
	published, _, err := monitors.Publish(ctx, monitorapplication.PublishInput{Subject: admin, MonitorID: monitor.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: monitor.Version, DraftVersion: int64Value(draft.Version)}})
	if err != nil {
		t.Fatalf("Publish initial: %v", err)
	}

	type draftResult struct {
		monitor *monitordomain.Monitor
		config  *monitordomain.MonitorConfigVersion
		err     error
	}
	draftResults := make(chan draftResult, 2)
	var drafts sync.WaitGroup
	for range 2 {
		drafts.Add(1)
		go func() {
			defer drafts.Done()
			changed, replacement, err := monitors.ReplaceDraft(context.Background(), monitorapplication.ReplaceDraftInput{Subject: monitorEditor(admin.UserID), MonitorID: monitor.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: published.Version, DraftVersion: nil}, Draft: monitorDraft(connection.ID)})
			draftResults <- draftResult{changed, replacement, err}
		}()
	}
	drafts.Wait()
	close(draftResults)
	var winner draftResult
	successes, conflicts := 0, 0
	for result := range draftResults {
		if result.err == nil {
			successes++
			winner = result
		} else if appCode(result.err) == sharederrors.CodeMonitorVersionConflict {
			conflicts++
		} else {
			t.Fatalf("first draft concurrent error=%v", result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("first draft concurrency successes=%d conflicts=%d", successes, conflicts)
	}

	type publishResult struct{ err error }
	publishResults := make(chan publishResult, 2)
	var publishes sync.WaitGroup
	for range 2 {
		publishes.Add(1)
		go func() {
			defer publishes.Done()
			_, _, err := monitors.Publish(context.Background(), monitorapplication.PublishInput{Subject: admin, MonitorID: monitor.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: winner.monitor.Version, DraftVersion: int64Value(winner.config.Version)}})
			publishResults <- publishResult{err}
		}()
	}
	publishes.Wait()
	close(publishResults)
	successes, conflicts = 0, 0
	for result := range publishResults {
		if result.err == nil {
			successes++
		} else if appCode(result.err) == sharederrors.CodeMonitorVersionConflict {
			conflicts++
		} else {
			t.Fatalf("publish concurrent error=%v", result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("publish concurrency successes=%d conflicts=%d", successes, conflicts)
	}

	failing, err := monitorapplication.NewService(monitorapplication.Dependencies{Runtime: runtime, Monitors: monitorpostgres.NewRepository(runtime), Sources: sources, Audit: monitorFailingAudit{err: errors.New("audit failure")}})
	if err != nil {
		t.Fatalf("New failing MonitorService: %v", err)
	}
	if _, _, err := failing.Create(ctx, monitorapplication.CreateInput{Subject: monitorEditor(admin.UserID), Draft: monitorapplication.DraftInput{Name: "audit rollback monitor", Config: monitorDraft(connection.ID).Config, Rules: monitorDraft(connection.ID).Rules, Sources: monitorDraft(connection.ID).Sources}}); err == nil {
		t.Fatal("Create with failing audit succeeded")
	}
	var persisted int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitors WHERE name = 'audit rollback monitor'`).Scan(&persisted); err != nil {
		t.Fatalf("count rollback monitor: %v", err)
	}
	if persisted != 0 {
		t.Fatalf("audit failure persisted monitor count=%d", persisted)
	}
}

func TestMonitorServiceDisableAndResumeSerializeThroughConfigurationLock(t *testing.T) {
	runtime := monitorRuntime(t)
	defer func() { _ = runtime.Close() }()
	admin := monitorAdmin(t, runtime)
	ctx := context.Background()
	usage := monitorpostgres.NewSourceUsageReader(runtime)
	sources, err := sourceapplication.NewService(sourceapplication.Dependencies{Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), MonitorUsage: usage, PublishedReferences: monitorpostgres.NewPublishedReferenceReader(runtime), Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewSourceService(): %v", err)
	}
	connection, err := sources.Create(ctx, sourceapplication.CreateInput{Subject: admin, Connection: monitorSourceConnection("monitor-interleaving-source")})
	if err != nil {
		t.Fatalf("Create source: %v", err)
	}
	monitors, err := monitorapplication.NewService(monitorapplication.Dependencies{Runtime: runtime, Monitors: monitorpostgres.NewRepository(runtime), Sources: sources, Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewMonitorService(): %v", err)
	}
	monitor, draft, err := monitors.Create(ctx, monitorapplication.CreateInput{Subject: monitorEditor(admin.UserID), Draft: monitorDraft(connection.ID)})
	if err != nil {
		t.Fatalf("Create monitor: %v", err)
	}
	published, _, err := monitors.Publish(ctx, monitorapplication.PublishInput{Subject: admin, MonitorID: monitor.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: monitor.Version, DraftVersion: int64Value(draft.Version)}})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	paused, err := monitors.Pause(ctx, monitorapplication.LifecycleInput{Subject: admin, MonitorID: monitor.ID, ExpectedMonitorVersion: published.Version})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	errorsOut := make(chan error, 2)
	go func() {
		_, err := sources.Disable(context.Background(), sourceapplication.LifecycleInput{Subject: admin, ID: connection.ID, ExpectedVersion: connection.Version})
		errorsOut <- err
	}()
	go func() {
		_, err := monitors.Resume(context.Background(), monitorapplication.LifecycleInput{Subject: admin, MonitorID: monitor.ID, ExpectedMonitorVersion: paused.Version})
		errorsOut <- err
	}()
	first, second := <-errorsOut, <-errorsOut
	successes, required := 0, 0
	for _, err := range []error{first, second} {
		if err == nil {
			successes++
		} else if appCode(err) == sharederrors.CodeSourceConnectionRequired {
			required++
		} else {
			t.Fatalf("interleaving error=%v", err)
		}
	}
	if successes != 1 || required != 1 {
		t.Fatalf("disable/resume serial outcome successes=%d required=%d", successes, required)
	}
}

func TestMonitorServicePublishAndSourceDisableSerializeThroughConfigurationLock(t *testing.T) {
	runtime := monitorRuntime(t)
	defer func() { _ = runtime.Close() }()
	admin := monitorAdmin(t, runtime)
	ctx := context.Background()
	usage := monitorpostgres.NewSourceUsageReader(runtime)
	sources, err := sourceapplication.NewService(sourceapplication.Dependencies{Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), MonitorUsage: usage, PublishedReferences: monitorpostgres.NewPublishedReferenceReader(runtime), Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewSourceService(): %v", err)
	}
	connection, err := sources.Create(ctx, sourceapplication.CreateInput{Subject: admin, Connection: monitorSourceConnection("monitor-publish-disable-source")})
	if err != nil {
		t.Fatalf("Create source: %v", err)
	}
	monitors, err := monitorapplication.NewService(monitorapplication.Dependencies{Runtime: runtime, Monitors: monitorpostgres.NewRepository(runtime), Sources: sources, Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewMonitorService(): %v", err)
	}
	monitor, draft, err := monitors.Create(ctx, monitorapplication.CreateInput{Subject: monitorEditor(admin.UserID), Draft: monitorDraft(connection.ID)})
	if err != nil {
		t.Fatalf("Create monitor: %v", err)
	}
	errorsOut := make(chan error, 2)
	go func() {
		_, _, err := monitors.Publish(context.Background(), monitorapplication.PublishInput{Subject: admin, MonitorID: monitor.ID, Expected: monitordomain.ExpectedVersions{MonitorVersion: monitor.Version, DraftVersion: int64Value(draft.Version)}})
		errorsOut <- err
	}()
	go func() {
		_, err := sources.Disable(context.Background(), sourceapplication.LifecycleInput{Subject: admin, ID: connection.ID, ExpectedVersion: connection.Version})
		errorsOut <- err
	}()
	first, second := <-errorsOut, <-errorsOut
	successes, required := 0, 0
	for _, err := range []error{first, second} {
		if err == nil {
			successes++
		} else if appCode(err) == sharederrors.CodeSourceConnectionRequired {
			required++
		} else {
			t.Fatalf("publish/disable interleaving error=%v", err)
		}
	}
	if successes != 1 || required != 1 {
		t.Fatalf("publish/disable serial outcome successes=%d required=%d", successes, required)
	}
}

func TestMonitorServicePublishesSuccessfulIntentProfileAtomicallyWithoutLegacyRules(t *testing.T) {
	runtime := monitorRuntime(t)
	defer func() { _ = runtime.Close() }()
	admin := monitorAdmin(t, runtime)
	ctx := context.Background()
	usage := monitorpostgres.NewSourceUsageReader(runtime)
	sources, err := sourceapplication.NewService(sourceapplication.Dependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), MonitorUsage: usage,
		PublishedReferences: monitorpostgres.NewPublishedReferenceReader(runtime), Audit: operationspostgres.NewAuditWriter(runtime),
	})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := sources.Create(ctx, sourceapplication.CreateInput{
		Subject: admin, Connection: monitorSourceConnection("intent-publication-source"),
	})
	if err != nil {
		t.Fatal(err)
	}
	intentRepository, err := monitorpostgres.NewIntentRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	publicationClock := monitorIntentPublicationClock{now: time.Date(2026, time.July, 18, 8, 0, 0, 0, time.UTC)}
	publication, err := monitorapplication.NewIntentPublicationService(intentRepository, monitorIntentBackfillScheduler{})
	if err != nil {
		t.Fatal(err)
	}
	draftInput := monitorDraft(connection.ID)
	draftInput.Rules = nil
	creator, err := monitorapplication.NewService(monitorapplication.Dependencies{
		Runtime: runtime, Monitors: monitorpostgres.NewRepository(runtime), Sources: sources,
		Audit: operationspostgres.NewAuditWriter(runtime),
	})
	if err != nil {
		t.Fatal(err)
	}
	monitor, draft, err := creator.Create(ctx, monitorapplication.CreateInput{Subject: monitorEditor(admin.UserID), Draft: draftInput})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	seedSuccessfulIntentPreview(t, runtime, intentRepository, monitor.ID, draft.ID, publicationClock.now)

	auditFailure := errors.New("intent publication audit rollback")
	failing, err := monitorapplication.NewService(monitorapplication.Dependencies{
		Runtime: runtime, Monitors: monitorpostgres.NewRepository(runtime), Sources: sources,
		Audit: monitorFailingAudit{err: auditFailure}, IntentPublication: publication,
	})
	if err != nil {
		t.Fatal(err)
	}
	publicationPreview, err := failing.Preview(ctx, admin, monitor.ID)
	if err != nil {
		t.Fatalf("Preview(v2 intent): %v", err)
	}
	if !publicationPreview.Eligible || len(publicationPreview.Sources) != 1 ||
		publicationPreview.Sources[0].IncludedTermCount != 3 || publicationPreview.Sources[0].ExcludedTermCount != 0 ||
		!strings.Contains(publicationPreview.Sources[0].CompiledQuery, "launch") ||
		!strings.Contains(publicationPreview.Sources[0].CompiledQuery, "HotKey") {
		t.Fatalf("v2 publication preview = %#v", publicationPreview)
	}
	if _, _, err := failing.Publish(ctx, monitorapplication.PublishInput{
		Subject: admin, MonitorID: monitor.ID,
		Expected: monitordomain.ExpectedVersions{MonitorVersion: monitor.Version, DraftVersion: int64Value(draft.Version)},
	}); !errors.Is(err, auditFailure) {
		t.Fatalf("Publish(audit failure) = %v", err)
	}
	var state string
	var publishedProfiles int
	if err := runtime.SQL.QueryRow(`SELECT state FROM monitor_config_versions WHERE id=$1`, draft.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_compiled_profiles WHERE purpose='published' AND config_version_id=$1`, draft.ID).Scan(&publishedProfiles); err != nil {
		t.Fatal(err)
	}
	if state != "draft" || publishedProfiles != 0 {
		t.Fatalf("failed publication leaked state/profile = %s/%d", state, publishedProfiles)
	}

	service, err := monitorapplication.NewService(monitorapplication.Dependencies{
		Runtime: runtime, Monitors: monitorpostgres.NewRepository(runtime), Sources: sources,
		Audit: operationspostgres.NewAuditWriter(runtime), IntentPublication: publication,
	})
	if err != nil {
		t.Fatal(err)
	}
	publishedMonitor, publishedConfig, err := service.Publish(ctx, monitorapplication.PublishInput{
		Subject: admin, MonitorID: monitor.ID,
		Expected: monitordomain.ExpectedVersions{MonitorVersion: monitor.Version, DraftVersion: int64Value(draft.Version)},
	})
	if err != nil {
		t.Fatalf("Publish(): %v", err)
	}
	if publishedMonitor.Status != monitordomain.MonitorStatusActive || publishedConfig.State != monitordomain.ConfigVersionPublished {
		t.Fatalf("published monitor/config = %#v / %#v", publishedMonitor, publishedConfig)
	}
	var profileStatus string
	if err := runtime.SQL.QueryRow(`
SELECT status FROM monitor_compiled_profiles
WHERE purpose='published' AND monitor_version_id=$1`, publishedConfig.ID).Scan(&profileStatus); err != nil {
		t.Fatal(err)
	}
	if profileStatus != "ready" {
		t.Fatalf("published compiled profile status = %q", profileStatus)
	}
	var legacyRuleCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_rules WHERE config_version_id=$1`, publishedConfig.ID).Scan(&legacyRuleCount); err != nil {
		t.Fatal(err)
	}
	if legacyRuleCount != 0 {
		t.Fatalf("legacy rule count = %d", legacyRuleCount)
	}
	targets, err := monitorpostgres.NewPublishedCollectionTargetReader(runtime).ListDue(ctx, publishedConfig.PublishedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || len(targets[0].Terms) == 0 {
		t.Fatalf("published collection targets = %#v", targets)
	}
	seen := map[string]bool{}
	for _, term := range targets[0].Terms {
		seen[term.Value] = !term.Excluded
	}
	if !seen["launch"] || !seen["Track launch disruption"] || !seen["HotKey"] {
		t.Fatalf("published compiled terms = %#v", targets[0].Terms)
	}
}

func seedSuccessfulIntentPreview(t *testing.T, runtime *database.Runtime, repository *monitorpostgres.IntentRepository, monitorID, configID int64, now time.Time) {
	t.Helper()
	var draftID, revisionID, entityID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_drafts (monitor_id,config_version_id) VALUES ($1,$2) RETURNING id`, monitorID, configID).Scan(&draftID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_draft_revisions (draft_id,monitor_id,config_version_id,resource_version,objective)
VALUES ($1,$2,$3,1,'Track launch disruption') RETURNING id`, draftID, monitorID, configID).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_intent_clauses (revision_id,draft_id,resource_version,ordinal,operator,field,value)
VALUES ($1,$2,1,0,'must','action','launch')`, revisionID, draftID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_entities (revision_id,draft_id,resource_version,ordinal,canonical_id,display_name,ambiguity_note)
VALUES ($1,$2,1,0,'product:hotkey','HotKey','product') RETURNING id`, revisionID, draftID).Scan(&entityID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_intent_entity_aliases (entity_id,draft_id,resource_version,ordinal,alias)
VALUES ($1,$2,1,0,'HotKey')`, entityID, draftID); err != nil {
		t.Fatal(err)
	}
	reservation, err := repository.ReserveAndEnqueue(context.Background(), monitorapplication.ReserveIntentRunDTO{
		IdempotencyKey: fmt.Sprintf("monitor.%d.preview", monitorID), RequestHash: strings.Repeat("8", 64), RequestedAt: now,
		Task: monitorapplication.IntentRunTaskDTO{
			Kind: "preview", MonitorID: monitorID, DraftID: draftID, DraftResourceVersion: 1,
			InputHash: strings.Repeat("a", 64), AnalysisProfile: "preview-v1", SampleLimit: 25,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := now.Add(time.Second)
	running := reservation.Run
	running.Status, running.StartedAt = "running", &startedAt
	if _, err := repository.SaveTransition(context.Background(), monitorapplication.IntentRunTransitionDTO{Expected: reservation.Run, Next: running}); err != nil {
		t.Fatal(err)
	}
	profileCommand := monitorapplication.PersistPreviewCompiledProfileDTO{
		Task: monitorapplication.IntentAnalysisTaskDTO{
			Run: monitorapplication.IntentRunReferenceDTO{
				RunID: running.ID, Kind: "preview", MonitorID: monitorID, DraftID: draftID,
				DraftResourceVersion: 1, InputHash: running.InputHash,
			}, AnalysisProfile: "preview-v1", SampleLimit: 25,
		},
		CompilerVersion:          monitorapplication.IntentCompilerVersion,
		MatchingAlgorithmVersion: "rrf-k60-v1", LexicalAlgorithmVersion: "fts-trgm-dice-v1",
		SemanticAlgorithmVersion: "halfvec-cosine-v1", StructuredAlgorithmVersion: "entity-hard-rule-v1",
		SearchNormalizationProfileVersion: monitorapplication.IntentSearchNormalizationProfileVersion,
		SemanticState:                     "unavailable", SemanticUnavailableReason: monitorapplication.IntentSemanticGenerationUnavailable,
		ProfileHash: strings.Repeat("c", 64), ReadyAt: startedAt,
		Clauses: []monitorapplication.CompiledIntentClauseDTO{
			{Operator: "must", Field: "action", Value: "launch", NormalizedValue: "launch", Origin: "intent_clause"},
			{Operator: "should", Field: "phrase", Value: "Track launch disruption", NormalizedValue: "track launch disruption", Origin: "objective_derived"},
		},
		Entities: []monitorapplication.CompiledIntentEntityDTO{{
			CanonicalID: "product:hotkey", Aliases: []string{"HotKey"}, NormalizedAliases: []string{"hotkey"},
		}},
	}
	profileReceipt, err := repository.PersistPreviewCompiledProfile(context.Background(), profileCommand)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CompletePreviewCompiledProfile(context.Background(), monitorapplication.CompletePreviewCompiledProfileDTO{
		CompiledProfileID: profileReceipt.CompiledProfileID, ConfigVersionID: configID,
		IntentRevisionID: profileReceipt.IntentRevisionID, ProfileHash: profileCommand.ProfileHash,
		SemanticState:             monitorapplication.IntentSemanticStateUnavailable,
		SemanticUnavailableReason: monitorapplication.IntentSemanticModelUnavailable, ReadyAt: profileCommand.ReadyAt,
	}); err != nil {
		t.Fatal(err)
	}
	completedAt := now.Add(2 * time.Second)
	succeeded := running
	succeeded.Status, succeeded.CompletedAt = "succeeded", &completedAt
	if _, err := repository.CompletePreview(context.Background(), monitorapplication.CompletePreviewRunMutationDTO{
		Transition:        monitorapplication.IntentRunTransitionDTO{Expected: running, Next: succeeded},
		Preview:           monitorapplication.IntentPreviewDTO{Warnings: []string{"preview_uncalibrated"}},
		ResultFingerprint: strings.Repeat("d", 64),
	}); err != nil {
		t.Fatal(err)
	}
}

type monitorIntentPublicationClock struct{ now time.Time }

func (clock monitorIntentPublicationClock) Now() time.Time { return clock.now }

func monitorRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("InitializeEmpty: %v", err)
	}
	return runtime
}
func monitorAdmin(t *testing.T, runtime *database.Runtime) identitydomain.Subject {
	t.Helper()
	var id int64
	email := fmt.Sprintf("monitor-admin-%d@example.test", time.Now().UnixNano())
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email, password_hash, display_name, role, status) VALUES ($1, 'hash', 'Monitor Admin', 'admin', 'active') RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed monitor admin: %v", err)
	}
	return identitydomain.Subject{UserID: id, SessionID: 1, Role: identitydomain.RoleAdmin}
}
func monitorEditor(id int64) identitydomain.Subject {
	return identitydomain.Subject{UserID: id, SessionID: 2, Role: identitydomain.RoleEditor}
}
func monitorSourceConnection(name string) sourcedomain.SourceConnection {
	return sourcedomain.SourceConnection{SourceType: sourcedomain.SourceTypeRSS, Name: name, Endpoint: "https://feeds.example.test/rss", AuthType: sourcedomain.AuthTypeNone, Config: sourcedomain.DefaultSourceConfig(), Enabled: true}
}
func monitorDraft(sourceID int64) monitorapplication.DraftInput {
	return monitorapplication.DraftInput{Name: "AI news", Description: "immutable configuration", Config: monitordomain.MonitorConfig{Timezone: "UTC", Languages: []string{"en"}, Regions: []string{"US"}, CollectionIntervalSeconds: 300, RelevanceThreshold: 60, EventThreshold: 0, RetentionDays: 30}, Rules: []monitordomain.MonitorRule{{RuleType: monitordomain.RuleTypeKeyword, Operator: monitordomain.RuleOperatorContains, Value: "AI", Weight: 100, Priority: 1, Enabled: true}}, Sources: []monitordomain.MonitorSource{{SourceConnectionID: sourceID, Priority: 1, Enabled: true}}}
}
func int64Value(value int64) *int64 { return &value }
func appCode(err error) int {
	var app *sharederrors.AppError
	if errors.As(err, &app) {
		return app.Code
	}
	return 0
}
func tableCounts(t *testing.T, runtime *database.Runtime) string {
	t.Helper()
	var result string
	if err := runtime.SQL.QueryRow(`SELECT concat_ws(':', (SELECT count(*) FROM monitors), (SELECT count(*) FROM monitor_config_versions), (SELECT count(*) FROM monitor_rules), (SELECT count(*) FROM monitor_sources), (SELECT count(*) FROM audit_logs))`).Scan(&result); err != nil {
		t.Fatalf("table counts: %v", err)
	}
	return result
}
