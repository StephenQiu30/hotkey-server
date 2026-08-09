package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var (
	ErrIntentVersionConflict      = errors.New("monitor intent resource version conflict")
	ErrExpansionCandidateNotFound = errors.New("expansion candidate was not found")
)

// IntentDraft is an immutable aggregate snapshot. Every successful mutation
// returns a new resource version; replacing the definition discards candidates
// tied to the previous input so stale expansions cannot leak forward.
type IntentDraft struct {
	monitorID       int64
	draftID         int64
	resourceVersion int64
	definition      IntentDefinition
	candidates      []ExpansionCandidate
}

func NewIntentDraft(monitorID, draftID, resourceVersion int64, definition IntentDefinition, candidates []ExpansionCandidate) (IntentDraft, error) {
	if monitorID <= 0 || draftID <= 0 || resourceVersion <= 0 || definition.fingerprint == "" {
		return IntentDraft{}, fmt.Errorf("%w: draft identity is invalid", ErrInvalidIntent)
	}
	if len(candidates) > maximumIntentClauses {
		return IntentDraft{}, fmt.Errorf("%w: too many expansion candidates", ErrInvalidExpansionCandidate)
	}
	validated := make([]ExpansionCandidate, len(candidates))
	identities := make(map[string]struct{}, len(candidates))
	for index, candidate := range candidates {
		candidate, err := RestoreExpansionCandidate(
			candidate.id, candidate.value, candidate.provenance, candidate.assessment,
			candidate.status, candidate.Review(),
		)
		if err != nil {
			return IntentDraft{}, err
		}
		if _, duplicate := identities[candidate.id]; duplicate {
			return IntentDraft{}, fmt.Errorf("%w: duplicate candidate id", ErrInvalidExpansionCandidate)
		}
		identities[candidate.id] = struct{}{}
		validated[index] = candidate
	}
	return IntentDraft{monitorID: monitorID, draftID: draftID, resourceVersion: resourceVersion, definition: definition, candidates: validated}, nil
}

func (draft IntentDraft) MonitorID() int64             { return draft.monitorID }
func (draft IntentDraft) DraftID() int64               { return draft.draftID }
func (draft IntentDraft) ResourceVersion() int64       { return draft.resourceVersion }
func (draft IntentDraft) Definition() IntentDefinition { return draft.definition }
func (draft IntentDraft) Candidates() []ExpansionCandidate {
	return append([]ExpansionCandidate(nil), draft.candidates...)
}

func (draft IntentDraft) ReplaceDefinition(expectedResourceVersion int64, definition IntentDefinition) (IntentDraft, error) {
	if draft.resourceVersion != expectedResourceVersion {
		return IntentDraft{}, ErrIntentVersionConflict
	}
	if definition.fingerprint == "" {
		return IntentDraft{}, ErrInvalidIntent
	}
	return NewIntentDraft(draft.monitorID, draft.draftID, draft.resourceVersion+1, definition, nil)
}

// AttachExpansionCandidates accepts one successful expansion run only while
// the exact monitor draft used by that run is still current. A caller cannot
// substitute a preview, unfinished run, or same-numbered draft from another
// Monitor. Generated values remain pending until an explicit review mutation.
func (draft IntentDraft) AttachExpansionCandidates(expectedResourceVersion int64, run IntentAnalysisRun, candidates []ExpansionCandidate) (IntentDraft, error) {
	if draft.resourceVersion != expectedResourceVersion ||
		run.monitorID != draft.monitorID || run.draftID != draft.draftID ||
		run.draftResourceVersion != draft.resourceVersion {
		return IntentDraft{}, ErrIntentVersionConflict
	}
	if run.kind != IntentRunExpansion || run.status != IntentRunSucceeded || !run.UsableForDraft(draft.draftID, draft.resourceVersion) || !validIntentSHA256(run.inputHash) {
		return IntentDraft{}, fmt.Errorf("%w: a successful expansion run is required", ErrInvalidExpansionCandidate)
	}
	if len(candidates) == 0 || len(draft.candidates)+len(candidates) > maximumIntentClauses {
		return IntentDraft{}, fmt.Errorf("%w: expansion candidate count is invalid", ErrInvalidExpansionCandidate)
	}
	combined := append([]ExpansionCandidate(nil), draft.candidates...)
	ids := make(map[string]struct{}, len(combined)+len(candidates))
	batchValues := make(map[string]struct{}, len(candidates))
	for _, candidate := range combined {
		ids[candidate.id] = struct{}{}
	}
	for _, candidate := range candidates {
		if candidate.status != ExpansionApprovalPending {
			return IntentDraft{}, fmt.Errorf("%w: generated candidate must be pending", ErrInvalidExpansionCandidate)
		}
		validated, err := RestoreExpansionCandidate(candidate.id, candidate.value, candidate.provenance, candidate.assessment, candidate.status, nil)
		if err != nil {
			return IntentDraft{}, err
		}
		if run.inputHash != validated.provenance.inputHash {
			return IntentDraft{}, fmt.Errorf("%w: candidate input hash does not match its expansion run", ErrInvalidExpansionCandidate)
		}
		if _, duplicate := ids[validated.id]; duplicate {
			return IntentDraft{}, fmt.Errorf("%w: duplicate candidate id", ErrInvalidExpansionCandidate)
		}
		valueKey := canonicalIntentKey(validated.value)
		if _, duplicate := batchValues[valueKey]; duplicate {
			return IntentDraft{}, fmt.Errorf("%w: duplicate candidate value", ErrInvalidExpansionCandidate)
		}
		ids[validated.id] = struct{}{}
		batchValues[valueKey] = struct{}{}
		combined = append(combined, validated)
	}
	return NewIntentDraft(draft.monitorID, draft.draftID, draft.resourceVersion+1, draft.definition, combined)
}

