package domain

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestEventUpdateDetectsMaterialChangesByPriority(t *testing.T) {
	now := time.Date(2026, 8, 4, 1, 2, 3, 123456789, time.UTC)
	base := eventUpdateHeatFixture(now)
	previousBase := eventUpdateHeatFixture(now.Add(-time.Hour))
	tests := []struct {
		name       string
		previous   *HeatResult
		change     func(*HeatResult)
		wantKind   EventUpdateKind
		wantReason []string
	}{
		{name: "first snapshot", wantKind: EventUpdateEventCreated, wantReason: []string{"first_snapshot"}},
		{name: "reactivation wins over every lower priority rule", previous: eventUpdateHeatPointer(previousBase), change: func(current *HeatResult) {
			current.HeatScore, current.TrendStatus, current.SourceCount = 82, TrendRising, 4
		}, wantKind: EventUpdateReactivated, wantReason: []string{"reactivated", "rising", "source_expansion", "heat_delta"}},
		{name: "first rising status", previous: eventUpdateHeatPointer(eventUpdateHeatWith(previousBase, 50, TrendStable, 2)), change: func(current *HeatResult) {
			current.HeatScore, current.TrendStatus = 55, TrendEmerging
		}, wantKind: EventUpdateRising, wantReason: []string{"rising"}},
		{name: "first cooling status", previous: eventUpdateHeatPointer(eventUpdateHeatWith(previousBase, 50, TrendStable, 2)), change: func(current *HeatResult) {
			current.HeatScore, current.TrendStatus = 45, TrendFalling
		}, wantKind: EventUpdateCooling, wantReason: []string{"cooling"}},
		{name: "two additional independent sources", previous: eventUpdateHeatPointer(eventUpdateHeatWith(previousBase, 50, TrendStable, 3)), change: func(current *HeatResult) {
			current.HeatScore, current.TrendStatus, current.SourceCount = 55, TrendStable, 5
		}, wantKind: EventUpdateSourceExpansion, wantReason: []string{"source_expansion"}},
		{name: "source count doubles from nonzero baseline", previous: eventUpdateHeatPointer(eventUpdateHeatWith(previousBase, 50, TrendStable, 1)), change: func(current *HeatResult) {
			current.HeatScore, current.TrendStatus, current.SourceCount = 55, TrendStable, 2
		}, wantKind: EventUpdateSourceExpansion, wantReason: []string{"source_expansion"}},
		{name: "absolute heat delta at boundary", previous: eventUpdateHeatPointer(eventUpdateHeatWith(previousBase, 50, TrendStable, 2)), change: func(current *HeatResult) {
			current.HeatScore, current.TrendStatus = 60, TrendStable
		}, wantKind: EventUpdateMetricChanged, wantReason: []string{"heat_delta"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := base
			if test.change != nil {
				test.change(&current)
			}
			candidate, err := DetectEventUpdate(test.previous, current)
			if err != nil {
				t.Fatalf("DetectEventUpdate() error = %v", err)
			}
			if candidate == nil || candidate.Kind != test.wantKind {
				t.Fatalf("DetectEventUpdate() = %#v, want kind %q", candidate, test.wantKind)
			}
			for _, reason := range test.wantReason {
				if !eventUpdateContainsReason(candidate.ReasonCodes, reason) {
					t.Fatalf("reason_codes = %#v, want %q", candidate.ReasonCodes, reason)
				}
			}
			if candidate.EventID != current.EventID || candidate.ObservedAt != current.WindowEnd || candidate.EvidenceSetHash != current.EvidenceSetHash || len(candidate.IdempotencyKey) != 64 {
				t.Fatalf("candidate did not freeze the current metric input: %#v", candidate)
			}
		})
	}
}

func TestEventUpdateIgnoresNonMaterialRecomputation(t *testing.T) {
	previous := eventUpdateHeatFixture(time.Date(2026, 8, 4, 1, 0, 0, 0, time.UTC))
	current := previous
	current.WindowEnd = current.WindowEnd.Add(time.Hour)
	current.HeatScore = previous.HeatScore + 9.99
	current.SourceCount++
	current.EvidenceSetHash = strings.Repeat("c", 64)
	if candidate, err := DetectEventUpdate(&previous, current); err != nil || candidate != nil {
		t.Fatalf("DetectEventUpdate() = %#v/%v, want nil candidate", candidate, err)
	}
}

