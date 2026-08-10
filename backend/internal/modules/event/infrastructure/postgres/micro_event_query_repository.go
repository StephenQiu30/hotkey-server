package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type MicroEventQueryPostgresRepository struct{ runtime *database.Runtime }

var _ eventapplication.MicroEventQueryRepository = (*MicroEventQueryPostgresRepository)(nil)

func NewMicroEventQueryPostgresRepository(runtime *database.Runtime) (*MicroEventQueryPostgresRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("micro-event query database runtime is required")
	}
	return &MicroEventQueryPostgresRepository{runtime: runtime}, nil
}

const microEventProjectionSelect = `
SELECT event.id,event.version,event.event_key,event.status,event.primary_subject_key,event.primary_action_key,
       to_json(event.location_keys),to_json(event.identifier_keys),event.event_started_at,event.event_ended_at,
       event.clustering_profile_version,
       storyline.value,heat.value,evidence_state.value,
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
         'window_started_at',snapshot.window_started_at,'window_ended_at',snapshot.window_ended_at,
         'independent_lineage_roots',snapshot.independent_lineage_root_count,'velocity',snapshot.velocity,
         'acceleration',snapshot.acceleration,'coverage',snapshot.coverage,
         'normalized_engagement',snapshot.normalized_engagement,'recency',snapshot.recency,
         'available_weight',snapshot.available_weight,'heat_score',snapshot.heat_score,
         'reason_codes',snapshot.reason_codes) AS value
  FROM micro_event_heat_snapshots AS snapshot JOIN event_heat_profiles AS profile ON profile.id=snapshot.heat_profile_id
  WHERE snapshot.micro_event_id=event.id AND profile.status='active'
  ORDER BY snapshot.window_ended_at DESC,snapshot.window_started_at DESC,snapshot.id DESC LIMIT 1
) AS heat ON true
LEFT JOIN LATERAL (
  SELECT jsonb_build_object('id',snapshot.id,'version',snapshot.version,'micro_event_id',snapshot.micro_event_id,
         'event_version',snapshot.micro_event_version,'profile_id',snapshot.evidence_state_profile_id,
         'algorithm_version',snapshot.algorithm_version,'evidence_set_hash',snapshot.evidence_set_hash,
         'state',snapshot.evidence_state,'independent_origin_count',snapshot.independent_origin_count,
         'reason_codes',snapshot.reason_codes,'calculated_at',snapshot.calculated_at) AS value
  FROM evidence_state_snapshots AS snapshot JOIN evidence_state_profiles AS profile ON profile.id=snapshot.evidence_state_profile_id
  WHERE snapshot.micro_event_id=event.id AND profile.status='active'
  ORDER BY snapshot.calculated_at DESC,snapshot.id DESC LIMIT 1
) AS evidence_state ON true
`

type microEventProjectionRecord struct {
	id, version                        int64
	eventKey, status                   string
	subject, action, profile           string
	locationsJSON, identifiersJSON     []byte
	startedAt                          time.Time
	endedAt                            sql.NullTime
	storylineJSON, heatJSON, stateJSON []byte
	familyCount, documentCount         int
}

func (repository *MicroEventQueryPostgresRepository) ListMicroEvents(ctx context.Context, query eventapplication.MicroEventListQuery) (eventapplication.MicroEventPageDTO, error) {
	statuses := query.Statuses
	if len(statuses) == 0 {
		statuses = []string{"active", "review_pending", "closed", "merged"}
	}
	rows, err := repository.queryExecutor(ctx).QueryContext(ctx, microEventProjectionSelect+`
WHERE event.id>$1 AND event.status=ANY($2)
GROUP BY event.id,storyline.value,heat.value,evidence_state.value
ORDER BY event.id LIMIT $3`, query.CursorID, statuses, query.Limit+1)
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
		page.NextCursorID = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}

func (repository *MicroEventQueryPostgresRepository) GetMicroEvent(ctx context.Context, id int64) (eventapplication.MicroEventProjectionDTO, error) {
	if repository == nil || repository.runtime == nil || id <= 0 {
		return eventapplication.MicroEventProjectionDTO{}, eventapplication.ErrInvalidMicroEventQuery
	}
	rows, err := repository.queryExecutor(ctx).QueryContext(ctx, microEventProjectionSelect+`
WHERE event.id=$1 GROUP BY event.id,storyline.value,heat.value,evidence_state.value`, id)
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
	return record.dto()
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
	rows, err := repository.queryExecutor(ctx).QueryContext(ctx, `
WITH projection AS (
 SELECT evidence.*,claim.micro_event_id,claim.subject,claim.predicate,claim.object,
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
 WHERE claim.micro_event_id=$1 AND evidence.id>$2
)
SELECT id,version,claim_id,document_version_id,text_quote_selector_id,content_family_id,lineage_root_document_version_id,
       lineage_decision_id,member_version,subject,predicate,object,relation,
       CASE WHEN citable THEN 'ready' WHEN retention_until<=$3 THEN 'retention_expired' ELSE 'rights_unavailable' END,
       CASE WHEN citable THEN exact_quote END,CASE WHEN citable THEN prefix END,CASE WHEN citable THEN suffix END,
       CASE WHEN citable THEN utf8_byte_start END,CASE WHEN citable THEN utf8_byte_end END,
       CASE WHEN citable THEN quote_sha256 END,CASE WHEN citable THEN plaintext_sha256 END,
       CASE WHEN citable THEN selector_version END,CASE WHEN citable THEN markdown_anchor END,
       source_record_url_snapshot,canonical_url_snapshot,publisher_name_snapshot,content_origin_name_snapshot,
       published_at_snapshot,captured_at_snapshot,extraction_schema_version,decision_origin,created_at
FROM projection ORDER BY id LIMIT $4`, query.MicroEventID, query.CursorID, query.AsOf.UTC(), query.Limit+1)
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
		if err := rows.Scan(&item.ID, &item.Version, &item.ClaimID, &item.DocumentVersionID, &item.TextQuoteSelectorID,
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
		page.NextCursorID = page.Items[len(page.Items)-1].ID
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
		&record.storylineJSON, &record.heatJSON, &record.stateJSON, &record.familyCount, &record.documentCount); err != nil {
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
			WindowStartedAt: projected.WindowStartedAt.UTC(), WindowEndedAt: projected.WindowEndedAt.UTC(),
			IndependentLineageRoots: projected.IndependentLineageRoots, Velocity: projected.Velocity,
			Acceleration: projected.Acceleration, Coverage: projected.Coverage, NormalizedEngagement: projected.NormalizedEngagement,
			Recency: projected.Recency, AvailableWeight: projected.AvailableWeight, HeatScore: projected.HeatScore,
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
