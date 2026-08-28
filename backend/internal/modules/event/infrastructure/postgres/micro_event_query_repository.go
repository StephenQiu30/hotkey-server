package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type MicroEventQueryPostgresRepository struct {
	runtime     *database.Runtime
	now         func() time.Time
	cursorCodec *pagination.Codec
}

var _ eventapplication.MicroEventQueryRepository = (*MicroEventQueryPostgresRepository)(nil)
var _ eventapplication.ContentSearchReader = (*MicroEventQueryPostgresRepository)(nil)

func NewMicroEventQueryPostgresRepository(runtime *database.Runtime) (*MicroEventQueryPostgresRepository, error) {
	seed := "event-query:unavailable"
	if runtime != nil && runtime.Pool != nil {
		seed = "event-query:" + runtime.Pool.Config().ConnString()
	}
	return NewMicroEventQueryPostgresRepositoryWithCursorCodec(runtime, pagination.NewTestCodec(seed))
}

func NewMicroEventQueryPostgresRepositoryWithCursorCodec(runtime *database.Runtime, codec *pagination.Codec) (*MicroEventQueryPostgresRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("micro-event query database runtime is required")
	}
	if codec == nil {
		return nil, fmt.Errorf("micro-event query cursor codec is required")
	}
	return &MicroEventQueryPostgresRepository{runtime: runtime, now: func() time.Time { return time.Now().UTC() }, cursorCodec: codec}, nil
}