func TestEventUpdateIdempotencyKeyUsesMetricUpdateV1OrderedInput(t *testing.T) {
	current := eventUpdateHeatFixture(time.Date(2026, 8, 4, 1, 2, 3, 123456789, time.UTC))
	current.HeatScore = 81.2
	current.TrendScore = -12.35
	current.TrendStatus = TrendRising
	current.SourceCount = 3
	current.ContentCount = 9
	key, err := EventUpdateIdempotencyKey(current)
	if err != nil {
		t.Fatalf("EventUpdateIdempotencyKey() error = %v", err)
	}
	const want = "88d0e010d482f63ca7261ff82d50b12d7b566cb2653ce82275b1b9f462e0a3b9"
	if key != want {
		t.Fatalf("EventUpdateIdempotencyKey() = %q, want %q", key, want)
	}

	sameInstant := current
	sameInstant.WindowEnd = current.WindowEnd.In(time.FixedZone("UTC+8", 8*60*60))
	if got, err := EventUpdateIdempotencyKey(sameInstant); err != nil || got != want {
		t.Fatalf("timezone-equivalent key = %q/%v, want %q", got, err, want)
	}
	changed := current
	changed.CapabilityProfileSetHash = strings.Repeat("d", 64)
	if got, err := EventUpdateIdempotencyKey(changed); err != nil || got == want {
		t.Fatalf("capability-profile change key = %q/%v, want a distinct key", got, err)
	}
}

func eventUpdateHeatFixture(observedAt time.Time) HeatResult {
	return HeatResult{
		EventID: 42, HeatScore: 50, TrendScore: 0, TrendStatus: TrendDormant,
		SourceCount: 2, ContentCount: 4, HeatVersion: HeatAlgorithmVersionV1,
		EvidenceSetHash: strings.Repeat("a", 64), CapabilityProfileSetHash: strings.Repeat("b", 64),
		WindowHours: 24, WindowEnd: observedAt,
	}
}

func eventUpdateHeatWith(value HeatResult, heat float64, trend TrendStatus, sources int) HeatResult {
	value.HeatScore, value.TrendStatus, value.SourceCount = heat, trend, sources
	return value
}

func eventUpdateHeatPointer(value HeatResult) *HeatResult { return &value }

func eventUpdateContainsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func TestRadarDimensionsAndFiveRankingScoresUseTheV1Formula(t *testing.T) {
	asOf := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	dimensions, err := CalculateRadarDimensions(RadarDimensionInput{HeatScore: 72.345, TrendScore: 25, IndependentSourceCount: 2, LastSeenAt: asOf.Add(-24 * time.Hour), AsOf: asOf})
	if err != nil {
		t.Fatal(err)
	}
	if dimensions.Attention != 72.35 || dimensions.Momentum != 62.5 || dimensions.Breadth != 50 || dimensions.Freshness != 50 || dimensions.DataConfidence != 50 {
		t.Fatalf("dimensions = %#v", dimensions)
	}
	monitorScore := 88.126
	for _, testCase := range []struct {
		sort RadarSort
		want float64
	}{{RadarSortMomentum, 62.5}, {RadarSortAttention, 72.35}, {RadarSortBreadth, 50}, {RadarSortLatest, 50}, {RadarSortRelevance, 88.13}} {
		got, err := RadarRankingScore(testCase.sort, dimensions, &monitorScore)
		if err != nil || got != testCase.want {
			t.Fatalf("RadarRankingScore(%q) = %v/%v, want %v", testCase.sort, got, err, testCase.want)
		}
	}
	if _, err := RadarRankingScore(RadarSortRelevance, dimensions, nil); err == nil {
		t.Fatal("relevance ranking accepted no monitor score")
	}
	clipped, err := CalculateRadarDimensions(RadarDimensionInput{HeatScore: 120, TrendScore: -120, IndependentSourceCount: 9, LastSeenAt: asOf.Add(time.Hour), AsOf: asOf})
	if err != nil || clipped.Attention != 100 || clipped.Momentum != 0 || clipped.Breadth != 100 || clipped.Freshness != 100 || clipped.DataConfidence != 100 {
		t.Fatalf("clipped dimensions = %#v/%v", clipped, err)
	}
	if _, err := CalculateRadarDimensions(RadarDimensionInput{HeatScore: math.NaN(), LastSeenAt: asOf, AsOf: asOf}); err == nil {
		t.Fatal("formula accepted NaN")
	}
}

