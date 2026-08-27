package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type microEventClusteringGolden struct {
	ProfileVersion string                           `json:"profile_version"`
	Cases          []microEventClusteringGoldenCase `json:"cases"`
}

type microEventClusteringGoldenCase struct {
	Name                    string                                `json:"name"`
	Coverage                string                                `json:"coverage"`
	ContentFamilyID         int64                                 `json:"content_family_id"`
	DocumentMatchDecisionID int64                                 `json:"document_match_decision_id"`
	Candidates              []microEventClusteringGoldenCandidate `json:"candidates"`
	ExistingAssignment      *struct {
		EventID                 int64  `json:"event_id"`
		EventVersion            int64  `json:"event_version"`
		DecisionID              int64  `json:"decision_id"`
		OriginalMatchDecisionID int64  `json:"original_match_decision_id"`
		Action                  string `json:"action"`
	} `json:"existing_assignment"`
	ExpectedAction  string `json:"expected_action"`
	ExpectedEventID int64  `json:"expected_event_id"`
	ExpectedCommit  bool   `json:"expected_commit"`
}

type microEventClusteringGoldenCandidate struct {
	MicroEventID        int64    `json:"micro_event_id"`
	EventVersion        int64    `json:"event_version"`
	DenseAvailable      bool     `json:"dense_available"`
	HardConflict        bool     `json:"hard_conflict"`
	HardConflictReasons []string `json:"hard_conflict_reasons"`
	Features            struct {
		Sparse     float64 `json:"sparse"`
		Dense      float64 `json:"dense"`
		Entity     float64 `json:"entity"`
		Action     float64 `json:"action"`
		Location   float64 `json:"location"`
		Identifier float64 `json:"identifier"`
		Time       float64 `json:"time"`
		Lineage    float64 `json:"lineage"`
	} `json:"features"`
}

