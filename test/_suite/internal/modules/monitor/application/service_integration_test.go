package application_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	monitorapplication "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/application"
	monitordomain "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/infrastructure/postgres"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/internal/modules/operations/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

type monitorFailingAudit struct{ err error }

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
