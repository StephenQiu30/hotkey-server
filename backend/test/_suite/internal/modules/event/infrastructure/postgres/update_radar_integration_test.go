//go:build integration

package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	eventdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestEventUpdateRepositoryIsAppendOnlyAndConcurrentIdempotent(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	observedAt := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	eventID := seedEventUpdateEvent(t, runtime, observedAt)
	repository := NewUpdateRepository(runtime)
	firstHeat := repositoryEventUpdateHeat(eventID, observedAt, 40, eventdomain.TrendStable, 1)
	firstCandidate := eventUpdateCandidate(t, nil, firstHeat)
	first, created, err := repository.AppendUpdate(ctx, *firstCandidate)
	if err != nil || !created || first == nil || first.SequenceNo != 1 {
		t.Fatalf("first AppendUpdate() = %#v/%t/%v", first, created, err)
	}
	reused, created, err := repository.AppendUpdate(ctx, *firstCandidate)
	if err != nil || created || reused == nil || reused.ID != first.ID || reused.SequenceNo != 1 {
		t.Fatalf("repeated AppendUpdate() = %#v/%t/%v, want first row", reused, created, err)
	}

	secondHeat := repositoryEventUpdateHeat(eventID, observedAt.Add(time.Hour), 60, eventdomain.TrendRising, 3)
	secondCandidate := eventUpdateCandidate(t, &firstHeat, secondHeat)
	start := make(chan struct{})
	type result struct {
		update  *eventdomain.EventUpdate
		created bool
		err     error
	}
	results := make(chan result, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			update, inserted, appendErr := repository.AppendUpdate(context.Background(), *secondCandidate)
			results <- result{update: update, created: inserted, err: appendErr}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	createdCount := 0
	var secondID int64
	for result := range results {
		if result.err != nil || result.update == nil || result.update.SequenceNo != 2 {
			t.Fatalf("concurrent AppendUpdate() = %#v/%t/%v", result.update, result.created, result.err)
		}
		if secondID != 0 && result.update.ID != secondID {
			t.Fatalf("concurrent IDs = %d and %d, want the same row", secondID, result.update.ID)
		}
		secondID = result.update.ID
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("concurrent created count = %d, want 1", createdCount)
	}
	assertEventUpdateSequences(t, runtime, eventID, []int64{1, 2})

	if _, err := runtime.SQL.Exec(`UPDATE event_updates SET summary = 'tampered' WHERE id = $1`, first.ID); err == nil {
		t.Fatal("event_updates UPDATE succeeded, want append-only rejection")
	}
	if _, err := runtime.SQL.Exec(`DELETE FROM event_updates WHERE id = $1`, first.ID); err == nil {
		t.Fatal("event_updates DELETE succeeded, want append-only rejection")
	}
}

func seedEventUpdateEvent(t *testing.T, runtime *database.Runtime, now time.Time) int64 {
	t.Helper()
	var eventID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO events (event_key,title_zh,summary,lifecycle_status,first_seen_at,last_seen_at) VALUES ('evt-update-' || md5(random()::text),'更新事件','','active',$1,$1) RETURNING id`, now).Scan(&eventID); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	return eventID
}

func repositoryEventUpdateHeat(eventID int64, observedAt time.Time, heat float64, trend eventdomain.TrendStatus, sources int) eventdomain.HeatResult {
	return eventdomain.HeatResult{
		EventID: eventID, HeatScore: heat, TrendScore: 20, TrendStatus: trend,
		SourceCount: sources, ContentCount: sources + 1, HeatVersion: eventdomain.HeatAlgorithmVersionV1,
		EvidenceSetHash: strings.Repeat("a", 63) + string(rune('a'+sources)), CapabilityProfileSetHash: strings.Repeat("b", 64),
		WindowHours: 24, WindowEnd: observedAt,
	}
}

func eventUpdateCandidate(t *testing.T, previous *eventdomain.HeatResult, current eventdomain.HeatResult) *eventdomain.EventUpdateCandidate {
	t.Helper()
	candidate, err := eventdomain.DetectEventUpdate(previous, current)
	if err != nil || candidate == nil {
		t.Fatalf("DetectEventUpdate() = %#v/%v", candidate, err)
	}
	return candidate
}

func assertEventUpdateSequences(t *testing.T, runtime *database.Runtime, eventID int64, want []int64) {
	t.Helper()
	rows, err := runtime.SQL.Query(`SELECT sequence_no FROM event_updates WHERE event_id = $1 ORDER BY sequence_no`, eventID)
	if err != nil {
		t.Fatalf("query event update sequences: %v", err)
	}
	defer rows.Close()
	var got []int64
	for rows.Next() {
		var sequence int64
		if err := rows.Scan(&sequence); err != nil {
			t.Fatal(err)
		}
		got = append(got, sequence)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("sequences = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("sequences = %#v, want %#v", got, want)
		}
	}
}

type radarFixture struct {
	asOf     time.Time
	monitor  int64
	eventIDs map[string]int64
}

func TestRadarRepositoryFiltersAndUsesAllFiveDeterministicOrders(t *testing.T) {
	ctx, runtime, fixture := seedRadarRepositoryFixture(t)
	defer runtime.Close()
	repository := NewRadarRepository(runtime)

	orders := map[eventdomain.RadarSort][]string{
		eventdomain.RadarSortAttention: {"a", "b", "c", "d", "e"},
		eventdomain.RadarSortMomentum:  {"b", "d", "c", "a", "e"},
		eventdomain.RadarSortBreadth:   {"c", "e", "d", "b", "a"},
		eventdomain.RadarSortLatest:    {"e", "d", "b", "a", "c"},
	}
	for sort, wantKeys := range orders {
		page, err := repository.ListRadar(ctx, eventdomain.RadarQuery{Window: eventdomain.RadarWindow7Days, Sort: sort, Limit: 100, AsOf: fixture.asOf})
		if err != nil {
			t.Fatalf("ListRadar(%s): %v", sort, err)
		}
		assertRadarOrder(t, page.Items, fixture, wantKeys)
		for _, item := range page.Items {
			if item.WatchRelevance != nil || item.WatchFinalScore != nil {
				t.Fatalf("global Radar leaked watch scores: %#v", item)
			}
		}
	}
	keywordPage, err := repository.ListRadar(ctx, eventdomain.RadarQuery{Window: eventdomain.RadarWindow7Days, Keyword: "Radar b", Sort: eventdomain.RadarSortMomentum, Limit: 100, AsOf: fixture.asOf})
	if err != nil {
		t.Fatalf("ListRadar(keyword): %v", err)
	}
	assertRadarOrder(t, keywordPage.Items, fixture, []string{"b"})

	monitorPage, err := repository.ListRadar(ctx, eventdomain.RadarQuery{Window: eventdomain.RadarWindow7Days, MonitorID: &fixture.monitor, Sort: eventdomain.RadarSortRelevance, Limit: 100, AsOf: fixture.asOf})
	if err != nil {
		t.Fatal(err)
	}
	assertRadarOrder(t, monitorPage.Items, fixture, []string{"c", "e", "d", "b"})
	if monitorPage.Items[0].WatchRelevance == nil || *monitorPage.Items[0].WatchRelevance != 91 || monitorPage.Items[0].WatchFinalScore == nil || *monitorPage.Items[0].WatchFinalScore != 99 || monitorPage.Items[0].RankingScore != 99 {
		t.Fatalf("monitor relevance projection = %#v", monitorPage.Items[0])
	}

	activePage, err := repository.ListRadar(ctx, eventdomain.RadarQuery{Window: eventdomain.RadarWindow7Days, Lifecycles: []eventdomain.LifecycleStatus{eventdomain.LifecycleActive}, Sort: eventdomain.RadarSortMomentum, Limit: 100, AsOf: fixture.asOf})
	if err != nil {
		t.Fatal(err)
	}
	assertRadarOrder(t, activePage.Items, fixture, []string{"d", "a", "e"})
	disputedPage, err := repository.ListRadar(ctx, eventdomain.RadarQuery{Window: eventdomain.RadarWindow7Days, Trends: []eventdomain.TrendStatus{eventdomain.TrendRising}, Verifications: []eventdomain.RadarConfirmation{eventdomain.RadarConfirmationDisputed}, Sort: eventdomain.RadarSortMomentum, Limit: 100, AsOf: fixture.asOf})
	if err != nil {
		t.Fatal(err)
	}
	assertRadarOrder(t, disputedPage.Items, fixture, []string{"b"})
	minHeat := 90.0
	hotPage, err := repository.ListRadar(ctx, eventdomain.RadarQuery{Window: eventdomain.RadarWindow24Hours, MinHeat: &minHeat, Sort: eventdomain.RadarSortAttention, Limit: 100, AsOf: fixture.asOf})
	if err != nil {
		t.Fatal(err)
	}
	assertRadarOrder(t, hotPage.Items, fixture, []string{"a"})

	itemA := radarItemByID(t, ordersPage(t, repository, ctx, fixture.asOf), fixture.eventIDs["a"])
	if itemA.Confirmation != eventdomain.RadarConfirmationInsufficient || itemA.ConfirmationScore != nil {
		t.Fatalf("insufficient confirmation = %q/%v, want null score", itemA.Confirmation, itemA.ConfirmationScore)
	}
	itemD := radarItemByID(t, ordersPage(t, repository, ctx, fixture.asOf), fixture.eventIDs["d"])
	if itemD.LatestUpdate == nil || itemD.LatestUpdate.SequenceNo != 2 || string(itemD.LatestUpdate.Kind) != "rising" {
		t.Fatalf("latest event update = %#v", itemD.LatestUpdate)
	}
}

func TestRadarRepositorySelectsOnlyFactsCreatedByAsOf(t *testing.T) {
	ctx, runtime, fixture := seedRadarRepositoryFixture(t)
	defer runtime.Close()
	futureCreatedAt := fixture.asOf.Add(time.Minute)
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO event_metric_snapshots (event_id,captured_at,heat_score,trend_score,source_count,content_count,heat_version,evidence_set_hash,capability_profile_set_hash,window_hours,trend_status,created_at) VALUES ($1,$2,1,100,20,20,'heat-v1',repeat('c',64),repeat('d',64),24,'rising',$3)`, fixture.eventIDs["a"], fixture.asOf, futureCreatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO event_claims (event_id,normalized_claim,claim_hash,status,confidence,first_seen_at,last_seen_at,created_at) VALUES ($1,'future disputed claim',repeat('f',64),'disputed',50,$2,$2,$3)`, fixture.eventIDs["a"], fixture.asOf, futureCreatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO event_updates (event_id,sequence_no,kind,summary,observed_at,reason_codes,before_state,after_state,evidence_set_hash,idempotency_key,created_at) VALUES ($1,1,'metric_changed','future-created update',$2,'{}','{}','{}',repeat('e',64),repeat('9',64),$3)`, fixture.eventIDs["e"], fixture.asOf.Add(-time.Minute), futureCreatedAt); err != nil {
		t.Fatal(err)
	}
	lateEventID := seedRadarEvent(t, runtime, "late-created", fixture.asOf.Add(-time.Minute), 100, 100, eventdomain.TrendRising, 10, eventdomain.LifecycleActive, fixture.monitor, 100, 100, "visible")
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE events SET created_at = $1 WHERE id = $2`, futureCreatedAt, lateEventID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE monitor_events SET status = 'visible', created_at = $1 WHERE monitor_id = $2 AND event_id = $3`, futureCreatedAt, fixture.monitor, fixture.eventIDs["a"]); err != nil {
		t.Fatal(err)
	}

	repository := NewRadarRepository(runtime)
	global, err := repository.ListRadar(ctx, eventdomain.RadarQuery{Window: eventdomain.RadarWindow7Days, Sort: eventdomain.RadarSortAttention, Limit: 100, AsOf: fixture.asOf})
	if err != nil {
		t.Fatal(err)
	}
	assertRadarOrder(t, global.Items, fixture, []string{"a", "b", "c", "d", "e"})
	itemA := radarItemByID(t, global.Items, fixture.eventIDs["a"])
	if itemA.Attention != 95 || itemA.Confirmation != eventdomain.RadarConfirmationInsufficient {
		t.Fatalf("future-created metric or claim leaked into as_of page: %#v", itemA)
	}
	itemE := radarItemByID(t, global.Items, fixture.eventIDs["e"])
	if itemE.LatestUpdate != nil {
		t.Fatalf("future-created EventUpdate leaked into as_of page: %#v", itemE.LatestUpdate)
	}
	monitor, err := repository.ListRadar(ctx, eventdomain.RadarQuery{Window: eventdomain.RadarWindow7Days, MonitorID: &fixture.monitor, Sort: eventdomain.RadarSortRelevance, Limit: 100, AsOf: fixture.asOf})
	if err != nil {
		t.Fatal(err)
	}
	assertRadarOrder(t, monitor.Items, fixture, []string{"c", "e", "d", "b"})
}