func TestMicroEventClusteringGoldenCoversLineageReplayAndCandidateIdempotency(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("testdata", "micro-event-clustering", "v1", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture microEventClusteringGolden
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	wantedCoverage := map[string]bool{
		"cross_source": false, "syndicated": false, "same_source_comment": false,
		"near_duplicate": false, "late_arrival": false, "duplicate_candidate": false,
	}
	if fixture.ProfileVersion != CanonicalMicroEventClusteringProfileVersion || len(fixture.Cases) != len(wantedCoverage) {
		t.Fatalf("invalid micro-event clustering fixture: %#v", fixture)
	}
	for _, testCase := range fixture.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			if _, exists := wantedCoverage[testCase.Coverage]; !exists || wantedCoverage[testCase.Coverage] {
				t.Fatalf("missing or duplicate coverage label %q", testCase.Coverage)
			}
			wantedCoverage[testCase.Coverage] = true
			target := MicroEventAssignmentTargetDTO{
				ContentFamilyID: testCase.ContentFamilyID, DocumentMatchDecisionID: testCase.DocumentMatchDecisionID,
				DocumentVersionID: testCase.DocumentMatchDecisionID + 10_000, MonitorID: 7, MonitorVersionID: 8,
				EffectiveMatchDecision: "accepted", PrimarySubjectKey: "subject:hotkey", PrimaryActionKey: "action:released",
				OccurredAt: time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC),
			}
			for _, candidate := range testCase.Candidates {
				target.Candidates = append(target.Candidates, MicroEventCandidateDTO{
					MicroEventID: candidate.MicroEventID, EventVersion: candidate.EventVersion,
					DenseAvailable: candidate.DenseAvailable, HardConflict: candidate.HardConflict,
					HardConflictReasons: append([]string(nil), candidate.HardConflictReasons...),
					Features: MicroEventFeaturesDTO{
						SparseSimilarity: candidate.Features.Sparse, DenseSimilarity: candidate.Features.Dense,
						EntityOverlap: candidate.Features.Entity, ActionOverlap: candidate.Features.Action,
						LocationConsistency: candidate.Features.Location, IdentifierConsistency: candidate.Features.Identifier,
						TimeSimilarity: candidate.Features.Time, LineageRelation: candidate.Features.Lineage,
					},
				})
			}
			if existing := testCase.ExistingAssignment; existing != nil {
				target.ExistingAssignment = &CommitMicroEventMembershipResult{
					Event: MicroEventDTO{ID: existing.EventID, Version: existing.EventVersion,
						EventKey: "existing-event", PrimarySubjectKey: target.PrimarySubjectKey,
						PrimaryActionKey: target.PrimaryActionKey, EventStartedAt: target.OccurredAt,
						ClusteringProfileVersion: fixture.ProfileVersion},
					Decision: MicroEventMembershipDecisionDTO{ID: existing.DecisionID, ContentFamilyID: target.ContentFamilyID,
						DocumentMatchDecisionID: existing.OriginalMatchDecisionID, MicroEventID: existing.EventID,
						EventVersion: existing.EventVersion, Action: existing.Action,
						ClusteringProfileVersion: fixture.ProfileVersion},
				}
			}
			repository := &microEventGoldenRepository{target: target}
			service, err := NewMicroEventService(repository)
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Assign(context.Background(), AssignContentFamilyToMicroEventCommand{
				ContentFamilyID: testCase.ContentFamilyID, DocumentMatchDecisionID: testCase.DocumentMatchDecisionID,
				ClusteringProfileVersion: fixture.ProfileVersion,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Decision.Action != testCase.ExpectedAction || result.Event.ID != testCase.ExpectedEventID || (repository.commitCount == 1) != testCase.ExpectedCommit {
				t.Fatalf("result=%#v commits=%d, want action=%q event=%d commit=%v", result, repository.commitCount,
					testCase.ExpectedAction, testCase.ExpectedEventID, testCase.ExpectedCommit)
			}
		})
	}
	for coverage, found := range wantedCoverage {
		if !found {
			t.Errorf("fixture is missing %s coverage", coverage)
		}
	}
}

type microEventGoldenRepository struct {
	target      MicroEventAssignmentTargetDTO
	commitCount int
}

func (repository *microEventGoldenRepository) ReadMicroEventAssignmentTarget(context.Context, ReadMicroEventAssignmentTargetQuery) (MicroEventAssignmentTargetDTO, error) {
	return repository.target, nil
}

func (repository *microEventGoldenRepository) CommitMicroEventMembership(_ context.Context, command CommitMicroEventMembershipCommand) (CommitMicroEventMembershipResult, error) {
	repository.commitCount++
	eventID := command.CandidateMicroEventID
	eventVersion := command.ExpectedEventVersion + 1
	status := "active"
	if command.Action == "create" {
		eventID, eventVersion = 9999, 1
	}
	if command.Action == "review" {
		status = "review_pending"
	}
	return CommitMicroEventMembershipResult{
		Event: MicroEventDTO{ID: eventID, Version: eventVersion, EventKey: command.EventKey, Status: status,
			PrimarySubjectKey: command.PrimarySubjectKey, PrimaryActionKey: command.PrimaryActionKey,
			EventStartedAt: command.OccurredAt, ClusteringProfileVersion: command.ClusteringProfileVersion},
		Decision: MicroEventMembershipDecisionDTO{ID: int64(8000 + repository.commitCount), ContentFamilyID: command.ContentFamilyID,
			DocumentMatchDecisionID: command.DocumentMatchDecisionID, MicroEventID: eventID, EventVersion: eventVersion,
			Action: command.Action, SameEventScore: command.SameEventScore, LeadingMargin: command.LeadingMargin,
			Features: command.Features, ClusteringProfileVersion: command.ClusteringProfileVersion,
			ReasonCodes: append([]string(nil), command.ReasonCodes...)},
	}, nil
}
