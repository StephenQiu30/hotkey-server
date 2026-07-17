//go:build integration

package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	eventdomain "github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/internal/modules/operations/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

func TestRecomputeEventMetricsIsIdempotentAndProjectsVersionedCapabilities(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	windowEnd := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	sourceID, eventID := seedMetricRecomputeFixture(t, runtime, windowEnd)
	profileRepository := sourcepostgres.NewMetricCapabilityRepository(runtime)
	v1 := metricCapabilityProfileForRecompute("v1")
	if err := profileRepository.CreateDraft(ctx, &v1); err != nil {
		t.Fatalf("CreateDraft(v1): %v", err)
	}
	if err := profileRepository.Publish(ctx, &v1); err != nil {
		t.Fatalf("Publish(v1): %v", err)
	}
	capabilities, err := sourceapplication.NewMetricCapabilityService(sourceapplication.MetricCapabilityDependencies{Runtime: runtime, Profiles: profileRepository, SourceContexts: sourcepostgres.NewRepository(runtime), Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewMetricCapabilityService: %v", err)
	}
	service, err := eventapplication.NewHeatService(eventapplication.HeatServiceDependencies{Snapshots: NewRepository(runtime), Capabilities: capabilities})
	if err != nil {
		t.Fatalf("NewHeatService: %v", err)
	}
	command := eventapplication.MetricRecomputeCommand{EventID: eventID, WindowEnd: windowEnd, HeatVersion: eventdomain.HeatAlgorithmVersionV1}
	first, err := service.RecomputeEventMetrics(ctx, command)
	if err != nil || len(first) != 3 {
		t.Fatalf("first RecomputeEventMetrics() = %#v/%v", first, err)
	}
	assertMetricProjection(t, runtime, eventID, 3, first[2], 80)
	assertMetricQueryProjection(t, NewRepository(runtime), ctx, eventID, first[2])
	second, err := service.RecomputeEventMetrics(ctx, command)
	if err != nil || len(second) != 3 {
		t.Fatalf("second RecomputeEventMetrics() = %#v/%v", second, err)
	}
	assertMetricSnapshotCount(t, runtime, eventID, 3)

	v2 := metricCapabilityProfileForRecompute("v2")
	if err := profileRepository.CreateDraft(ctx, &v2); err != nil {
		t.Fatalf("CreateDraft(v2): %v", err)
	}
	if err := profileRepository.Archive(ctx, &v1); err != nil {
		t.Fatalf("Archive(v1): %v", err)
	}
	if err := profileRepository.Publish(ctx, &v2); err != nil {
		t.Fatalf("Publish(v2): %v", err)
	}
	if _, err := service.RecomputeEventMetrics(ctx, command); err != nil {
		t.Fatalf("RecomputeEventMetrics(profile switch): %v", err)
	}
	assertMetricSnapshotCount(t, runtime, eventID, 6)

	if _, err := runtime.SQL.Exec(`UPDATE contents SET content_status = 'deleted', deleted_at = now() WHERE id = (SELECT content_id FROM event_contents WHERE event_id = $1 LIMIT 1)`, eventID); err != nil {
		t.Fatalf("delete evidence: %v", err)
	}
	third, err := service.RecomputeEventMetrics(ctx, command)
	if err != nil || len(third) != 3 || third[2].ContentCount != 0 || !containsMetricReason(third[2].ReasonCodes, "no_active_evidence") {
		t.Fatalf("RecomputeEventMetrics(deleted evidence) = %#v/%v", third, err)
	}
	assertMetricSnapshotCount(t, runtime, eventID, 9)
	if sourceID <= 0 {
		t.Fatal("source fixture was not created")
	}
}

func TestEvidenceRecomputeSynchronouslyAppendsMetricSnapshots(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	_, eventID := seedMetricRecomputeFixture(t, runtime, time.Now().UTC().Truncate(time.Hour))
	profiles := sourcepostgres.NewMetricCapabilityRepository(runtime)
	profile := metricCapabilityProfileForRecompute("v1")
	if err := profiles.CreateDraft(ctx, &profile); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if err := profiles.Publish(ctx, &profile); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	capabilities, err := sourceapplication.NewMetricCapabilityService(sourceapplication.MetricCapabilityDependencies{Runtime: runtime, Profiles: profiles, SourceContexts: sourcepostgres.NewRepository(runtime), Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewMetricCapabilityService: %v", err)
	}
	heat, err := eventapplication.NewHeatService(eventapplication.HeatServiceDependencies{Snapshots: NewRepository(runtime), Capabilities: capabilities})
	if err != nil {
		t.Fatalf("NewHeatService: %v", err)
	}
	if _, err := eventapplication.NewEvidenceService(NewRepository(runtime), heat).Recompute(ctx, eventapplication.EvidenceRecomputeCommand{EventID: eventID, ReasonCode: "metric_refresh"}); err != nil {
		t.Fatalf("EvidenceService.Recompute: %v", err)
	}
	assertMetricSnapshotCount(t, runtime, eventID, 3)
}

func TestGovernanceMergeSynchronouslyRecomputesBothAffectedEvents(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	windowEnd := time.Now().UTC().Truncate(time.Hour)
	_, sourceEventID := seedMetricRecomputeFixture(t, runtime, windowEnd)
	var targetEventID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at) VALUES ('evt-metric-target-' || md5(random()::text), '目标事件', '', 'detected', $1, $2) RETURNING id`, windowEnd.Add(-time.Hour), windowEnd).Scan(&targetEventID); err != nil {
		t.Fatalf("insert merge target: %v", err)
	}
	profiles := sourcepostgres.NewMetricCapabilityRepository(runtime)
	profile := metricCapabilityProfileForRecompute("v1")
	if err := profiles.CreateDraft(ctx, &profile); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if err := profiles.Publish(ctx, &profile); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	capabilities, err := sourceapplication.NewMetricCapabilityService(sourceapplication.MetricCapabilityDependencies{Runtime: runtime, Profiles: profiles, SourceContexts: sourcepostgres.NewRepository(runtime), Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewMetricCapabilityService: %v", err)
	}
	heat, err := eventapplication.NewHeatService(eventapplication.HeatServiceDependencies{Snapshots: NewRepository(runtime), Capabilities: capabilities})
	if err != nil {
		t.Fatalf("NewHeatService: %v", err)
	}
	if _, err := eventapplication.NewGovernanceService(NewRepository(runtime), heat).Merge(ctx, eventapplication.MergeCommand{SourceEventID: sourceEventID, TargetEventID: targetEventID, SourceExpectedVersion: 1, TargetExpectedVersion: 1, ReasonCode: "metric_merge"}); err != nil {
		t.Fatalf("GovernanceService.Merge: %v", err)
	}
	assertMetricSnapshotCount(t, runtime, sourceEventID, 3)
	assertMetricSnapshotCount(t, runtime, targetEventID, 3)
}

func seedMetricRecomputeFixture(t *testing.T, runtime *database.Runtime, windowEnd time.Time) (int64, int64) {
	t.Helper()
	var sourceID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint) VALUES ('rss', 'metric-source-' || md5(random()::text), 'https://metric.example.test') RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	contentIDs := make([]int64, 0, 5)
	for index, views := range []int64{10, 20, 30, 40, 500} {
		var contentID int64
		if err := runtime.SQL.QueryRow(`INSERT INTO contents (source_connection_id, external_id, content_type, title, canonical_url, published_at, fetched_at, dedupe_key) VALUES ($1,$2::varchar,'article',$2::text,'https://metric.example.test/' || $2::text,$3,$3,repeat('a',64)) RETURNING id`, sourceID, fmt.Sprintf("metric-%d", index), windowEnd.Add(-time.Hour)).Scan(&contentID); err != nil {
			t.Fatalf("insert content %d: %v", index, err)
		}
		contentIDs = append(contentIDs, contentID)
		for capturedAt, count := range map[time.Time]int64{windowEnd.Add(-25 * time.Hour): 0, windowEnd.Add(-30 * time.Minute): views} {
			if _, err := runtime.SQL.Exec(`INSERT INTO content_metric_snapshots (content_id, captured_at, view_count, like_count) VALUES ($1,$2,$3,$4)`, contentID, capturedAt, count, count/10); err != nil {
				t.Fatalf("insert metric snapshot %d: %v", index, err)
			}
		}
	}
	var eventID, monitorID, configID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at) VALUES ('evt-metric-' || md5(random()::text), '指标事件', '', 'active', $1, $2) RETURNING id`, windowEnd.Add(-48*time.Hour), windowEnd.Add(-30*time.Minute)).Scan(&eventID); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO event_contents (event_id, content_id, membership_score, evidence_role, origin) VALUES ($1,$2,90,'primary','rule')`, eventID, contentIDs[len(contentIDs)-1]); err != nil {
		t.Fatalf("insert event evidence: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name, status) VALUES ('metric monitor', 'active') RETURNING id`).Scan(&monitorID); err != nil {
		t.Fatalf("insert monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision) VALUES ($1,1) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("insert monitor config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state = 'published', config_hash = repeat('a', 64), published_at = $1 WHERE id = $2`, windowEnd, configID); err != nil {
		t.Fatalf("publish monitor config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET published_config_version_id = $1 WHERE id = $2`, configID, monitorID); err != nil {
		t.Fatalf("publish monitor config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO monitor_events (monitor_id, event_id, relevance_score, final_score, first_matched_at, last_matched_at) VALUES ($1,$2,80,80,$3,$3)`, monitorID, eventID, windowEnd); err != nil {
		t.Fatalf("insert monitor event: %v", err)
	}
	return sourceID, eventID
}

func metricCapabilityProfileForRecompute(version string) sourcedomain.MetricCapabilityProfile {
	return sourcedomain.MetricCapabilityProfile{SourceType: sourcedomain.SourceTypeRSS, ProfileVersion: version, SupportsViews: true, SupportsLikes: true, IndependenceStrategy: sourcedomain.IndependenceBySourceConnection, NormalizationWindowHours: 24, CredibilityWeight: 0.8, MaxSingleItemContribution: 50}
}

func assertMetricProjection(t *testing.T, runtime *database.Runtime, eventID, snapshots int64, expected eventdomain.HeatResult, previousMonitorScore float64) {
	t.Helper()
	assertMetricSnapshotCount(t, runtime, eventID, snapshots)
	var heat, trend float64
	var version, capabilityHash string
	if err := runtime.SQL.QueryRow(`SELECT heat_score, trend_score, heat_version, metric_capability_profile_set_hash FROM events WHERE id = $1`, eventID).Scan(&heat, &trend, &version, &capabilityHash); err != nil {
		t.Fatalf("read event projection: %v", err)
	}
	if heat != expected.HeatScore || trend != expected.TrendScore || version != expected.HeatVersion || capabilityHash != expected.CapabilityProfileSetHash {
		t.Fatalf("event projection = %v/%v/%q/%q, want %#v", heat, trend, version, capabilityHash, expected)
	}
	var monitorScore float64
	if err := runtime.SQL.QueryRow(`SELECT final_score FROM monitor_events WHERE event_id = $1`, eventID).Scan(&monitorScore); err != nil {
		t.Fatalf("read monitor projection: %v", err)
	}
	if monitorScore == previousMonitorScore {
		t.Fatalf("monitor score = %v, want metric projection to recompute it", monitorScore)
	}
}

func assertMetricQueryProjection(t *testing.T, repository *Repository, ctx context.Context, eventID int64, expected eventdomain.HeatResult) {
	t.Helper()
	event, err := repository.Get(ctx, eventID)
	if err != nil {
		t.Fatalf("get event metric projection: %v", err)
	}
	if event.HeatScore != expected.HeatScore || event.TrendScore != expected.TrendScore || event.TrendStatus != expected.TrendStatus || event.HeatWindowHours != expected.WindowHours || event.HeatVersion != expected.HeatVersion || event.MetricCapabilityProfileSetHash != expected.CapabilityProfileSetHash || event.HeatCalculatedAt == nil {
		t.Fatalf("event query metric projection = %#v, want %#v", event, expected)
	}
	latest, err := repository.LatestHeatSnapshot(ctx, eventID)
	if err != nil {
		t.Fatalf("latest metric snapshot: %v", err)
	}
	if latest.WindowHours != expected.WindowHours || latest.TrendStatus != expected.TrendStatus || latest.CapabilityProfileSetHash != expected.CapabilityProfileSetHash || strings.Join(latest.ReasonCodes, ",") != strings.Join(expected.ReasonCodes, ",") {
		t.Fatalf("latest metric response = %#v, want %#v", latest, expected)
	}
}

func assertMetricSnapshotCount(t *testing.T, runtime *database.Runtime, eventID, want int64) {
	t.Helper()
	var count int64
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_metric_snapshots WHERE event_id = $1`, eventID).Scan(&count); err != nil {
		t.Fatalf("count metric snapshots: %v", err)
	}
	if count != want {
		t.Fatalf("metric snapshot count = %d, want %d", count, want)
	}
}

func containsMetricReason(reasons []string, want string) bool {
	return strings.Contains(","+strings.Join(reasons, ",")+",", ","+want+",")
}