func TestRadarRepositoryCursorFreezesAsOfAndRejectsAnotherShape(t *testing.T) {
	ctx, runtime, fixture := seedRadarRepositoryFixture(t)
	defer runtime.Close()
	repository := NewRadarRepository(runtime)
	mintedAt := fixture.asOf.Add(-2 * time.Hour)
	repository.now = func() time.Time { return mintedAt }
	query := eventdomain.RadarQuery{
		Window: eventdomain.RadarWindow7Days, MonitorID: &fixture.monitor,
		Lifecycles:    []eventdomain.LifecycleStatus{eventdomain.LifecycleDetected, eventdomain.LifecycleActive, eventdomain.LifecycleCooling},
		Verifications: []eventdomain.RadarConfirmation{eventdomain.RadarConfirmationDisputed, eventdomain.RadarConfirmationCorroborated, eventdomain.RadarConfirmationSingleSource, eventdomain.RadarConfirmationUnverified},
		Sort:          eventdomain.RadarSortRelevance, Limit: 1, AsOf: fixture.asOf,
	}
	first, err := repository.ListRadar(ctx, query)
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" || !first.AsOf.Equal(fixture.asOf) {
		t.Fatalf("first page = %#v/%v", first, err)
	}
	assertRadarOrder(t, first.Items, fixture, []string{"c"})

	// Every mutable projection that could otherwise alter membership or order is
	// changed after page one. The cursor must retain the original top-100 IDs,
	// scores, and tie-break timestamps while later pages re-read only details.
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE events SET lifecycle_status = 'closed', last_seen_at = $1 WHERE id = ANY($2)`, fixture.asOf.Add(-6*24*time.Hour), []int64{fixture.eventIDs["e"], fixture.eventIDs["d"], fixture.eventIDs["b"]}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE monitor_events SET final_score = CASE event_id WHEN $1 THEN 1 WHEN $2 THEN 100 ELSE 99 END, status = 'hidden' WHERE monitor_id = $3 AND event_id = ANY($4)`, fixture.eventIDs["e"], fixture.eventIDs["d"], fixture.monitor, []int64{fixture.eventIDs["e"], fixture.eventIDs["d"], fixture.eventIDs["b"]}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE event_claims SET status = 'retracted' WHERE event_id = ANY($1)`, []int64{fixture.eventIDs["e"], fixture.eventIDs["d"], fixture.eventIDs["b"]}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO event_metric_snapshots (event_id,captured_at,heat_score,trend_score,source_count,content_count,heat_version,evidence_set_hash,capability_profile_set_hash,window_hours,trend_status,created_at) VALUES ($1,$2,100,100,20,20,'heat-v1',repeat('c',64),repeat('d',64),24,'rising',$3)`, fixture.eventIDs["b"], fixture.asOf.Add(-time.Minute), fixture.asOf.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	seedRadarEvent(t, runtime, "future", fixture.asOf.Add(time.Minute), 100, 100, eventdomain.TrendRising, 10, eventdomain.LifecycleActive, fixture.monitor, 100, 100, "visible")

	query.Cursor, query.Limit, query.AsOf = first.NextCursor, 2, fixture.asOf.Add(24*time.Hour)
	second, err := repository.ListRadar(ctx, query)
	if err != nil || !second.AsOf.Equal(fixture.asOf) || second.NextCursor == "" {
		t.Fatalf("second page = %#v/%v", second, err)
	}
	assertRadarOrder(t, second.Items, fixture, []string{"e", "d"})
	assertFrozenRadarPosition(t, second.Items[0], 85, fixture.asOf.Add(-30*time.Minute))
	assertFrozenRadarPosition(t, second.Items[1], 80, fixture.asOf.Add(-30*time.Minute))
	query.Cursor, query.Limit = second.NextCursor, 100
	third, err := repository.ListRadar(ctx, query)
	if err != nil || third.NextCursor != "" || !third.AsOf.Equal(fixture.asOf) {
		t.Fatalf("third page = %#v/%v", third, err)
	}
	assertRadarOrder(t, third.Items, fixture, []string{"b"})
	assertFrozenRadarPosition(t, third.Items[0], 70, fixture.asOf.Add(-2*time.Hour))

	seen := map[int64]bool{}
	items := append(append(append([]eventdomain.RadarEvent(nil), first.Items...), second.Items...), third.Items...)
	for _, item := range items {
		if seen[item.EventID] {
			t.Fatalf("event %d repeated across frozen pages", item.EventID)
		}
		seen[item.EventID] = true
	}
	if len(seen) != 4 {
		t.Fatalf("frozen cursor returned %d original monitor events, want 4", len(seen))
	}
	for _, changed := range []eventdomain.RadarQuery{
		{Window: eventdomain.RadarWindow24Hours, MonitorID: &fixture.monitor, Lifecycles: query.Lifecycles, Verifications: query.Verifications, Sort: eventdomain.RadarSortRelevance, Limit: 10, Cursor: first.NextCursor},
		{Window: eventdomain.RadarWindow7Days, MonitorID: &fixture.monitor, Lifecycles: query.Lifecycles, Verifications: query.Verifications, Sort: eventdomain.RadarSortAttention, Limit: 10, Cursor: first.NextCursor},
		{Window: eventdomain.RadarWindow7Days, MonitorID: &fixture.monitor, Lifecycles: query.Lifecycles, Verifications: query.Verifications, Sort: eventdomain.RadarSortRelevance, MinHeat: radarRepositoryScore(50), Limit: 10, Cursor: first.NextCursor},
	} {
		if _, err := repository.ListRadar(ctx, changed); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("cross-shape cursor error = %v, want ErrInvalidInput", err)
		}
	}
}