func TestRadarConfirmationPriorityAndInsufficientNull(t *testing.T) {
	for _, testCase := range []struct {
		statuses []ClaimStatus
		want     RadarConfirmation
		score    *float64
	}{
		{nil, RadarConfirmationInsufficient, nil},
		{[]ClaimStatus{ClaimRetracted}, RadarConfirmationInsufficient, nil},
		{[]ClaimStatus{ClaimUnverified}, RadarConfirmationUnverified, radarScorePointer(30)},
		{[]ClaimStatus{ClaimUnverified, ClaimSingleSource}, RadarConfirmationSingleSource, radarScorePointer(60)},
		{[]ClaimStatus{ClaimSingleSource, ClaimCorroborated}, RadarConfirmationCorroborated, radarScorePointer(100)},
		{[]ClaimStatus{ClaimCorroborated, ClaimDisputed}, RadarConfirmationDisputed, radarScorePointer(20)},
	} {
		got, err := DeriveRadarConfirmation(testCase.statuses)
		if err != nil || got.Status != testCase.want || !sameRadarScore(got.Score, testCase.score) {
			t.Fatalf("DeriveRadarConfirmation(%#v) = %#v/%v", testCase.statuses, got, err)
		}
	}
}

func TestRadarQueryAndCursorBindSemanticShapeButAllowLimitChanges(t *testing.T) {
	asOf := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	monitorID, minHeat := int64(7), 40.0
	base := RadarQuery{Window: RadarWindow24Hours, Keyword: "发布", MonitorID: &monitorID, Lifecycles: []LifecycleStatus{LifecycleActive, LifecycleCooling}, Trends: []TrendStatus{TrendRising, TrendStable}, Verifications: []RadarConfirmation{RadarConfirmationCorroborated, RadarConfirmationDisputed}, MinHeat: &minHeat, Sort: RadarSortRelevance, Limit: 25, AsOf: asOf}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid query: %v", err)
	}
	baseHash, err := base.ShapeHash()
	if err != nil || len(baseHash) != 64 {
		t.Fatalf("ShapeHash() = %q/%v", baseHash, err)
	}
	equivalent := base
	equivalent.Limit, equivalent.Cursor, equivalent.AsOf = 100, "opaque", asOf.Add(time.Hour)
	equivalent.Lifecycles = []LifecycleStatus{LifecycleCooling, LifecycleActive}
	equivalent.Trends = []TrendStatus{TrendStable, TrendRising}
	equivalent.Verifications = []RadarConfirmation{RadarConfirmationDisputed, RadarConfirmationCorroborated}
	if got, _ := equivalent.ShapeHash(); got != baseHash {
		t.Fatalf("limit/cursor/as_of/filter order changed shape: %q != %q", got, baseHash)
	}
	for _, mutate := range []func(*RadarQuery){
		func(q *RadarQuery) { q.Window = RadarWindow7Days },
		func(q *RadarQuery) { q.Keyword = "另一条" },
		func(q *RadarQuery) { id := int64(8); q.MonitorID = &id },
		func(q *RadarQuery) { q.Lifecycles = []LifecycleStatus{LifecycleActive} },
		func(q *RadarQuery) { q.Trends = []TrendStatus{TrendRising} },
		func(q *RadarQuery) { q.Verifications = []RadarConfirmation{RadarConfirmationDisputed} },
		func(q *RadarQuery) { value := 41.0; q.MinHeat = &value },
		func(q *RadarQuery) { q.Sort = RadarSortMomentum },
	} {
		changed := base
		mutate(&changed)
		if got, _ := changed.ShapeHash(); got == baseHash {
			t.Fatalf("semantic change did not change shape: %#v", changed)
		}
	}
	cursor := RadarCursor{
		Version: RadarCursorVersionV1, AsOf: asOf, ExpiresAt: asOf.Add(15 * time.Minute), ShapeHash: baseHash,
		RankingScore: 75, LastSeenAt: asOf.Add(-time.Hour), EventID: 9,
		Remaining: []RadarCursorPosition{{EventID: 8, RankingScore: 70, LastSeenAt: asOf.Add(-2 * time.Hour)}},
	}
	if err := cursor.ValidateForAt(equivalent, asOf); err != nil {
		t.Fatalf("cursor rejected equivalent query: %v", err)
	}
	changed := base
	changed.Sort = RadarSortMomentum
	if err := cursor.ValidateForAt(changed, asOf); err == nil {
		t.Fatal("cursor accepted another query shape")
	}
}

