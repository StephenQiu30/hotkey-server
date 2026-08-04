package postgres

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type RadarRepository struct {
	runtime          *database.Runtime
	cursorSigningKey []byte
	now              func() time.Time
}

var _ application.RadarRepository = (*RadarRepository)(nil)

const (
	radarCursorSigningContext = "radar-cursor-v1"
	radarCursorTTL            = 15 * time.Minute
	radarCursorMaxPayloadSize = 32 << 10
)

// NewRadarRepository is retained for package and integration tests. Its key is
// derived from the fixture's private connection string, so tests never rely on
// a hard-coded cursor key that could accidentally be reused in production.
func NewRadarRepository(runtime *database.Runtime) *RadarRepository {
	secret := make([]byte, sha256.Size)
	if runtime != nil && runtime.Pool != nil {
		secret = []byte(runtime.Pool.Config().ConnString())
	} else {
		_, _ = io.ReadFull(rand.Reader, secret)
	}
	return newRadarRepository(runtime, secret)
}

// NewRadarRepositoryWithSigningSecret is the production constructor. The
// caller supplies an application secret; a context-specific HMAC key is
// derived so the same root secret cannot validate another token type.
func NewRadarRepositoryWithSigningSecret(runtime *database.Runtime, secret string) (*RadarRepository, error) {
	if len([]byte(strings.TrimSpace(secret))) < sha256.Size {
		return nil, fmt.Errorf("Radar cursor signing secret must be at least %d bytes", sha256.Size)
	}
	return newRadarRepository(runtime, []byte(secret)), nil
}

func newRadarRepository(runtime *database.Runtime, secret []byte) *RadarRepository {
	derivation := hmac.New(sha256.New, secret)
	_, _ = derivation.Write([]byte(radarCursorSigningContext))
	return &RadarRepository{runtime: runtime, cursorSigningKey: derivation.Sum(nil), now: time.Now}
}