func TestRadarRepositoryRejectsEverySignedCursorPayloadMutationAndExpiry(t *testing.T) {
	ctx, runtime, fixture := seedRadarRepositoryFixture(t)
	defer runtime.Close()
	repository := NewRadarRepository(runtime)
	mintedAt := fixture.asOf.Add(-time.Hour)
	repository.now = func() time.Time { return mintedAt }
	query := eventdomain.RadarQuery{Window: eventdomain.RadarWindow7Days, Sort: eventdomain.RadarSortBreadth, Limit: 1, AsOf: fixture.asOf}
	first, err := repository.ListRadar(ctx, query)
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page = %#v/%v", first, err)
	}
	for _, field := range []string{"as_of", "score", "last_seen_at", "event_id", "remaining"} {
		t.Run(field, func(t *testing.T) {
			changed := query
			changed.Cursor = tamperRadarCursorPayload(t, first.NextCursor, field)
			if _, err := repository.ListRadar(ctx, changed); !errors.Is(err, sharedrepository.ErrInvalidInput) {
				t.Fatalf("tampered %s cursor error = %v, want ErrInvalidInput", field, err)
			}
		})
	}
	repository.now = func() time.Time { return mintedAt.Add(radarCursorTTL + time.Nanosecond) }
	query.Cursor = first.NextCursor
	if _, err := repository.ListRadar(ctx, query); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("expired cursor error = %v, want ErrInvalidInput", err)
	}
}