func (draft IntentDraft) ReviewCandidate(expectedResourceVersion int64, candidateID string, decision ExpansionDecision, reviewerUserID int64, reviewedAt time.Time, note string) (IntentDraft, error) {
	if draft.resourceVersion != expectedResourceVersion {
		return IntentDraft{}, ErrIntentVersionConflict
	}
	candidates := append([]ExpansionCandidate(nil), draft.candidates...)
	found := false
	for index, candidate := range candidates {
		if candidate.id != candidateID {
			continue
		}
		reviewed, err := candidate.Decide(decision, reviewerUserID, reviewedAt, note)
		if err != nil {
			return IntentDraft{}, err
		}
		candidates[index] = reviewed
		found = true
		break
	}
	if !found {
		return IntentDraft{}, ErrExpansionCandidateNotFound
	}
	return NewIntentDraft(draft.monitorID, draft.draftID, draft.resourceVersion+1, draft.definition, candidates)
}

// MatchingFingerprint changes only when facts allowed to influence collection
// or matching change. Pending/rejected candidates and reviewer metadata are
// deliberately absent.
func (draft IntentDraft) MatchingFingerprint() string {
	digest := sha256.New()
	writeIntentHashPart(digest, "monitor-intent-matching-v1")
	writeIntentHashPart(digest, draft.definition.fingerprint)
	approved := ApprovedExpansionCandidates(draft.candidates)
	writeIntentHashPart(digest, "approved-expansion-candidates")
	writeIntentHashPart(digest, fmt.Sprintf("%d", len(approved)))
	for _, candidate := range approved {
		writeIntentHashPart(digest, candidate.value)
		writeIntentHashPart(digest, string(candidate.provenance.source))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// PublishedIntentVersion freezes the exact intent snapshot used by a published
// Monitor revision. All candidate decisions remain available for audit, while
// ApprovedCandidates is the only compilation-safe projection.
type PublishedIntentVersion struct {
	id                         int64
	monitorID                  int64
	revision                   int64
	sourceDraftResourceVersion int64
	definition                 IntentDefinition
	candidates                 []ExpansionCandidate
	publishedAt                time.Time
}

func NewPublishedIntentVersion(revision int64, draft IntentDraft, publishedAt time.Time) (PublishedIntentVersion, error) {
	if revision <= 0 || draft.monitorID <= 0 || draft.draftID <= 0 || draft.resourceVersion <= 0 || publishedAt.IsZero() {
		return PublishedIntentVersion{}, fmt.Errorf("%w: published intent version identity is invalid", ErrInvalidIntent)
	}
	return PublishedIntentVersion{
		id: draft.draftID, monitorID: draft.monitorID, revision: revision,
		sourceDraftResourceVersion: draft.resourceVersion, definition: draft.definition,
		candidates: append([]ExpansionCandidate(nil), draft.candidates...), publishedAt: publishedAt.UTC(),
	}, nil
}

func (version PublishedIntentVersion) ID() int64        { return version.id }
func (version PublishedIntentVersion) MonitorID() int64 { return version.monitorID }
func (version PublishedIntentVersion) Revision() int64  { return version.revision }
func (version PublishedIntentVersion) SourceDraftResourceVersion() int64 {
	return version.sourceDraftResourceVersion
}
func (version PublishedIntentVersion) Definition() IntentDefinition { return version.definition }
func (version PublishedIntentVersion) PublishedAt() time.Time       { return version.publishedAt }
func (version PublishedIntentVersion) Candidates() []ExpansionCandidate {
	return append([]ExpansionCandidate(nil), version.candidates...)
}
func (version PublishedIntentVersion) ApprovedCandidates() []ExpansionCandidate {
	return ApprovedExpansionCandidates(version.candidates)
}
