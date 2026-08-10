package domain

import "testing"

func TestMicroEventClusteringKeepsIndependentFeaturesAndColdStartThresholds(t *testing.T) {
	t.Parallel()
	strong := MicroEventCandidate{MicroEventID: 10, EventVersion: 3, DenseAvailable: true, Features: MicroEventFeatures{
		SparseSimilarity: .94, DenseSimilarity: .95, EntityOverlap: 1, ActionOverlap: 1,
		LocationConsistency: .95, IdentifierConsistency: .98, TimeSimilarity: .92, LineageRelation: .90,
	}}
	weaker := MicroEventCandidate{MicroEventID: 11, EventVersion: 2, DenseAvailable: true, Features: MicroEventFeatures{
		SparseSimilarity: .78, DenseSimilarity: .79, EntityOverlap: .75, ActionOverlap: .8,
		LocationConsistency: .8, IdentifierConsistency: .75, TimeSimilarity: .8, LineageRelation: .7,
	}}
	decision, err := DecideMicroEventMembership(MicroEventDecisionInput{ContentFamilyID: 7,
		Candidates: []MicroEventCandidate{strong, weaker}, ProfileVersion: "same-event-cold-start-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != MicroEventActionJoin || decision.MicroEventID != 10 || decision.SameEventScore < .90 || decision.LeadingMargin < .15 {
		t.Fatalf("strong decision = %#v", decision)
	}
	if decision.Features.SparseSimilarity == decision.Features.DenseSimilarity &&
		decision.Features.DenseSimilarity == decision.Features.EntityOverlap {
		t.Fatal("independent feature dimensions were collapsed into one score")
	}
}

func TestMicroEventClusteringReviewsAmbiguityAndCreatesOnHardConflict(t *testing.T) {
	t.Parallel()
	base := MicroEventCandidate{MicroEventID: 12, EventVersion: 1, Features: MicroEventFeatures{
		SparseSimilarity: .82, DenseSimilarity: .83, EntityOverlap: .75, ActionOverlap: .8,
		LocationConsistency: .75, IdentifierConsistency: .8, TimeSimilarity: .78, LineageRelation: .72,
	}}
	review, err := DecideMicroEventMembership(MicroEventDecisionInput{ContentFamilyID: 8,
		Candidates: []MicroEventCandidate{base}, ProfileVersion: "same-event-cold-start-v1"})
	if err != nil || review.Action != MicroEventActionReview || review.SameEventScore < .60 || review.SameEventScore >= .90 {
		t.Fatalf("review decision = %#v / %v", review, err)
	}
	base.HardConflict = true
	base.HardConflictReasons = []string{"location_conflict"}
	created, err := DecideMicroEventMembership(MicroEventDecisionInput{ContentFamilyID: 9,
		Candidates: []MicroEventCandidate{base}, ProfileVersion: "same-event-cold-start-v1"})
	if err != nil || created.Action != MicroEventActionCreate || created.MicroEventID != 0 || created.ReasonCodes[0] != "hard_conflict" {
		t.Fatalf("hard-conflict decision = %#v / %v", created, err)
	}
}

func TestMicroEventClusteringDoesNotMergeSameTopicDifferentOccurrence(t *testing.T) {
	t.Parallel()
	candidate := MicroEventCandidate{MicroEventID: 15, EventVersion: 5, Features: MicroEventFeatures{
		SparseSimilarity: .93, DenseSimilarity: .92, EntityOverlap: 1, ActionOverlap: 1,
		LocationConsistency: 1, IdentifierConsistency: .2, TimeSimilarity: .1, LineageRelation: .2,
	}, HardConflict: true, HardConflictReasons: []string{"time_conflict", "identifier_conflict"}}
	decision, err := DecideMicroEventMembership(MicroEventDecisionInput{ContentFamilyID: 12,
		Candidates: []MicroEventCandidate{candidate}, ProfileVersion: "same-event-cold-start-v1"})
	if err != nil || decision.Action != MicroEventActionCreate {
		t.Fatalf("same-topic hard negative = %#v / %v", decision, err)
	}
}