func TestRadarCursorRejectsExpiredOrMalformedFrozenPositions(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	query := RadarQuery{Window: RadarWindow24Hours, Sort: RadarSortMomentum, Limit: 25, AsOf: now}
	shapeHash, err := query.ShapeHash()
	if err != nil {
		t.Fatal(err)
	}
	valid := RadarCursor{
		Version: RadarCursorVersionV1, AsOf: now, ExpiresAt: now.Add(15 * time.Minute), ShapeHash: shapeHash,
		RankingScore: 75, LastSeenAt: now.Add(-time.Hour), EventID: 9,
		Remaining: []RadarCursorPosition{
			{EventID: 8, RankingScore: 70, LastSeenAt: now.Add(-2 * time.Hour)},
			{EventID: 7, RankingScore: 70, LastSeenAt: now.Add(-3 * time.Hour)},
		},
	}
	if err := valid.ValidateForAt(query, now); err != nil {
		t.Fatalf("valid frozen cursor: %v", err)
	}

	for name, mutate := range map[string]func(*RadarCursor){
		"expired":                func(cursor *RadarCursor) { cursor.ExpiresAt = now.Add(-time.Nanosecond) },
		"expiry before snapshot": func(cursor *RadarCursor) { cursor.ExpiresAt = now.Add(-time.Minute) },
		"duplicate event":        func(cursor *RadarCursor) { cursor.Remaining[1].EventID = cursor.Remaining[0].EventID },
		"unordered score":        func(cursor *RadarCursor) { cursor.Remaining[0].RankingScore = 76 },
		"unordered tie":          func(cursor *RadarCursor) { cursor.Remaining[1].LastSeenAt = now.Add(-time.Hour) },
		"future last seen":       func(cursor *RadarCursor) { cursor.Remaining[0].LastSeenAt = now.Add(time.Second) },
		"invalid event":          func(cursor *RadarCursor) { cursor.Remaining[0].EventID = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			cursor := valid
			cursor.Remaining = append([]RadarCursorPosition(nil), valid.Remaining...)
			mutate(&cursor)
			if err := cursor.ValidateForAt(query, now); err == nil {
				t.Fatalf("ValidateForAt accepted %#v", cursor)
			}
		})
	}

	tooMany := valid
	tooMany.Remaining = make([]RadarCursorPosition, RadarCursorMaximumEvents)
	for index := range tooMany.Remaining {
		tooMany.Remaining[index] = RadarCursorPosition{EventID: int64(1000 - index), RankingScore: 70, LastSeenAt: now.Add(-time.Duration(index+2) * time.Hour)}
	}
	if err := tooMany.ValidateForAt(query, now); err == nil {
		t.Fatal("cursor accepted more than the remaining positions possible in a top-100 snapshot")
	}
}

func TestRadarQueryRejectsInvalidValuesAndRelevanceWithoutMonitor(t *testing.T) {
	valid := RadarQuery{Window: RadarWindow24Hours, Sort: RadarSortMomentum, Limit: 50, AsOf: time.Now().UTC()}
	for _, mutate := range []func(*RadarQuery){
		func(q *RadarQuery) { q.Window = "2h" },
		func(q *RadarQuery) { q.Sort = RadarSortRelevance },
		func(q *RadarQuery) { q.Limit = 101 },
		func(q *RadarQuery) { id := int64(0); q.MonitorID = &id },
		func(q *RadarQuery) { value := 101.0; q.MinHeat = &value },
		func(q *RadarQuery) { q.Lifecycles = []LifecycleStatus{"invented"} },
		func(q *RadarQuery) { q.Trends = []TrendStatus{"invented"} },
		func(q *RadarQuery) { q.Verifications = []RadarConfirmation{"invented"} },
	} {
		query := valid
		mutate(&query)
		if err := query.Validate(); err == nil {
			t.Fatalf("Validate accepted %#v", query)
		}
	}
}

func TestRadarQueryRejectsKeywordOverOneHundredCharacters(t *testing.T) {
	query := RadarQuery{Window: RadarWindow24Hours, Keyword: strings.Repeat("界", 101), Sort: RadarSortMomentum, Limit: 20, AsOf: time.Now().UTC()}
	if err := query.Validate(); err == nil {
		t.Fatal("Validate() accepted an oversized public keyword")
	}
}

func radarScorePointer(value float64) *float64 { return &value }
func sameRadarScore(left, right *float64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