func (repository *MicroEventQueryPostgresRepository) ListContentSearchReferences(ctx context.Context, contentIDs []int64) ([]eventapplication.ContentSearchReference, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if len(contentIDs) == 0 {
		return []eventapplication.ContentSearchReference{}, nil
	}
	for _, contentID := range contentIDs {
		if contentID <= 0 {
			return nil, fmt.Errorf("%w: positive content ids are required", sharedrepository.ErrInvalidInput)
		}
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT content.id,
       min(current_event.id),
       min(current_event.primary_subject_key || ' · ' || current_event.primary_action_key)
FROM contents AS content
JOIN documents AS document
  ON document.source_connection_id = content.source_connection_id
 AND document.external_work_id = content.external_id
 AND document.document_state = 'active'
JOIN document_versions AS document_version
  ON document_version.id = document.current_document_version_id
JOIN content_family_members AS family_member
  ON family_member.document_version_id = document_version.id
 AND family_member.active
JOIN micro_event_members AS member
  ON member.content_family_id = family_member.family_id
 AND member.active
JOIN micro_events AS membership_event ON membership_event.id = member.micro_event_id
JOIN micro_events AS current_event
  ON current_event.id = COALESCE(membership_event.merged_into_micro_event_id, membership_event.id)
WHERE content.id = ANY($1)
  AND content.content_status = 'active'
  AND content.deleted_at IS NULL
GROUP BY content.id
HAVING count(DISTINCT current_event.id) = 1
ORDER BY content.id ASC`, contentIDs)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	references := make([]eventapplication.ContentSearchReference, 0, len(contentIDs))
	for rows.Next() {
		var reference eventapplication.ContentSearchReference
		if err := rows.Scan(&reference.ContentID, &reference.MicroEventID, &reference.MicroEventTitle); err != nil {
			return nil, databaserepository.MapError(err)
		}
		references = append(references, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return references, nil
}

const microEventProjectionSelectFormat = `
SELECT event.id,event.version,event.event_key,event.status,event.primary_subject_key,event.primary_action_key,
       to_json(event.location_keys),to_json(event.identifier_keys),event.event_started_at,event.event_ended_at,
       event.clustering_profile_version,
       storyline.value,heat.value,relevance.score,evidence_state.value,
       count(DISTINCT member.content_family_id)::integer,
       count(DISTINCT family_member.document_version_id) FILTER (WHERE family_member.active)::integer
FROM micro_events AS event
LEFT JOIN micro_event_members AS member ON member.micro_event_id=event.id AND member.active
LEFT JOIN content_family_members AS family_member ON family_member.family_id=member.content_family_id AND family_member.active
LEFT JOIN LATERAL (
  SELECT jsonb_build_object('id',storyline.id,'version',storyline.version,'storyline_key',storyline.storyline_key,
         'title',storyline.title,'summary',storyline.summary,'status',storyline.status,
         'relation_profile_version',storyline.relation_profile_version) AS value
  FROM storyline_events AS relation JOIN storylines AS storyline ON storyline.id=relation.storyline_id
  WHERE relation.micro_event_id=event.id AND relation.active ORDER BY relation.created_at DESC,relation.id DESC LIMIT 1
) AS storyline ON true
LEFT JOIN LATERAL (
  SELECT jsonb_build_object('id',snapshot.id,'micro_event_id',snapshot.micro_event_id,
         'micro_event_version',snapshot.micro_event_version,'heat_profile_id',snapshot.heat_profile_id,
	     'heat_profile_version',profile.profile_version,
         'window_started_at',snapshot.window_started_at,'window_ended_at',snapshot.window_ended_at,
         'independent_lineage_roots',snapshot.independent_lineage_root_count,'velocity',snapshot.velocity,
         'acceleration',snapshot.acceleration,'coverage',snapshot.coverage,
         'normalized_engagement',snapshot.normalized_engagement,'recency',snapshot.recency,
         'available_weight',snapshot.available_weight,'heat_score',snapshot.heat_score,
	     'reason_codes',snapshot.reason_codes) AS value,
	 snapshot.heat_score::float8 AS score,snapshot.window_ended_at
  FROM micro_event_heat_snapshots AS snapshot JOIN event_heat_profiles AS profile ON profile.id=snapshot.heat_profile_id
	WHERE snapshot.micro_event_id=event.id AND profile.status='active'
	  AND snapshot.window_started_at=snapshot.window_ended_at-interval '1 hour'
	  AND %s
  ORDER BY snapshot.window_ended_at DESC,snapshot.window_started_at DESC,snapshot.id DESC LIMIT 1
) AS heat ON true
LEFT JOIN LATERAL (
  SELECT max(decision.relevance_probability)::float8 AS score
  FROM micro_event_members AS relevant_member
  JOIN content_family_members AS relevant_family_member
    ON relevant_family_member.family_id=relevant_member.content_family_id AND relevant_family_member.active
  JOIN document_match_decisions AS decision
    ON decision.document_version_id=relevant_family_member.document_version_id
  JOIN relevance_decision_profiles AS relevance_profile
    ON relevance_profile.id=decision.relevance_profile_id AND relevance_profile.status='active'
  LEFT JOIN LATERAL (
    SELECT decision_override.decision
    FROM document_match_overrides AS decision_override
    WHERE decision_override.match_decision_id=decision.id AND %s
    ORDER BY decision_override.sequence_no DESC,decision_override.id DESC LIMIT 1
  ) AS effective_override ON true
  WHERE relevant_member.micro_event_id=event.id AND relevant_member.active
    AND decision.relevance_probability IS NOT NULL
    AND COALESCE(effective_override.decision,decision.decision)='accepted'
    AND %s
    AND %s
) AS relevance ON true
LEFT JOIN LATERAL (
  SELECT jsonb_build_object('id',snapshot.id,'version',snapshot.version,'micro_event_id',snapshot.micro_event_id,
         'event_version',snapshot.micro_event_version,'profile_id',snapshot.evidence_state_profile_id,
         'algorithm_version',snapshot.algorithm_version,'evidence_set_hash',snapshot.evidence_set_hash,
         'state',snapshot.evidence_state,'independent_origin_count',snapshot.independent_origin_count,
         'reason_codes',snapshot.reason_codes,'calculated_at',snapshot.calculated_at) AS value
  FROM evidence_state_snapshots AS snapshot JOIN evidence_state_profiles AS profile ON profile.id=snapshot.evidence_state_profile_id
  WHERE snapshot.micro_event_id=event.id AND profile.status='active' AND %s
  ORDER BY snapshot.calculated_at DESC,snapshot.id DESC LIMIT 1
) AS evidence_state ON true
`

func microEventProjectionSelect(heatAsOfPredicate, overrideAsOfPredicate, relevanceAsOfPredicate, relevanceMonitorPredicate, evidenceAsOfPredicate string) string {
	return fmt.Sprintf(microEventProjectionSelectFormat, heatAsOfPredicate, overrideAsOfPredicate,
		relevanceAsOfPredicate, relevanceMonitorPredicate, evidenceAsOfPredicate)
}

type microEventListCursor struct {
	Version        int       `json:"v"`
	Sort           string    `json:"s"`
	Filter         string    `json:"f"`
	AsOf           time.Time `json:"a"`
	HasHeat        bool      `json:"hh"`
	HeatScore      float64   `json:"h,omitempty"`
	HeatWindowEnd  time.Time `json:"hw,omitempty"`
	HasRelevance   bool      `json:"hr"`
	RelevanceScore float64   `json:"r,omitempty"`
	EventStartedAt time.Time `json:"e"`
	ID             int64     `json:"id"`
}

type microEventEvidenceCursor struct {
	Version      int       `json:"v"`
	MicroEventID int64     `json:"event_id"`
	AsOf         time.Time `json:"as_of"`
	ID           int64     `json:"id"`
}

func microEventFilterFingerprint(query eventapplication.MicroEventListQuery) string {
	statuses := append([]string(nil), query.Statuses...)
	sourceTypes := append([]string(nil), query.SourceTypes...)
	evidenceStates := append([]string(nil), query.EvidenceStates...)
	sort.Strings(statuses)
	sort.Strings(sourceTypes)
	sort.Strings(evidenceStates)
	type fingerprint struct {
		Statuses       []string `json:"statuses"`
		MonitorID      int64    `json:"monitor_id"`
		SourceTypes    []string `json:"source_types"`
		EvidenceStates []string `json:"evidence_states"`
		StartedFrom    string   `json:"started_from"`
		StartedTo      string   `json:"started_to"`
	}
	value := fingerprint{Statuses: statuses, MonitorID: query.MonitorID, SourceTypes: sourceTypes, EvidenceStates: evidenceStates}
	if query.StartedFrom != nil {
		value.StartedFrom = query.StartedFrom.UTC().Format(time.RFC3339Nano)
	}
	if query.StartedTo != nil {
		value.StartedTo = query.StartedTo.UTC().Format(time.RFC3339Nano)
	}
	payload, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(payload))
}

func encodeMicroEventListCursor(codec *pagination.Codec, cursor microEventListCursor) (string, error) {
	cursor.Version = 3
	return codec.Seal("micro_event_list", cursor)
}

func decodeMicroEventListCursor(codec *pagination.Codec, value, expectedSort, expectedFilter string) (microEventListCursor, error) {
	var cursor microEventListCursor
	if err := codec.Open(value, "micro_event_list", &cursor); err != nil {
		return microEventListCursor{}, eventapplication.ErrInvalidMicroEventQuery
	}
	validHeat := cursor.Sort == "heat" && (cursor.HasHeat && !cursor.HeatWindowEnd.IsZero() && cursor.HeatScore >= 0 && cursor.HeatScore <= 100 ||
		!cursor.HasHeat && cursor.HeatWindowEnd.IsZero() && cursor.HeatScore == 0) ||
		cursor.Sort != "heat" && !cursor.HasHeat && cursor.HeatWindowEnd.IsZero() && cursor.HeatScore == 0
	validRelevance := cursor.Sort == "relevance" && (cursor.HasRelevance && cursor.RelevanceScore >= 0 && cursor.RelevanceScore <= 1 ||
		!cursor.HasRelevance && cursor.RelevanceScore == 0) ||
		cursor.Sort != "relevance" && !cursor.HasRelevance && cursor.RelevanceScore == 0
	if cursor.Version != 3 || cursor.Sort != expectedSort ||
		cursor.Filter != expectedFilter || cursor.AsOf.IsZero() || cursor.EventStartedAt.IsZero() || cursor.ID <= 0 ||
		!validHeat || !validRelevance {
		return microEventListCursor{}, eventapplication.ErrInvalidMicroEventQuery
	}
	return cursor, nil
}

func encodeMicroEventEvidenceCursor(codec *pagination.Codec, cursor microEventEvidenceCursor) (string, error) {
	cursor.Version = 1
	return codec.Seal("micro_event_evidence_list", cursor)
}

func decodeMicroEventEvidenceCursor(codec *pagination.Codec, value string, expectedMicroEventID int64) (microEventEvidenceCursor, error) {
	var cursor microEventEvidenceCursor
	if err := codec.Open(value, "micro_event_evidence_list", &cursor); err != nil {
		return microEventEvidenceCursor{}, fmt.Errorf("%w: micro-event evidence cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	if cursor.Version != 1 || cursor.MicroEventID != expectedMicroEventID || cursor.AsOf.IsZero() || cursor.ID <= 0 {
		return microEventEvidenceCursor{}, fmt.Errorf("%w: micro-event evidence cursor shape", sharedrepository.ErrInvalidInput)
	}
	return cursor, nil
}

type microEventProjectionRecord struct {
	id, version                        int64
	eventKey, status                   string
	subject, action, profile           string
	locationsJSON, identifiersJSON     []byte
	startedAt                          time.Time
	endedAt                            sql.NullTime
	storylineJSON, heatJSON, stateJSON []byte
	relevanceScore                     sql.NullFloat64
	familyCount, documentCount         int
}

func (repository *MicroEventQueryPostgresRepository) ListMicroEvents(ctx context.Context, query eventapplication.MicroEventListQuery) (eventapplication.MicroEventPageDTO, error) {
	if repository == nil || repository.runtime == nil || repository.now == nil {
		return eventapplication.MicroEventPageDTO{}, eventapplication.ErrInvalidMicroEventQuery
	}
	statuses := query.Statuses
	if len(statuses) == 0 {
		statuses = []string{"active", "review_pending", "closed", "merged"}
	}
	query.Statuses = statuses
	fingerprint := microEventFilterFingerprint(query)
	cursor := microEventListCursor{Sort: query.Sort, Filter: fingerprint, AsOf: repository.now().UTC()}
	if query.Cursor != "" {
		decoded, err := decodeMicroEventListCursor(repository.cursorCodec, query.Cursor, query.Sort, fingerprint)
		if err != nil {
			return eventapplication.MicroEventPageDTO{}, err
		}
		cursor = decoded
	}
	arguments := []any{statuses, cursor.AsOf, query.Limit + 1}
	addArgument := func(value any) string {
		arguments = append(arguments, value)
		return fmt.Sprintf("$%d", len(arguments))
	}
	relevanceMonitorPredicate := "true"
	if query.MonitorID > 0 {
		relevanceMonitorPredicate = "decision.monitor_id=" + addArgument(query.MonitorID)
	}
	statement := microEventProjectionSelect("snapshot.calculated_at<=$2", "decision_override.created_at<=$2",
		"decision.decided_at<=$2", relevanceMonitorPredicate, "snapshot.created_at<=$2") + `
WHERE event.status=ANY($1) AND event.created_at<=$2`
	if query.MonitorID > 0 {
		statement += ` AND relevance.score IS NOT NULL`
	}
	if query.StartedFrom != nil {
		statement += ` AND event.event_started_at>=` + addArgument(query.StartedFrom.UTC())
	}
	if query.StartedTo != nil {
		statement += ` AND event.event_started_at<=` + addArgument(query.StartedTo.UTC())
	}
	if len(query.SourceTypes) > 0 {
		placeholder := addArgument(query.SourceTypes)
		statement += ` AND EXISTS (
  SELECT 1
  FROM micro_event_members AS filtered_member
  JOIN content_family_members AS filtered_family_member
    ON filtered_family_member.family_id=filtered_member.content_family_id AND filtered_family_member.active
  JOIN document_versions AS filtered_version ON filtered_version.id=filtered_family_member.document_version_id
  JOIN source_observations AS filtered_observation ON filtered_observation.id=filtered_version.source_observation_id
  JOIN source_connections AS filtered_source ON filtered_source.id=filtered_observation.source_connection_id
  WHERE filtered_member.micro_event_id=event.id AND filtered_member.active
    AND filtered_source.source_type=ANY(` + placeholder + `)
)`
	}
	if len(query.EvidenceStates) > 0 {
		statement += ` AND (evidence_state.value->>'state')=ANY(` + addArgument(query.EvidenceStates) + `)`
	}
	groupBy := `
GROUP BY event.id,storyline.value,heat.value,heat.score,heat.window_ended_at,relevance.score,evidence_state.value`
	if query.Sort == "latest" {
		if query.Cursor != "" {
			startedAt := addArgument(cursor.EventStartedAt)
			id := addArgument(cursor.ID)
			statement += ` AND (event.event_started_at<` + startedAt + ` OR (event.event_started_at=` + startedAt + ` AND event.id<` + id + `))`
		}
		statement += groupBy + `
ORDER BY event.event_started_at DESC,event.id DESC LIMIT $3`
	} else if query.Sort == "heat" {
		if query.Cursor != "" && cursor.HasHeat {
			heatScore := addArgument(cursor.HeatScore)
			heatWindow := addArgument(cursor.HeatWindowEnd)
			startedAt := addArgument(cursor.EventStartedAt)
			id := addArgument(cursor.ID)
			statement += ` AND (heat.score IS NULL OR heat.score<` + heatScore + `
  OR (heat.score=` + heatScore + ` AND heat.window_ended_at<` + heatWindow + `)
  OR (heat.score=` + heatScore + ` AND heat.window_ended_at=` + heatWindow + ` AND event.event_started_at<` + startedAt + `)
  OR (heat.score=` + heatScore + ` AND heat.window_ended_at=` + heatWindow + ` AND event.event_started_at=` + startedAt + ` AND event.id<` + id + `))`
		} else if query.Cursor != "" {
			startedAt := addArgument(cursor.EventStartedAt)
			id := addArgument(cursor.ID)
			statement += ` AND heat.score IS NULL
			  AND (event.event_started_at<` + startedAt + ` OR (event.event_started_at=` + startedAt + ` AND event.id<` + id + `))`
		}
		statement += groupBy + `
ORDER BY heat.score DESC NULLS LAST,heat.window_ended_at DESC NULLS LAST,event.event_started_at DESC,event.id DESC LIMIT $3`
	} else {
		if query.Cursor != "" && cursor.HasRelevance {
			relevanceScore := addArgument(cursor.RelevanceScore)
			startedAt := addArgument(cursor.EventStartedAt)
			id := addArgument(cursor.ID)
			statement += ` AND (relevance.score IS NULL OR relevance.score<` + relevanceScore + `
  OR (relevance.score=` + relevanceScore + ` AND event.event_started_at<` + startedAt + `)
  OR (relevance.score=` + relevanceScore + ` AND event.event_started_at=` + startedAt + ` AND event.id<` + id + `))`
		} else if query.Cursor != "" {
			startedAt := addArgument(cursor.EventStartedAt)
			id := addArgument(cursor.ID)
			statement += ` AND relevance.score IS NULL
  AND (event.event_started_at<` + startedAt + ` OR (event.event_started_at=` + startedAt + ` AND event.id<` + id + `))`
		}
		statement += groupBy + `
ORDER BY relevance.score DESC NULLS LAST,event.event_started_at DESC,event.id DESC LIMIT $3`
	}
	rows, err := repository.queryExecutor(ctx).QueryContext(ctx, statement, arguments...)
	if err != nil {
		return eventapplication.MicroEventPageDTO{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	items := []eventapplication.MicroEventProjectionDTO{}
	for rows.Next() {
		record, err := scanMicroEventProjection(rows)
		if err != nil {
			return eventapplication.MicroEventPageDTO{}, err
		}
		value, err := record.dto()
		if err != nil {
			return eventapplication.MicroEventPageDTO{}, err
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		return eventapplication.MicroEventPageDTO{}, databaserepository.MapError(err)
	}
	page := eventapplication.MicroEventPageDTO{Items: items}
	if len(page.Items) > query.Limit {
		page.Items = page.Items[:query.Limit]
		last := page.Items[len(page.Items)-1]
		next := microEventListCursor{Sort: query.Sort, Filter: fingerprint, AsOf: cursor.AsOf,
			EventStartedAt: last.EventStartedAt, ID: last.ID}
		if query.Sort == "heat" && last.LatestHeat != nil {
			next.HasHeat, next.HeatScore, next.HeatWindowEnd = true, last.LatestHeat.HeatScore, last.LatestHeat.WindowEndedAt
		}
		if query.Sort == "relevance" && last.RelevanceScore != nil {
			next.HasRelevance, next.RelevanceScore = true, *last.RelevanceScore
		}
		page.NextCursor, err = encodeMicroEventListCursor(repository.cursorCodec, next)
		if err != nil {
			return eventapplication.MicroEventPageDTO{}, fmt.Errorf("encode micro-event cursor: %w", err)
		}
	}
	return page, nil
}

func (repository *MicroEventQueryPostgresRepository) GetMicroEvent(ctx context.Context, id int64) (eventapplication.MicroEventProjectionDTO, error) {
	if repository == nil || repository.runtime == nil || id <= 0 {
		return eventapplication.MicroEventProjectionDTO{}, eventapplication.ErrInvalidMicroEventQuery
	}
	rows, err := repository.queryExecutor(ctx).QueryContext(ctx, microEventProjectionSelect("true", "true", "true", "true", "true")+`
WHERE event.id=$1 GROUP BY event.id,storyline.value,heat.value,heat.score,heat.window_ended_at,relevance.score,evidence_state.value`, id)
	if err != nil {
		return eventapplication.MicroEventProjectionDTO{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	if !rows.Next() {
		return eventapplication.MicroEventProjectionDTO{}, sharedrepository.ErrNotFound
	}
	record, err := scanMicroEventProjection(rows)
	if err != nil {
		return eventapplication.MicroEventProjectionDTO{}, err
	}
	if err := rows.Close(); err != nil {
		return eventapplication.MicroEventProjectionDTO{}, databaserepository.MapError(err)
	}
	result, err := record.dto()
	if err != nil {
		return eventapplication.MicroEventProjectionDTO{}, err
	}
	members, err := repository.queryExecutor(ctx).QueryContext(ctx, `SELECT id,version,content_family_id,membership_decision_id,clustering_profile_version
FROM micro_event_members WHERE micro_event_id=$1 AND active ORDER BY content_family_id,id`, id)
	if err != nil {
		return eventapplication.MicroEventProjectionDTO{}, databaserepository.MapError(err)
	}
	defer members.Close()
	result.Members = make([]eventapplication.MicroEventMemberProjectionDTO, 0, result.ContentFamilyCount)
	for members.Next() {
		var member eventapplication.MicroEventMemberProjectionDTO
		if err := members.Scan(&member.ID, &member.Version, &member.ContentFamilyID, &member.MembershipDecisionID,
			&member.ClusteringProfileVersion); err != nil {
			return eventapplication.MicroEventProjectionDTO{}, databaserepository.MapError(err)
		}
		result.Members = append(result.Members, member)
	}
	if err := members.Err(); err != nil {
		return eventapplication.MicroEventProjectionDTO{}, databaserepository.MapError(err)
	}
	return result, nil
}

func (repository *MicroEventQueryPostgresRepository) GetMicroEventSummary(ctx context.Context, eventID, eventVersion int64) (*eventapplication.EvidenceSummaryDTO, error) {
	if repository == nil || repository.runtime == nil || eventID <= 0 || eventVersion <= 0 {
		return nil, eventapplication.ErrInvalidMicroEventQuery
	}
	executor := repository.queryExecutor(ctx)
	var summary eventapplication.EvidenceSummaryDTO
	if err := executor.QueryRowContext(ctx, `
SELECT id,version,micro_event_id,micro_event_version,summary_profile_version,created_at
FROM micro_event_summaries
WHERE micro_event_id=$1 AND micro_event_version=$2
ORDER BY created_at DESC,id DESC LIMIT 1`, eventID, eventVersion).Scan(&summary.ID, &summary.Version,
		&summary.MicroEventID, &summary.EventVersion, &summary.SummaryProfileVersion, &summary.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, eventapplication.ErrEvidenceSummaryUnavailable
		}
		return nil, databaserepository.MapError(err)
	}
	rows, err := executor.QueryContext(ctx, `
SELECT sentence.id,sentence.version,sentence.micro_event_summary_id,sentence.ordinal,sentence.sentence,
       sentence.editorial_note,sentence.decision_origin,sentence.model_run_id,sentence.actor_user_id,sentence.created_at,
       COALESCE(jsonb_agg(citation.claim_evidence_version_id ORDER BY citation.ordinal)
         FILTER (WHERE citation.id IS NOT NULL),'[]'::jsonb)
FROM micro_event_summary_sentences AS sentence
LEFT JOIN micro_event_summary_sentence_evidences AS citation ON citation.summary_sentence_id=sentence.id
WHERE sentence.micro_event_summary_id=$1
GROUP BY sentence.id ORDER BY sentence.ordinal`, summary.ID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var sentence eventapplication.EvidenceSummarySentenceDTO
		var modelRunID, actorUserID sql.NullInt64
		var evidenceJSON []byte
		if err := rows.Scan(&sentence.ID, &sentence.Version, &sentence.SummaryID, &sentence.Ordinal, &sentence.Text,
			&sentence.EditorialNote, &sentence.DecisionOrigin, &modelRunID, &actorUserID, &sentence.CreatedAt,
			&evidenceJSON); err != nil {
			return nil, databaserepository.MapError(err)
		}
		if err := json.Unmarshal(evidenceJSON, &sentence.ClaimEvidenceVersionIDs); err != nil {
			return nil, fmt.Errorf("decode evidence summary citations: %w", err)
		}
		if modelRunID.Valid {
			value := modelRunID.Int64
			sentence.ModelRunID = &value
		}
		if actorUserID.Valid {
			value := actorUserID.Int64
			sentence.ActorUserID = &value
		}
		sentence.CreatedAt = sentence.CreatedAt.UTC()
		summary.Sentences = append(summary.Sentences, sentence)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	summary.CreatedAt = summary.CreatedAt.UTC()
	return &summary, nil
}

func (repository *MicroEventQueryPostgresRepository) ListMicroEventEvidence(ctx context.Context, query eventapplication.MicroEventEvidenceQuery) (eventapplication.MicroEventEvidencePageDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || repository.cursorCodec == nil || repository.now == nil ||
		query.MicroEventID <= 0 || query.Limit < 1 || query.Limit > 100 {
		return eventapplication.MicroEventEvidencePageDTO{}, eventapplication.ErrInvalidMicroEventQuery
	}
	cursor := microEventEvidenceCursor{MicroEventID: query.MicroEventID, AsOf: repository.now().UTC()}
	if query.Cursor != "" {
		decoded, err := decodeMicroEventEvidenceCursor(repository.cursorCodec, query.Cursor, query.MicroEventID)
		if err != nil {
			return eventapplication.MicroEventEvidencePageDTO{}, err
		}
		cursor = decoded
	}
	rows, err := repository.queryExecutor(ctx).QueryContext(ctx, `
WITH projection AS (
 SELECT evidence.*,claim.version AS claim_version,claim.micro_event_id,claim.subject,claim.predicate,claim.object,
        selector.exact_quote,selector.prefix,selector.suffix,selector.utf8_byte_start,selector.utf8_byte_end,
		selector.markdown_anchor,lineage.lineage_decision_id,lineage.member_version,
        selector.retention_until>$3
        AND current_rights_action_allowed(selector.quote_rights_decision_id,selector.source_connection_id,
            'document_version',evidence.document_version_id::text,evidence.plaintext_sha256,'quote',$3)
        AND current_rights_action_allowed(selector.retain_rights_decision_id,selector.source_connection_id,
            'document_version',evidence.document_version_id::text,evidence.plaintext_sha256,'retain',$3) AS citable
 FROM claim_evidence_versions AS evidence
 JOIN claims AS claim ON claim.id=evidence.claim_id
 JOIN document_text_quote_selectors AS selector ON selector.id=evidence.text_quote_selector_id
 LEFT JOIN LATERAL (
  SELECT min(member.lineage_decision_id) AS lineage_decision_id,min(member.version) AS member_version
  FROM content_family_members AS member
  WHERE member.document_version_id=evidence.document_version_id AND member.active
  HAVING count(*)=1
 ) AS lineage ON true
 WHERE claim.micro_event_id=$1 AND evidence.id>$2 AND evidence.created_at<=$3
)
SELECT id,version,claim_id,claim_version,document_version_id,text_quote_selector_id,content_family_id,lineage_root_document_version_id,
       lineage_decision_id,member_version,subject,predicate,object,relation,
       CASE WHEN citable THEN 'ready' WHEN retention_until<=$3 THEN 'retention_expired' ELSE 'rights_unavailable' END,
       CASE WHEN citable THEN exact_quote END,CASE WHEN citable THEN prefix END,CASE WHEN citable THEN suffix END,
       CASE WHEN citable THEN utf8_byte_start END,CASE WHEN citable THEN utf8_byte_end END,
       CASE WHEN citable THEN quote_sha256 END,CASE WHEN citable THEN plaintext_sha256 END,
       CASE WHEN citable THEN selector_version END,CASE WHEN citable THEN markdown_anchor END,
       source_record_url_snapshot,canonical_url_snapshot,publisher_name_snapshot,content_origin_name_snapshot,
       published_at_snapshot,captured_at_snapshot,extraction_schema_version,decision_origin,created_at
FROM projection ORDER BY id LIMIT $4`, query.MicroEventID, cursor.ID, cursor.AsOf, query.Limit+1)
	if err != nil {
		return eventapplication.MicroEventEvidencePageDTO{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	items := []eventapplication.ClaimEvidenceProjectionDTO{}
	for rows.Next() {
		var item eventapplication.ClaimEvidenceProjectionDTO
		var exact, prefix, suffix, quoteSHA, plaintextSHA, selectorVersion, anchor sql.NullString
		var start, end sql.NullInt64
		var lineageDecisionID, memberVersion sql.NullInt64
		var sourceURL, canonicalURL, publisher, contentOrigin sql.NullString
		var published sql.NullTime
		if err := rows.Scan(&item.ID, &item.Version, &item.ClaimID, &item.ClaimVersion, &item.DocumentVersionID, &item.TextQuoteSelectorID,
			&item.ContentFamilyID, &item.LineageRootID, &lineageDecisionID, &memberVersion,
			&item.ClaimSubject, &item.ClaimPredicate, &item.ClaimObject,
			&item.Relation, &item.Availability, &exact, &prefix, &suffix, &start, &end, &quoteSHA, &plaintextSHA,
			&selectorVersion, &anchor, &sourceURL, &canonicalURL, &publisher, &contentOrigin, &published,
			&item.CapturedAt, &item.ExtractionSchemaVersion, &item.DecisionOrigin, &item.CreatedAt); err != nil {
			return eventapplication.MicroEventEvidencePageDTO{}, databaserepository.MapError(err)
		}
		item.ExactQuote, item.Prefix, item.Suffix = nullableClaimEvidenceString(exact), nullableClaimEvidenceString(prefix), nullableClaimEvidenceString(suffix)
		item.LineageDecisionID, item.ContentFamilyMemberVersion = nullableClaimEvidenceInt64(lineageDecisionID), nullableClaimEvidenceInt64(memberVersion)
		item.UTF8ByteStart, item.UTF8ByteEnd = nullableClaimEvidenceInt64(start), nullableClaimEvidenceInt64(end)
		item.QuoteSHA256, item.PlaintextSHA256 = nullableClaimEvidenceString(quoteSHA), nullableClaimEvidenceString(plaintextSHA)
		item.SelectorVersion, item.MarkdownAnchor = nullableClaimEvidenceString(selectorVersion), nullableClaimEvidenceString(anchor)
		item.SourceRecordURL, item.CanonicalURL = nullableClaimEvidenceString(sourceURL), nullableClaimEvidenceString(canonicalURL)
		item.PublisherName, item.ContentOriginName = nullableClaimEvidenceString(publisher), nullableClaimEvidenceString(contentOrigin)
		item.PublishedAt = nullableClaimEvidenceTime(published)
		item.CapturedAt, item.CreatedAt = item.CapturedAt.UTC(), item.CreatedAt.UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return eventapplication.MicroEventEvidencePageDTO{}, databaserepository.MapError(err)
	}
	page := eventapplication.MicroEventEvidencePageDTO{Items: items}
	if len(page.Items) > query.Limit {
		page.Items = page.Items[:query.Limit]
		page.NextCursor, err = encodeMicroEventEvidenceCursor(repository.cursorCodec, microEventEvidenceCursor{
			MicroEventID: query.MicroEventID, AsOf: cursor.AsOf, ID: page.Items[len(page.Items)-1].ID,
		})
		if err != nil {
			return eventapplication.MicroEventEvidencePageDTO{}, fmt.Errorf("encode micro-event evidence cursor: %w", err)
		}
	}
	return page, nil
}

type microEventRows interface {
	Next() bool
	Scan(...any) error
}

func scanMicroEventProjection(rows microEventRows) (microEventProjectionRecord, error) {
	var record microEventProjectionRecord
	if err := rows.Scan(&record.id, &record.version, &record.eventKey, &record.status, &record.subject, &record.action,
		&record.locationsJSON, &record.identifiersJSON, &record.startedAt, &record.endedAt, &record.profile,
		&record.storylineJSON, &record.heatJSON, &record.relevanceScore, &record.stateJSON, &record.familyCount, &record.documentCount); err != nil {
		return microEventProjectionRecord{}, databaserepository.MapError(err)
	}
	return record, nil
}

type microEventProjectionStoryline struct {
	ID                     int64  `json:"id"`
	Version                int64  `json:"version"`
	StorylineKey           string `json:"storyline_key"`
	Title                  string `json:"title"`
	Summary                string `json:"summary"`
	Status                 string `json:"status"`
	RelationProfileVersion string `json:"relation_profile_version"`
}
type microEventProjectionHeat struct {
	ID                      int64     `json:"id"`
	MicroEventID            int64     `json:"micro_event_id"`
	MicroEventVersion       int64     `json:"micro_event_version"`
	HeatProfileID           int64     `json:"heat_profile_id"`
	HeatProfileVersion      string    `json:"heat_profile_version"`
	WindowStartedAt         time.Time `json:"window_started_at"`
	WindowEndedAt           time.Time `json:"window_ended_at"`
	IndependentLineageRoots int       `json:"independent_lineage_roots"`
	Velocity                float64   `json:"velocity"`
	Acceleration            float64   `json:"acceleration"`
	Coverage                float64   `json:"coverage"`
	NormalizedEngagement    *float64  `json:"normalized_engagement"`
	Recency                 float64   `json:"recency"`
	AvailableWeight         float64   `json:"available_weight"`
	HeatScore               float64   `json:"heat_score"`
	ReasonCodes             []string  `json:"reason_codes"`
}
type microEventProjectionState struct {
	ID                     int64     `json:"id"`
	Version                int64     `json:"version"`
	MicroEventID           int64     `json:"micro_event_id"`
	EventVersion           int64     `json:"event_version"`
	ProfileID              int64     `json:"profile_id"`
	AlgorithmVersion       string    `json:"algorithm_version"`
	EvidenceSetHash        string    `json:"evidence_set_hash"`
	State                  string    `json:"state"`
	IndependentOriginCount int       `json:"independent_origin_count"`
	ReasonCodes            []string  `json:"reason_codes"`
	CalculatedAt           time.Time `json:"calculated_at"`
}

func (record microEventProjectionRecord) dto() (eventapplication.MicroEventProjectionDTO, error) {
	locations, identifiers := []string{}, []string{}
	if err := json.Unmarshal(record.locationsJSON, &locations); err != nil {
		return eventapplication.MicroEventProjectionDTO{}, err
	}
	if err := json.Unmarshal(record.identifiersJSON, &identifiers); err != nil {
		return eventapplication.MicroEventProjectionDTO{}, err
	}
	value := eventapplication.MicroEventProjectionDTO{ID: record.id, Version: record.version, EventKey: strings.TrimSpace(record.eventKey),
		Status: record.status, PrimarySubjectKey: record.subject, PrimaryActionKey: record.action, LocationKeys: locations,
		IdentifierKeys: identifiers, EventStartedAt: record.startedAt.UTC(), ClusteringProfileVersion: record.profile,
		ContentFamilyCount: record.familyCount, DocumentCount: record.documentCount}
	if record.relevanceScore.Valid {
		relevance := record.relevanceScore.Float64
		value.RelevanceScore = &relevance
	}
	if record.endedAt.Valid {
		ended := record.endedAt.Time.UTC()
		value.EventEndedAt = &ended
	}
	if len(record.storylineJSON) > 0 && string(record.storylineJSON) != "null" {
		var projected microEventProjectionStoryline
		if err := json.Unmarshal(record.storylineJSON, &projected); err != nil {
			return value, err
		}
		value.Storyline = &eventapplication.StorylineDTO{ID: projected.ID, Version: projected.Version, StorylineKey: projected.StorylineKey,
			Title: projected.Title, Summary: projected.Summary, Status: projected.Status, RelationProfileVersion: projected.RelationProfileVersion}
	}
	if len(record.heatJSON) > 0 && string(record.heatJSON) != "null" {
		var projected microEventProjectionHeat
		if err := json.Unmarshal(record.heatJSON, &projected); err != nil {
			return value, err
		}
		value.LatestHeat = &eventapplication.EventHeatSnapshotDTO{ID: projected.ID, MicroEventID: projected.MicroEventID,
			MicroEventVersion: projected.MicroEventVersion, HeatProfileID: projected.HeatProfileID,
			HeatProfileVersion: projected.HeatProfileVersion,
			WindowStartedAt:    projected.WindowStartedAt.UTC(), WindowEndedAt: projected.WindowEndedAt.UTC(),
			IndependentLineageRoots: projected.IndependentLineageRoots, Velocity: projected.Velocity,
			Acceleration: projected.Acceleration, Coverage: projected.Coverage, NormalizedEngagement: projected.NormalizedEngagement,
			Recency: projected.Recency, AvailableWeight: projected.AvailableWeight, HeatScore: projected.HeatScore,
			WarmingUp:   slices.Contains(projected.ReasonCodes, "warming_up"),
			ReasonCodes: projected.ReasonCodes}
	}
	if len(record.stateJSON) > 0 && string(record.stateJSON) != "null" {
		var projected microEventProjectionState
		if err := json.Unmarshal(record.stateJSON, &projected); err != nil {
			return value, err
		}
		value.LatestEvidenceState = &eventapplication.EvidenceStateSnapshotDTO{ID: projected.ID, Version: projected.Version,
			MicroEventID: projected.MicroEventID, EventVersion: projected.EventVersion, ProfileID: projected.ProfileID,
			AlgorithmVersion: projected.AlgorithmVersion, EvidenceSetHash: strings.TrimSpace(projected.EvidenceSetHash),
			State: projected.State, IndependentOriginCount: projected.IndependentOriginCount,
			ReasonCodes: projected.ReasonCodes, CalculatedAt: projected.CalculatedAt.UTC()}
	}
	return value, nil
}

type microEventProjectionExecutor interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (repository *MicroEventQueryPostgresRepository) queryExecutor(ctx context.Context) microEventProjectionExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}