func (repository *RadarRepository) ListRadar(ctx context.Context, query domain.RadarQuery) (domain.RadarPage, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || len(repository.cursorSigningKey) != sha256.Size {
		return domain.RadarPage{}, sharedrepository.ErrUnavailable
	}
	now := time.Now().UTC()
	if repository.now != nil {
		now = repository.now().UTC()
	}
	var cursor domain.RadarCursor
	var err error
	if query.Cursor != "" {
		cursor, err = decodeRadarCursor(query.Cursor, repository.cursorSigningKey)
		if err != nil {
			return domain.RadarPage{}, fmt.Errorf("%w: Radar cursor: %v", sharedrepository.ErrInvalidInput, err)
		}
		query.AsOf = cursor.AsOf.UTC()
	}
	if err := query.Validate(); err != nil {
		return domain.RadarPage{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	shapeHash, err := query.ShapeHash()
	if err != nil {
		return domain.RadarPage{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	if query.Cursor != "" {
		if err := cursor.ValidateForAt(query, now); err != nil {
			return domain.RadarPage{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
		}
	}
	if query.MonitorID != nil {
		var exists bool
		if err := repository.runtime.SQL.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM monitors WHERE id = $1 AND created_at <= $2 AND (deleted_at IS NULL OR deleted_at > $2))`, *query.MonitorID, query.AsOf.UTC()).Scan(&exists); err != nil {
			return domain.RadarPage{}, databaserepository.MapError(err)
		}
		if !exists {
			return domain.RadarPage{}, sharedrepository.ErrNotFound
		}
	}
	if query.Cursor == "" {
		return repository.listInitialRadarPage(ctx, query, shapeHash, now)
	}
	return repository.listFrozenRadarPage(ctx, query, cursor)
}

func (repository *RadarRepository) listInitialRadarPage(ctx context.Context, query domain.RadarQuery, shapeHash string, now time.Time) (domain.RadarPage, error) {
	items, err := repository.queryRadar(ctx, query, nil, true, domain.RadarCursorMaximumEvents)
	if err != nil {
		return domain.RadarPage{}, err
	}
	page := domain.RadarPage{Items: items, AsOf: query.AsOf.UTC()}
	if len(items) <= query.Limit {
		return page, nil
	}
	positions := radarPositions(items)
	page.Items = items[:query.Limit]
	last := positions[query.Limit-1]
	cursor := domain.RadarCursor{
		Version: domain.RadarCursorVersionV1, AsOf: page.AsOf, ExpiresAt: now.Add(radarCursorTTL), ShapeHash: shapeHash,
		RankingScore: last.RankingScore, LastSeenAt: last.LastSeenAt, EventID: last.EventID,
		Remaining: append([]domain.RadarCursorPosition(nil), positions[query.Limit:]...),
	}
	page.NextCursor, err = encodeRadarCursor(cursor, repository.cursorSigningKey)
	if err != nil {
		return domain.RadarPage{}, fmt.Errorf("%w: encode Radar cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return page, nil
}

func (repository *RadarRepository) listFrozenRadarPage(ctx context.Context, query domain.RadarQuery, cursor domain.RadarCursor) (domain.RadarPage, error) {
	count := min(query.Limit, len(cursor.Remaining))
	selected := cursor.Remaining[:count]
	eventIDs := make([]int64, len(selected))
	for index, position := range selected {
		eventIDs[index] = position.EventID
	}
	items, err := repository.queryRadar(ctx, query, eventIDs, false, len(eventIDs))
	if err != nil {
		return domain.RadarPage{}, err
	}
	byID := make(map[int64]domain.RadarEvent, len(items))
	for _, item := range items {
		byID[item.EventID] = item
	}
	page := domain.RadarPage{Items: make([]domain.RadarEvent, 0, len(selected)), AsOf: cursor.AsOf.UTC()}
	for _, position := range selected {
		item, exists := byID[position.EventID]
		if !exists {
			return domain.RadarPage{}, fmt.Errorf("%w: frozen Radar event %d is unavailable", sharedrepository.ErrUnavailable, position.EventID)
		}
		item.RankingScore = position.RankingScore
		item.LastSeenAt = position.LastSeenAt.UTC()
		page.Items = append(page.Items, item)
	}
	remaining := cursor.Remaining[count:]
	if len(remaining) == 0 {
		return page, nil
	}
	last := selected[len(selected)-1]
	next := domain.RadarCursor{
		Version: cursor.Version, AsOf: cursor.AsOf, ExpiresAt: cursor.ExpiresAt, ShapeHash: cursor.ShapeHash,
		RankingScore: last.RankingScore, LastSeenAt: last.LastSeenAt, EventID: last.EventID,
		Remaining: append([]domain.RadarCursorPosition(nil), remaining...),
	}
	page.NextCursor, err = encodeRadarCursor(next, repository.cursorSigningKey)
	if err != nil {
		return domain.RadarPage{}, fmt.Errorf("%w: encode Radar cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return page, nil
}

func (repository *RadarRepository) queryRadar(ctx context.Context, query domain.RadarQuery, eventIDs []int64, applyShape bool, limit int) ([]domain.RadarEvent, error) {
	statement, arguments := radarStatement(query, eventIDs, applyShape, limit)
	rows, err := repository.runtime.SQL.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	items := make([]domain.RadarEvent, 0, limit)
	for rows.Next() {
		item, err := scanRadarEvent(rows, query.Sort)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return items, nil
}

func radarPositions(items []domain.RadarEvent) []domain.RadarCursorPosition {
	positions := make([]domain.RadarCursorPosition, len(items))
	for index, item := range items {
		positions[index] = domain.RadarCursorPosition{EventID: item.EventID, RankingScore: item.RankingScore, LastSeenAt: item.LastSeenAt.UTC()}
	}
	return positions
}

type radarSQLBuilder struct {
	arguments []any
}

func (builder *radarSQLBuilder) bind(value any) string {
	builder.arguments = append(builder.arguments, value)
	return fmt.Sprintf("$%d", len(builder.arguments))
}

func radarStatement(query domain.RadarQuery, eventIDs []int64, applyShape bool, limit int) (string, []any) {
	builder := &radarSQLBuilder{}
	asOf := builder.bind(query.AsOf.UTC())
	windowStart := ""
	if applyShape {
		windowStart = builder.bind(query.AsOf.Add(-query.Window.Duration()).UTC())
	}
	monitorID := int64(0)
	if query.MonitorID != nil {
		monitorID = *query.MonitorID
	}
	monitor := builder.bind(monitorID)
	conditions := []string{
		"event.created_at <= " + asOf,
	}
	if applyShape {
		conditions = append(conditions,
			"event.deleted_at IS NULL",
			"event.last_seen_at >= "+windowStart,
			"event.last_seen_at <= "+asOf,
			"("+monitor+" = 0 OR watch.event_id IS NOT NULL)",
		)
	} else {
		conditions = append(conditions, "event.id = ANY("+builder.bind(eventIDs)+")")
	}
	appendRadarEnumFilter := func(column string, values []string) {
		if !applyShape || len(values) == 0 {
			return
		}
		placeholders := make([]string, 0, len(values))
		for _, value := range values {
			placeholders = append(placeholders, builder.bind(value))
		}
		conditions = append(conditions, column+" IN ("+strings.Join(placeholders, ",")+")")
	}
	lifecycles := make([]string, len(query.Lifecycles))
	for index, value := range query.Lifecycles {
		lifecycles[index] = string(value)
	}
	appendRadarEnumFilter("event.lifecycle_status", lifecycles)
	trends := make([]string, len(query.Trends))
	for index, value := range query.Trends {
		trends[index] = string(value)
	}
	appendRadarEnumFilter("metric.trend_status", trends)
	verifications := make([]string, len(query.Verifications))
	for index, value := range query.Verifications {
		verifications[index] = string(value)
	}
	appendRadarEnumFilter("confirmation.status", verifications)
	if applyShape && query.MinHeat != nil {
		conditions = append(conditions, "metric.heat_score >= "+builder.bind(*query.MinHeat))
	}

	rankingColumn := map[domain.RadarSort]string{
		domain.RadarSortMomentum: "momentum", domain.RadarSortAttention: "attention",
		domain.RadarSortBreadth: "breadth", domain.RadarSortLatest: "freshness",
		domain.RadarSortRelevance: "COALESCE(watch_final_score, 0)",
	}[query.Sort]
	statement := `WITH radar_base AS (
SELECT event.id AS event_id, event.version, event.event_key, event.title_zh,
       COALESCE(event.title_en, '') AS title_en, event.summary, event.lifecycle_status,
       event.first_seen_at, event.last_seen_at,
       metric.trend_score::double precision AS trend_score, metric.trend_status,
       metric.source_count,
       ROUND(LEAST(100::numeric, GREATEST(0::numeric, metric.heat_score)), 2)::double precision AS attention,
       ROUND(LEAST(100::numeric, GREATEST(0::numeric, (metric.trend_score + 100) / 2)), 2)::double precision AS momentum,
       ROUND(LEAST(100::numeric, metric.source_count * 25), 2)::double precision AS breadth,
       ROUND((100 * power(2::numeric, -GREATEST(0, EXTRACT(EPOCH FROM (` + asOf + ` - event.last_seen_at)) / 3600) / 24))::numeric, 2)::double precision AS freshness,
       confirmation.status AS confirmation,
       watch.relevance_score::double precision AS watch_relevance,
       watch.final_score::double precision AS watch_final_score,
       latest_update.value AS latest_update
FROM events event
JOIN LATERAL (
    SELECT snapshot.heat_score, snapshot.trend_score, snapshot.trend_status, snapshot.source_count
    FROM event_metric_snapshots snapshot
    WHERE snapshot.event_id = event.id AND snapshot.window_hours = 24
      AND snapshot.captured_at <= ` + asOf + ` AND snapshot.created_at <= ` + asOf + `
    ORDER BY snapshot.captured_at DESC, snapshot.id DESC LIMIT 1
) metric ON true
LEFT JOIN monitor_events watch
  ON watch.event_id = event.id AND watch.monitor_id = ` + monitor + `
 AND watch.status = 'visible' AND watch.created_at <= ` + asOf + `
JOIN LATERAL (
    SELECT CASE
      WHEN count(*) FILTER (WHERE claim.status <> 'retracted') = 0 THEN 'insufficient'
      WHEN bool_or(claim.status = 'disputed') FILTER (WHERE claim.status <> 'retracted') THEN 'disputed'
      WHEN bool_or(claim.status = 'corroborated') FILTER (WHERE claim.status <> 'retracted') THEN 'corroborated'
      WHEN bool_or(claim.status = 'single_source') FILTER (WHERE claim.status <> 'retracted') THEN 'single_source'
      ELSE 'unverified'
    END AS status
    FROM event_claims claim WHERE claim.event_id = event.id AND claim.created_at <= ` + asOf + `
) confirmation ON true
LEFT JOIN LATERAL (
    SELECT to_jsonb(update_row) AS value
	FROM event_updates update_row WHERE update_row.event_id = event.id
	  AND update_row.observed_at <= ` + asOf + ` AND update_row.created_at <= ` + asOf + `
    ORDER BY update_row.sequence_no DESC LIMIT 1
) latest_update ON true
WHERE ` + strings.Join(conditions, " AND ") + `
), radar_ranked AS (
SELECT radar_base.*,
       ROUND((breadth * 0.7 + freshness * 0.3)::numeric, 2)::double precision AS data_confidence,
       ` + rankingColumn + `::double precision AS ranking_score
FROM radar_base
)
SELECT event_id, version, event_key, title_zh, title_en, summary, lifecycle_status,
       first_seen_at, last_seen_at, trend_score, trend_status, source_count,
       attention, momentum, breadth, freshness, confirmation,
       watch_relevance, watch_final_score, latest_update, data_confidence, ranking_score
FROM radar_ranked`
	statement += ` ORDER BY ranking_score DESC, last_seen_at DESC, event_id DESC LIMIT ` + builder.bind(limit)
	return statement, builder.arguments
}

type radarRowScanner interface {
	Scan(...any) error
}

func scanRadarEvent(row radarRowScanner, sortValue domain.RadarSort) (domain.RadarEvent, error) {
	var item domain.RadarEvent
	var lifecycle, trend, confirmation string
	var sourceCount int64
	var freshness float64
	var watchRelevance, watchFinalScore sql.NullFloat64
	var latestUpdate []byte
	if err := row.Scan(
		&item.EventID, &item.Version, &item.EventKey, &item.TitleZH, &item.TitleEN, &item.Summary, &lifecycle,
		&item.FirstSeenAt, &item.LastSeenAt, &item.TrendScore, &trend, &sourceCount,
		&item.Attention, &item.Momentum, &item.Breadth, &freshness, &confirmation,
		&watchRelevance, &watchFinalScore, &latestUpdate, &item.DataConfidence, &item.RankingScore,
	); err != nil {
		return domain.RadarEvent{}, databaserepository.MapError(err)
	}
	item.LifecycleStatus = domain.LifecycleStatus(lifecycle)
	item.TrendStatus = domain.TrendStatus(trend)
	item.IndependentSourceCount = int(sourceCount)
	item.Confirmation = domain.RadarConfirmation(confirmation)
	switch item.Confirmation {
	case domain.RadarConfirmationDisputed:
		item.ConfirmationScore = radarFloatPointer(20)
	case domain.RadarConfirmationCorroborated:
		item.ConfirmationScore = radarFloatPointer(100)
	case domain.RadarConfirmationSingleSource:
		item.ConfirmationScore = radarFloatPointer(60)
	case domain.RadarConfirmationUnverified:
		item.ConfirmationScore = radarFloatPointer(30)
	}
	if watchRelevance.Valid {
		item.WatchRelevance = radarFloatPointer(watchRelevance.Float64)
	}
	if watchFinalScore.Valid {
		item.WatchFinalScore = radarFloatPointer(watchFinalScore.Float64)
	}
	item.ReasonCodes = []string{string(sortValue)}
	if len(latestUpdate) > 0 {
		update, err := decodeRadarLatestUpdate(latestUpdate)
		if err != nil {
			return domain.RadarEvent{}, err
		}
		item.LatestUpdate = update
	}
	return item, nil
}

type radarLatestUpdateJSON struct {
	ID              int64                  `json:"id"`
	Version         int64                  `json:"version"`
	EventID         int64                  `json:"event_id"`
	SequenceNo      int64                  `json:"sequence_no"`
	Kind            domain.EventUpdateKind `json:"kind"`
	Summary         string                 `json:"summary"`
	ObservedAt      time.Time              `json:"observed_at"`
	ReasonCodes     []string               `json:"reason_codes"`
	BeforeState     json.RawMessage        `json:"before_state"`
	AfterState      json.RawMessage        `json:"after_state"`
	EvidenceSetHash string                 `json:"evidence_set_hash"`
	CreatedAt       time.Time              `json:"created_at"`
}

func decodeRadarLatestUpdate(payload []byte) (*domain.EventUpdate, error) {
	var value radarLatestUpdateJSON
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, databaserepository.MapError(err)
	}
	update := &domain.EventUpdate{
		ID: value.ID, Version: value.Version, EventID: value.EventID, SequenceNo: value.SequenceNo,
		Kind: value.Kind, Summary: value.Summary, ObservedAt: value.ObservedAt,
		ReasonCodes: value.ReasonCodes, EvidenceSetHash: value.EvidenceSetHash, CreatedAt: value.CreatedAt,
	}
	if len(value.BeforeState) > 0 && strings.TrimSpace(string(value.BeforeState)) != "{}" {
		before, err := unmarshalEventUpdateState(value.BeforeState, value.EventID)
		if err != nil {
			return nil, fmt.Errorf("decode latest Radar update before state: %w", err)
		}
		update.BeforeState = before
	}
	if len(value.AfterState) > 0 {
		after, err := unmarshalEventUpdateState(value.AfterState, value.EventID)
		if err != nil {
			return nil, fmt.Errorf("decode latest Radar update after state: %w", err)
		}
		update.AfterState = *after
	}
	return update, nil
}

func encodeRadarCursor(cursor domain.RadarCursor, signingKey []byte) (string, error) {
	payload, err := canonicalRadarCursorPayload(cursor)
	if err != nil {
		return "", err
	}
	signature := signRadarCursor(payload, signingKey)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func decodeRadarCursor(encoded string, signingKey []byte) (domain.RadarCursor, error) {
	parts := strings.Split(encoded, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return domain.RadarCursor{}, fmt.Errorf("invalid signed cursor envelope")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return domain.RadarCursor{}, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return domain.RadarCursor{}, err
	}
	if len(payload) == 0 || len(payload) > radarCursorMaxPayloadSize || len(signature) != sha256.Size {
		return domain.RadarCursor{}, fmt.Errorf("invalid cursor size")
	}
	want := signRadarCursor(payload, signingKey)
	if !hmac.Equal(signature, want) {
		return domain.RadarCursor{}, fmt.Errorf("invalid cursor signature")
	}
	var cursor domain.RadarCursor
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return domain.RadarCursor{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return domain.RadarCursor{}, fmt.Errorf("invalid cursor payload")
	}
	canonical, err := canonicalRadarCursorPayload(cursor)
	if err != nil {
		return domain.RadarCursor{}, err
	}
	if !bytes.Equal(payload, canonical) {
		return domain.RadarCursor{}, fmt.Errorf("non-canonical cursor payload")
	}
	return cursor, nil
}

func canonicalRadarCursorPayload(cursor domain.RadarCursor) ([]byte, error) {
	return json.Marshal(cursor)
}

func signRadarCursor(payload, signingKey []byte) []byte {
	signature := hmac.New(sha256.New, signingKey)
	_, _ = signature.Write(payload)
	return signature.Sum(nil)
}

func radarFloatPointer(value float64) *float64 { return &value }