func TestRadarRepositoryProductionConstructorUsesAContextDerivedSecret(t *testing.T) {
	_, runtime, _ := seedRadarRepositoryFixture(t)
	defer runtime.Close()
	if _, err := NewRadarRepositoryWithSigningSecret(runtime, ""); err == nil {
		t.Fatal("production Radar repository accepted an empty signing secret")
	}
	const secret = "radar-production-secret-with-at-least-32-bytes"
	repository, err := NewRadarRepositoryWithSigningSecret(runtime, secret)
	if err != nil {
		t.Fatal(err)
	}
	want := hmac.New(sha256.New, []byte(secret))
	_, _ = want.Write([]byte(radarCursorSigningContext))
	if !hmac.Equal(repository.cursorSigningKey, want.Sum(nil)) {
		t.Fatal("production Radar cursor key was not derived with the radar-cursor-v1 context")
	}
	fallback := NewRadarRepository(runtime)
	wantFallback := hmac.New(sha256.New, []byte(runtime.Pool.Config().ConnString()))
	_, _ = wantFallback.Write([]byte(radarCursorSigningContext))
	if !hmac.Equal(fallback.cursorSigningKey, wantFallback.Sum(nil)) {
		t.Fatal("test Radar cursor key was not derived from the runtime database connection string")
	}
	if hmac.Equal(fallback.cursorSigningKey, repository.cursorSigningKey) {
		t.Fatal("test constructor reused the caller-provided production secret")
	}
}

