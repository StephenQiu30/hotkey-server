package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	reportapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// CandidateReader freezes the latest Product Event update and independently
// cited summary observed inside the report period. Legacy events never enter
// new report drafts.
type CandidateReader struct{ runtime *database.Runtime }

func NewCandidateReader(runtime *database.Runtime) *CandidateReader {
	return &CandidateReader{runtime: runtime}
}

func (reader *CandidateReader) ListForPeriod(ctx context.Context, monitorID *int64, start, end time.Time, limit int) ([]reportapplication.EventSnapshot, error) {
	if reader == nil || reader.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if start.IsZero() || !end.After(start) || limit < 1 || limit > 100 {
		return nil, sharedrepository.ErrInvalidInput
	}
	queryer := reportQueryerFor(ctx, reader.runtime)
	rows, err := queryer.QueryContext(ctx, `
SELECT event_row.id,event_row.version,update_row.id,summary_row.id,
       concat_ws(' ',event_row.primary_subject_key,event_row.primary_action_key),
       COALESCE((SELECT string_agg(sentence.sentence,' ' ORDER BY sentence.ordinal)
                 FROM micro_event_summary_sentences AS sentence
                 WHERE sentence.micro_event_summary_id=summary_row.id),''),
       update_row.heat_score,evidence_snapshot.evidence_set_hash,update_row.reason_codes
FROM micro_events AS event_row
JOIN LATERAL (
    SELECT id,heat_score,evidence_state_snapshot_id,reason_codes
    FROM micro_event_updates
    WHERE micro_event_id=event_row.id AND micro_event_version=event_row.version
      AND window_ended_at >= $1 AND window_ended_at < $2
    ORDER BY window_ended_at DESC,id DESC
    LIMIT 1
) AS update_row ON true
JOIN evidence_state_snapshots AS evidence_snapshot ON evidence_snapshot.id=update_row.evidence_state_snapshot_id
JOIN LATERAL (
    SELECT id FROM micro_event_summaries
    WHERE micro_event_id=event_row.id AND micro_event_version=event_row.version
    ORDER BY created_at DESC,id DESC
    LIMIT 1
) AS summary_row ON true
WHERE event_row.status IN ('active','review_pending')
  AND ($3::bigint IS NULL OR EXISTS (
      SELECT 1
      FROM micro_event_members AS member
      JOIN micro_event_membership_decisions AS decision ON decision.id=member.membership_decision_id
      WHERE member.micro_event_id=event_row.id AND member.active AND decision.monitor_id=$3
  ))
ORDER BY update_row.heat_score DESC,event_row.id ASC
LIMIT $4`, start.UTC(), end.UTC(), monitorID, limit)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]reportapplication.EventSnapshot, 0)
	for rows.Next() {
		var item reportapplication.EventSnapshot
		var reasons []byte
		if err := rows.Scan(&item.MicroEventID, &item.MicroEventVersion, &item.MicroEventUpdateID,
			&item.MicroEventSummaryID, &item.Title, &item.Summary, &item.HeatScore, &item.EvidenceSetHash, &reasons); err != nil {
			return nil, databaserepository.MapError(err)
		}
		if err := json.Unmarshal(reasons, &item.ReasonCodes); err != nil {
			return nil, fmt.Errorf("decode report candidate reasons: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	if err := rows.Close(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	for index := range items {
		items[index].Sentences, err = candidateSummarySentences(ctx, queryer, items[index].MicroEventSummaryID)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func candidateSummarySentences(ctx context.Context, queryer reportQueryer, summaryID int64) ([]domain.Sentence, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT sentence.id,sentence.ordinal,sentence.sentence,
sentence.editorial_note,sentence.decision_origin,sentence.model_run_id,sentence.actor_user_id,
COALESCE(json_agg(citation.claim_evidence_version_id ORDER BY citation.ordinal)
    FILTER (WHERE citation.claim_evidence_version_id IS NOT NULL),'[]'::json)
FROM micro_event_summary_sentences AS sentence
LEFT JOIN micro_event_summary_sentence_evidences AS citation ON citation.summary_sentence_id=sentence.id
WHERE sentence.micro_event_summary_id=$1
GROUP BY sentence.id ORDER BY sentence.ordinal`, summaryID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]domain.Sentence, 0)
	for rows.Next() {
		var sentence domain.Sentence
		var modelRunID, actorUserID sql.NullInt64
		var evidenceJSON []byte
		if err := rows.Scan(&sentence.SourceSummarySentenceID, &sentence.Ordinal, &sentence.Text,
			&sentence.EditorialNote, &sentence.DecisionOrigin, &modelRunID, &actorUserID, &evidenceJSON); err != nil {
			return nil, databaserepository.MapError(err)
		}
		sentence.ModelRunID, sentence.ActorUserID = nullableReportIDPointer(modelRunID), nullableReportIDPointer(actorUserID)
		if err := json.Unmarshal(evidenceJSON, &sentence.ClaimEvidenceVersionIDs); err != nil {
			return nil, fmt.Errorf("decode report candidate citations: %w", err)
		}
		result = append(result, sentence)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return result, nil
}

var _ reportapplication.SnapshotReader = (*CandidateReader)(nil)
