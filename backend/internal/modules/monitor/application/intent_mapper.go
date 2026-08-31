package application

import (
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/domain"
)

func intentDefinitionFromDTO(objectiveText string, clauseDTOs []IntentClauseDTO, entityDTOs []IntentEntityDTO, exampleDTOs []IntentExampleDTO) (domain.IntentDefinition, error) {
	objective, err := domain.NewIntentObjective(objectiveText)
	if err != nil {
		return domain.IntentDefinition{}, invalidIntentContract(err)
	}
	clauses := make([]domain.IntentClause, 0, len(clauseDTOs))
	for _, item := range clauseDTOs {
		clause, clauseErr := domain.NewIntentClause(domain.IntentClauseOperator(item.Operator), domain.IntentClauseField(item.Field), item.Value)
		if clauseErr != nil {
			return domain.IntentDefinition{}, invalidIntentContract(clauseErr)
		}
		clauses = append(clauses, clause)
	}
	entities := make([]domain.IntentEntity, 0, len(entityDTOs))
	for _, item := range entityDTOs {
		entity, entityErr := domain.NewIntentEntity(item.CanonicalID, item.DisplayName, item.Aliases, item.AmbiguityNote)
		if entityErr != nil {
			return domain.IntentDefinition{}, invalidIntentContract(entityErr)
		}
		entities = append(entities, entity)
	}
	examples := make([]domain.IntentExample, 0, len(exampleDTOs))
	for _, item := range exampleDTOs {
		example, exampleErr := domain.NewIntentExample(domain.IntentExampleLabel(item.Label), item.Text)
		if exampleErr != nil {
			return domain.IntentDefinition{}, invalidIntentContract(exampleErr)
		}
		examples = append(examples, example)
	}
	definition, err := domain.NewIntentDefinition(objective, clauses, entities, examples)
	if err != nil {
		return domain.IntentDefinition{}, invalidIntentContract(err)
	}
	return definition, nil
}

func intentDefinitionToDTO(definition domain.IntentDefinition) (string, []IntentClauseDTO, []IntentEntityDTO, []IntentExampleDTO) {
	clauses := make([]IntentClauseDTO, 0, len(definition.Clauses()))
	for _, clause := range definition.Clauses() {
		clauses = append(clauses, IntentClauseDTO{Operator: string(clause.Operator()), Field: string(clause.Field()), Value: clause.Value()})
	}
	entities := make([]IntentEntityDTO, 0, len(definition.Entities()))
	for _, entity := range definition.Entities() {
		entities = append(entities, IntentEntityDTO{
			CanonicalID: entity.CanonicalID(), DisplayName: entity.DisplayName(),
			Aliases: entity.Aliases(), AmbiguityNote: entity.AmbiguityNote(),
		})
	}
	examples := make([]IntentExampleDTO, 0, len(definition.Examples()))
	for _, example := range definition.Examples() {
		examples = append(examples, IntentExampleDTO{Label: string(example.Label()), Text: example.Text()})
	}
	return definition.Objective().String(), clauses, entities, examples
}

func expansionCandidateFromDTO(item ExpansionCandidateDTO) (domain.ExpansionCandidate, error) {
	provenance, err := domain.NewExpansionProvenance(
		domain.ExpansionSource(item.Source), item.Reason, item.ModelVersion,
		item.PromptVersion, item.InputHash,
	)
	if err != nil {
		return domain.ExpansionCandidate{}, invalidIntentContract(err)
	}
	assessment, err := domain.NewExpansionAssessment(item.Similarity, domain.ExpansionRisk(item.Risk))
	if err != nil {
		return domain.ExpansionCandidate{}, invalidIntentContract(err)
	}
	status := domain.ExpansionApprovalStatus(item.ApprovalStatus)
	var review *domain.ExpansionReview
	if status != domain.ExpansionApprovalPending {
		if item.ReviewerUserID == nil || item.ReviewedAt == nil {
			return domain.ExpansionCandidate{}, invalidIntentContract(fmt.Errorf("terminal candidate review is incomplete"))
		}
		decision := domain.ExpansionDecisionReject
		if status == domain.ExpansionApprovalApproved {
			decision = domain.ExpansionDecisionApprove
		}
		created, reviewErr := domain.NewExpansionReview(decision, *item.ReviewerUserID, *item.ReviewedAt, item.ReviewNote)
		if reviewErr != nil {
			return domain.ExpansionCandidate{}, invalidIntentContract(reviewErr)
		}
		review = &created
	} else if item.ReviewerUserID != nil || item.ReviewedAt != nil || item.ReviewNote != "" {
		return domain.ExpansionCandidate{}, invalidIntentContract(fmt.Errorf("pending candidate cannot contain review facts"))
	}
	candidate, err := domain.RestoreExpansionCandidate(item.ID, item.Value, provenance, assessment, status, review)
	if err != nil {
		return domain.ExpansionCandidate{}, invalidIntentContract(err)
	}
	return candidate, nil
}

