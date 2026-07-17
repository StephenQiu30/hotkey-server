package application

import (
	"context"
	"fmt"
	"sort"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type ClusteringInput struct {
	ContentID         int64
	ClusteringVersion string
	FeatureInputHash  string
	Candidates        []domain.Candidate
	Scores            map[string]domain.ScoreBreakdown
	HardConflicts     map[string]bool
}

func (input ClusteringInput) Validate() error {
	if input.ContentID <= 0 || input.ClusteringVersion == "" || len(input.ClusteringVersion) > 64 || len(input.FeatureInputHash) != 64 {
		return fmt.Errorf("invalid clustering input")
	}
	if len(input.Candidates) > domain.MaxCandidates {
		return fmt.Errorf("candidate limit exceeded")
	}
	for _, candidate := range input.Candidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
		if _, ok := input.Scores[candidate.EventKey]; !ok {
			return fmt.Errorf("missing score for %s", candidate.EventKey)
		}
	}
	return nil
}

type ClusteringService struct{}

func NewClusteringService() *ClusteringService { return &ClusteringService{} }

func (service *ClusteringService) Evaluate(_ context.Context, input ClusteringInput) ([]domain.Decision, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	candidates := domain.CompareCandidates(input.Candidates)
	decisions := make([]domain.Decision, 0, len(candidates)+1)
	for rank, candidate := range candidates {
		scores := input.Scores[candidate.EventKey]
		hardConflict := input.HardConflicts[candidate.EventKey] || candidate.HardConflict
		decision, score, err := domain.Decide(scores, hardConflict)
		if err != nil {
			return nil, err
		}
		candidateID := candidate.EventID
		persistedDecision := decision
		if decision == domain.DecisionNewEvent {
			persistedDecision = domain.DecisionReject
		}
		decisions = append(decisions, domain.Decision{
			ContentID: input.ContentID, CandidateEventID: &candidateID, CandidateEventKey: candidate.EventKey,
			ClusteringVersion: input.ClusteringVersion, FeatureInputHash: input.FeatureInputHash,
			Channel: candidate.Channel, CandidateRank: rank + 1, Scores: scores, MembershipScore: score,
			Decision: persistedDecision, DecisionOrigin: domain.DecisionOriginRule,
		})
	}
	winner := -1
	for index := range decisions {
		if decisions[index].Decision != domain.DecisionAccept {
			continue
		}
		if winner == -1 || decisions[index].MembershipScore > decisions[winner].MembershipScore || decisions[index].MembershipScore == decisions[winner].MembershipScore && decisions[index].CandidateEventKey < decisions[winner].CandidateEventKey {
			winner = index
		}
	}
	for index := range decisions {
		if decisions[index].Decision == domain.DecisionAccept && index != winner {
			decisions[index].Decision = domain.DecisionReject
			decisions[index].ReasonCodes = append(decisions[index].ReasonCodes, "competing_candidate_selected")
		}
	}
	hasJoinDecision := false
	for _, decision := range decisions {
		if decision.Decision == domain.DecisionAccept || decision.Decision == domain.DecisionReview {
			hasJoinDecision = true
			break
		}
	}
	if len(decisions) == 0 || !hasJoinDecision {
		decisions = append(decisions, domain.Decision{
			ContentID: input.ContentID, CandidateEventKey: "__new_event__", ClusteringVersion: input.ClusteringVersion,
			FeatureInputHash: input.FeatureInputHash, Channel: domain.ChannelFingerprint, CandidateRank: 0,
			Decision: domain.DecisionNewEvent, DecisionOrigin: domain.DecisionOriginRule,
		})
	}
	sort.SliceStable(decisions, func(i, j int) bool {
		if decisions[i].MembershipScore != decisions[j].MembershipScore {
			return decisions[i].MembershipScore > decisions[j].MembershipScore
		}
		return decisions[i].CandidateEventKey < decisions[j].CandidateEventKey
	})
	return decisions, nil
}
