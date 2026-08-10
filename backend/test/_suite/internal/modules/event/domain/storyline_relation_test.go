package domain

import (
	"testing"
	"time"
)

func TestStorylineRelationKeepsSpecificMicroEventsAndConnectsLongRunningSubject(t *testing.T) {
	now := time.Now().UTC()
	decision, err := DecideStorylineRelation(StorylineRelationInput{MicroEventID: 2, MicroEventTime: now,
		ProfileVersion: "storyline-relation-v1", Candidates: []StorylineCandidate{{StorylineID: 7,
			StorylineVersion: 3, SubjectSimilarity: 1, ActionOverlap: 0, TimeRecency: .8,
			LatestEventAt: now.Add(-24 * time.Hour)}}})
	if err != nil || decision.CreateNew || decision.StorylineID != 7 || decision.RelationType != "continues" {
		t.Fatalf("continuation = %#v / %v", decision, err)
	}
	if decision.RelationScore <= .55 {
		t.Fatalf("relation score = %v", decision.RelationScore)
	}
}

func TestStorylineRelationDoesNotCollapseDifferentSubjects(t *testing.T) {
	now := time.Now().UTC()
	decision, err := DecideStorylineRelation(StorylineRelationInput{MicroEventID: 2, MicroEventTime: now,
		ProfileVersion: "storyline-relation-v1", Candidates: []StorylineCandidate{{StorylineID: 7,
			StorylineVersion: 3, SubjectSimilarity: .2, ActionOverlap: 1, TimeRecency: 1,
			LatestEventAt: now.Add(-time.Hour)}}})
	if err != nil || !decision.CreateNew || decision.StorylineID != 0 {
		t.Fatalf("different subject = %#v / %v", decision, err)
	}
}