func expansionCandidateToDTO(candidate domain.ExpansionCandidate) ExpansionCandidateDTO {
	provenance := candidate.Provenance()
	assessment := candidate.Assessment()
	result := ExpansionCandidateDTO{
		ID: candidate.ID(), Value: candidate.Value(), Source: string(provenance.Source()),
		Reason: provenance.Reason(), ModelVersion: provenance.ModelVersion(),
		PromptVersion: provenance.PromptVersion(), InputHash: provenance.InputHash(),
		Similarity: assessment.Similarity(), Risk: string(assessment.Risk()),
		ApprovalStatus: string(candidate.ApprovalStatus()),
	}
	if review := candidate.Review(); review != nil {
		reviewer := review.ReviewerUserID()
		reviewedAt := review.ReviewedAt()
		result.ReviewerUserID = &reviewer
		result.ReviewedAt = &reviewedAt
		result.ReviewNote = review.Note()
	}
	return result
}

func intentDraftFromDTO(item IntentDraftDTO) (domain.IntentDraft, error) {
	definition, err := intentDefinitionFromDTO(item.Objective, item.Clauses, item.Entities, item.Examples)
	if err != nil {
		return domain.IntentDraft{}, err
	}
	candidates := make([]domain.ExpansionCandidate, 0, len(item.Candidates))
	for _, candidateDTO := range item.Candidates {
		candidate, candidateErr := expansionCandidateFromDTO(candidateDTO)
		if candidateErr != nil {
			return domain.IntentDraft{}, candidateErr
		}
		candidates = append(candidates, candidate)
	}
	draft, err := domain.NewIntentDraft(item.MonitorID, item.DraftID, item.ResourceVersion, definition, candidates)
	if err != nil {
		return domain.IntentDraft{}, invalidIntentContract(err)
	}
	return draft, nil
}

func intentDraftToDTO(draft domain.IntentDraft) IntentDraftDTO {
	objective, clauses, entities, examples := intentDefinitionToDTO(draft.Definition())
	candidates := make([]ExpansionCandidateDTO, 0, len(draft.Candidates()))
	for _, candidate := range draft.Candidates() {
		candidates = append(candidates, expansionCandidateToDTO(candidate))
	}
	return IntentDraftDTO{
		MonitorID: draft.MonitorID(), DraftID: draft.DraftID(), ResourceVersion: draft.ResourceVersion(),
		Objective: objective, Clauses: clauses, Entities: entities,
		Examples: examples, Candidates: candidates,
	}
}

func intentRunFromDTO(item IntentRunDTO) (domain.IntentAnalysisRun, error) {
	run, err := domain.RestoreIntentAnalysisRun(
		item.ID, domain.IntentRunKind(item.Kind), item.MonitorID, item.DraftID, item.DraftResourceVersion,
		item.InputHash, domain.IntentRunStatus(item.Status), item.QueuedAt,
		item.StartedAt, item.CompletedAt, item.InvalidatedAt, item.FailureReason,
	)
	if err != nil {
		return domain.IntentAnalysisRun{}, invalidIntentContract(err)
	}
	return run, nil
}

func intentRunToDTO(run domain.IntentAnalysisRun) IntentRunDTO {
	return IntentRunDTO{
		ID: run.ID(), Kind: string(run.Kind()), MonitorID: run.MonitorID(),
		DraftID: run.DraftID(), DraftResourceVersion: run.DraftResourceVersion(), InputHash: run.InputHash(), Status: string(run.Status()),
		QueuedAt: run.QueuedAt(), StartedAt: run.StartedAt(), CompletedAt: run.CompletedAt(),
		InvalidatedAt: run.InvalidatedAt(), FailureReason: run.FailureReason(),
	}
}

func invalidIntentContract(cause error) error {
	return fmt.Errorf("%w: %w", ErrInvalidIntentContract, cause)
}