func TestRadarRepositoryLimitsTheFrozenCandidateSetToOneHundred(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	asOf := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO events (event_key,title_zh,summary,lifecycle_status,first_seen_at,last_seen_at,created_at)
SELECT 'radar-cap-' || value, 'Radar cap ' || value, '', 'active', $1::timestamptz - value * interval '1 minute', $1::timestamptz - value * interval '1 minute', $1::timestamptz - interval '2 hours'
FROM generate_series(1, 105) value`, asOf); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO event_metric_snapshots (event_id,captured_at,heat_score,trend_score,source_count,content_count,heat_version,evidence_set_hash,capability_profile_set_hash,window_hours,trend_status,created_at)
SELECT id,last_seen_at,100,0,1,1,'heat-v1',repeat('a',64),repeat('b',64),24,'stable',$1::timestamptz - interval '2 hours'
FROM events WHERE event_key LIKE 'radar-cap-%'`, asOf); err != nil {
		t.Fatal(err)
	}
	repository := NewRadarRepository(runtime)
	repository.now = func() time.Time { return asOf.Add(-time.Hour) }
	query := eventdomain.RadarQuery{Window: eventdomain.RadarWindow7Days, Sort: eventdomain.RadarSortAttention, Limit: 17, AsOf: asOf}
	seen := map[int64]bool{}
	for {
		page, err := repository.ListRadar(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			if seen[item.EventID] {
				t.Fatalf("event %d repeated", item.EventID)
			}
			seen[item.EventID] = true
		}
		if page.NextCursor == "" {
			break
		}
		query.Cursor, query.Limit = page.NextCursor, 31
	}
	if len(seen) != eventdomain.RadarCursorMaximumEvents {
		t.Fatalf("frozen Radar candidate count = %d, want %d", len(seen), eventdomain.RadarCursorMaximumEvents)
	}
}

