package postgres

import (
	"context"
	"encoding/json"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

func (repository *Repository) SaveDecisions(ctx context.Context, decisions []domain.Decision) error {
	if !repository.available() {
		return sharedrepository.ErrUnavailable
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		for _, decision := range decisions {
			if err := decision.Validate(); err != nil {
				return err
			}
			feature, err := json.Marshal(decision.FeatureSnapshot)
			if err != nil {
				return err
			}
			if string(feature) == "null" {
				feature = []byte(`{}`)
			}
			reasons, err := json.Marshal(decision.ReasonCodes)
			if err != nil {
				return err
			}
			if string(reasons) == "null" {
				reasons = []byte(`[]`)
			}
			evidence, err := json.Marshal(decision.EvidenceContentIDs)
			if err != nil {
				return err
			}
			if string(evidence) == "null" {
				evidence = []byte(`[]`)
			}
			var candidateID any
			if decision.CandidateEventID != nil {
				candidateID = *decision.CandidateEventID
			}
			_, err = transaction.SQL.ExecContext(ctx, `
INSERT INTO event_clustering_decisions (content_id, candidate_event_id, candidate_event_key, clustering_version, feature_input_hash, channel, candidate_rank, entity_action_score, semantic_score, temporal_score, location_score, source_context_score, membership_score, decision, decision_origin, reason_codes, feature_snapshot, evidence_content_ids, actor_user_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,ARRAY(SELECT jsonb_array_elements_text($16::jsonb)),COALESCE($17::jsonb,'{}'::jsonb),ARRAY(SELECT (jsonb_array_elements_text($18::jsonb))::bigint),$19)
ON CONFLICT (content_id, clustering_version, feature_input_hash, candidate_event_key) DO NOTHING`, decision.ContentID, candidateID, decision.CandidateEventKey, decision.ClusteringVersion, decision.FeatureInputHash, decision.Channel, decision.CandidateRank, decision.Scores.EntityAction, decision.Scores.Semantic, decision.Scores.Temporal, decision.Scores.Location, decision.Scores.SourceContext, decision.MembershipScore, decision.Decision, decision.DecisionOrigin, reasons, feature, evidence, decision.ActorUserID)
			if err != nil {
				return sharedrepository.MapError(err)
			}
		}
		return nil
	})
}