func TestRadarRepositoryTimeBoundaryUsesAnIndex(t *testing.T) {
	ctx, runtime, fixture := seedRadarRepositoryFixture(t)
	defer runtime.Close()
	if _, err := runtime.SQL.ExecContext(ctx, `SET enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	rows, err := runtime.SQL.QueryContext(ctx, `EXPLAIN (COSTS OFF) SELECT id FROM events WHERE deleted_at IS NULL AND last_seen_at >= $1 AND last_seen_at <= $2 ORDER BY last_seen_at DESC,id DESC LIMIT 100`, fixture.asOf.Add(-7*24*time.Hour), fixture.asOf)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
	}
	if strings.Contains(plan.String(), "Seq Scan on events") || !strings.Contains(plan.String(), "Index") {
		t.Fatalf("Radar time-boundary EXPLAIN is unbounded:\n%s", plan.String())
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO events (event_key,title_zh,summary,lifecycle_status,first_seen_at,last_seen_at)
SELECT 'radar-capacity-' || value, '容量事件 ' || value, '不相关摘要', 'active', $1, $1
FROM generate_series(1,1000) AS value`, fixture.asOf); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `ANALYZE events`); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `SET enable_indexscan = off`); err != nil {
		t.Fatal(err)
	}
	keywordRows, err := runtime.SQL.QueryContext(ctx, `EXPLAIN (COSTS OFF) SELECT id FROM events WHERE deleted_at IS NULL AND lower(title_zh || ' ' || COALESCE(title_en,'') || ' ' || summary) LIKE '%radar b%' LIMIT 100`)
	if err != nil {
		t.Fatal(err)
	}
	defer keywordRows.Close()
	var keywordPlan strings.Builder
	for keywordRows.Next() {
		var line string
		if err := keywordRows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		keywordPlan.WriteString(line)
	}
	if strings.Contains(keywordPlan.String(), "Seq Scan on events") || !strings.Contains(keywordPlan.String(), "Index") {
		t.Fatalf("Radar keyword EXPLAIN is unbounded:\n%s", keywordPlan.String())
	}
	var keywordIndexExists bool
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname='events_search_active_trgm_idx')`).Scan(&keywordIndexExists); err != nil || !keywordIndexExists {
		t.Fatalf("Radar keyword index exists/error = %t/%v", keywordIndexExists, err)
	}
}

func seedRadarRepositoryFixture(t *testing.T) (context.Context, *database.Runtime, radarFixture) {
	t.Helper()
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		runtime.Close()
		t.Fatal(err)
	}
	fixture := radarFixture{asOf: time.Now().UTC().Add(time.Hour).Truncate(time.Second), eventIDs: map[string]int64{}}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name,status) VALUES ('radar-monitor','active') RETURNING id`).Scan(&fixture.monitor); err != nil {
		t.Fatal(err)
	}
	fixture.eventIDs["a"] = seedRadarEvent(t, runtime, "a", fixture.asOf.Add(-12*time.Hour), 95, -40, eventdomain.TrendFalling, 1, eventdomain.LifecycleActive, fixture.monitor, 40, 50, "hidden")
	fixture.eventIDs["b"] = seedRadarEvent(t, runtime, "b", fixture.asOf.Add(-2*time.Hour), 80, 90, eventdomain.TrendRising, 2, eventdomain.LifecycleDetected, fixture.monitor, 65, 70, "visible")
	fixture.eventIDs["c"] = seedRadarEvent(t, runtime, "c", fixture.asOf.Add(-72*time.Hour), 70, 0, eventdomain.TrendStable, 5, eventdomain.LifecycleCooling, fixture.monitor, 91, 99, "visible")
	fixture.eventIDs["d"] = seedRadarEvent(t, runtime, "d", fixture.asOf.Add(-30*time.Minute), 60, 20, eventdomain.TrendEmerging, 3, eventdomain.LifecycleActive, fixture.monitor, 75, 80, "visible")
	fixture.eventIDs["e"] = seedRadarEvent(t, runtime, "e", fixture.asOf.Add(-30*time.Minute), 55, -60, eventdomain.TrendStable, 3, eventdomain.LifecycleActive, fixture.monitor, 82, 85, "visible")
	for key, status := range map[string]string{"a": "retracted", "b": "disputed", "c": "corroborated", "d": "single_source", "e": "unverified"} {
		if _, err := runtime.SQL.Exec(`INSERT INTO event_claims (event_id,normalized_claim,claim_hash,status,confidence,first_seen_at,last_seen_at) VALUES ($1,$2,$3,$4,50,$5,$5)`, fixture.eventIDs[key], "claim-"+key, strings.Repeat(key, 64), status, fixture.asOf); err != nil {
			t.Fatalf("insert claim %s: %v", key, err)
		}
	}
	updates := NewUpdateRepository(runtime)
	first := eventUpdateCandidate(t, nil, repositoryEventUpdateHeat(fixture.eventIDs["d"], fixture.asOf.Add(-time.Hour), 40, eventdomain.TrendStable, 1))
	inserted, _, err := updates.AppendUpdate(ctx, *first)
	if err != nil {
		t.Fatal(err)
	}
	second := eventUpdateCandidate(t, &first.AfterState, repositoryEventUpdateHeat(fixture.eventIDs["d"], fixture.asOf, 60, eventdomain.TrendRising, 3))
	if _, _, err := updates.AppendUpdate(ctx, *second); err != nil || inserted == nil {
		t.Fatalf("seed latest update: %v", err)
	}
	return ctx, runtime, fixture
}

func seedRadarEvent(t *testing.T, runtime *database.Runtime, key string, lastSeen time.Time, heat, trend float64, trendStatus eventdomain.TrendStatus, sources int, lifecycle eventdomain.LifecycleStatus, monitorID int64, relevance, final float64, monitorStatus string) int64 {
	t.Helper()
	var eventID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO events (event_key,title_zh,summary,lifecycle_status,heat_score,trend_score,trend_status,first_seen_at,last_seen_at,heat_calculated_at) VALUES ($1,$2,'',$3,$4,$5,$6,$7,$7,$7) RETURNING id`, "radar-"+key, "Radar "+key, lifecycle, heat, trend, trendStatus, lastSeen).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO event_metric_snapshots (event_id,captured_at,heat_score,trend_score,source_count,content_count,heat_version,evidence_set_hash,capability_profile_set_hash,window_hours,trend_status) VALUES ($1,$2,$3,$4,$5,$6,'heat-v1',repeat('a',64),repeat('b',64),24,$7)`, eventID, lastSeen, heat, trend, sources, sources+1, trendStatus); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO monitor_events (monitor_id,event_id,relevance_score,final_score,first_matched_at,last_matched_at,status) VALUES ($1,$2,$3,$4,$5,$5,$6)`, monitorID, eventID, relevance, final, lastSeen, monitorStatus); err != nil {
		t.Fatal(err)
	}
	return eventID
}

func ordersPage(t *testing.T, repository *RadarRepository, ctx context.Context, asOf time.Time) []eventdomain.RadarEvent {
	t.Helper()
	page, err := repository.ListRadar(ctx, eventdomain.RadarQuery{Window: eventdomain.RadarWindow7Days, Sort: eventdomain.RadarSortMomentum, Limit: 100, AsOf: asOf})
	if err != nil {
		t.Fatal(err)
	}
	return page.Items
}

func radarItemByID(t *testing.T, items []eventdomain.RadarEvent, eventID int64) eventdomain.RadarEvent {
	t.Helper()
	for _, item := range items {
		if item.EventID == eventID {
			return item
		}
	}
	t.Fatalf("Radar event %d not found", eventID)
	return eventdomain.RadarEvent{}
}

func assertRadarOrder(t *testing.T, items []eventdomain.RadarEvent, fixture radarFixture, keys []string) {
	t.Helper()
	if len(items) != len(keys) {
		t.Fatalf("Radar item count = %d, want %d: %#v", len(items), len(keys), items)
	}
	for index, key := range keys {
		if items[index].EventID != fixture.eventIDs[key] {
			t.Fatalf("Radar order[%d] = %d, want %s/%d", index, items[index].EventID, key, fixture.eventIDs[key])
		}
	}
}

func assertFrozenRadarPosition(t *testing.T, item eventdomain.RadarEvent, score float64, lastSeenAt time.Time) {
	t.Helper()
	if item.RankingScore != score || !item.LastSeenAt.Equal(lastSeenAt) {
		t.Fatalf("frozen Radar position = score %.2f / last_seen_at %s, want %.2f / %s", item.RankingScore, item.LastSeenAt, score, lastSeenAt)
	}
}

func tamperRadarCursorPayload(t *testing.T, encoded, field string) string {
	t.Helper()
	parts := strings.Split(encoded, ".")
	if len(parts) != 2 {
		t.Fatalf("signed cursor has %d parts, want 2", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatal(err)
	}
	switch field {
	case "as_of":
		value[field] = time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	case "score":
		value[field] = value[field].(float64) + 1
	case "last_seen_at":
		value[field] = time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)
	case "event_id":
		value[field] = value[field].(float64) + 1
	case "remaining":
		remaining := value[field].([]any)
		position := remaining[0].(map[string]any)
		position["event_id"] = position["event_id"].(float64) + 1
	default:
		t.Fatalf("unknown cursor field %q", field)
	}
	changed, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(changed) + "." + parts[1]
}

func radarRepositoryScore(value float64) *float64 { return &value }
