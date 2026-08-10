package domain

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestContentFingerprintIsDeterministicWithoutRetainingPlaintext(t *testing.T) {
	t.Parallel()
	first, err := BuildContentFingerprint("  华东园区发生爆燃\r\n救援正在进行  ", "content-fingerprint-v1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildContentFingerprint("华东园区发生爆燃\n救援正在进行", "content-fingerprint-v1")
	if err != nil {
		t.Fatal(err)
	}
	if first.NormalizedContentSHA256 != second.NormalizedContentSHA256 || first.SimHashHex != second.SimHashHex || !equalMinHash(first.MinHash, second.MinHash) {
		t.Fatalf("equivalent text produced different fingerprints: %#v / %#v", first, second)
	}
	if len(first.MinHash) != ContentMinHashSize || len(first.SimHashHex) != 16 || strings.Contains(first.NormalizedContentSHA256, "园区") {
		t.Fatalf("invalid or plaintext-bearing fingerprint: %#v", first)
	}
}

func flipSimHashBits(t *testing.T, value string, count int) string {
	t.Helper()
	parsed, err := strconv.ParseUint(value, 16, 64)
	if err != nil {
		t.Fatal(err)
	}
	for bit := 0; bit < count; bit++ {
		parsed ^= uint64(1) << bit
	}
	return fmt.Sprintf("%016x", parsed)
}

func TestContentFamilyDecisionUsesExactAndConservativeNearDuplicateSignals(t *testing.T) {
	t.Parallel()
	base, err := BuildContentFingerprint("Acme launches the red rocket in Shanghai today", "content-fingerprint-v1")
	if err != nil {
		t.Fatal(err)
	}
	exact, err := DecideContentFamily(ContentFamilyDecisionInput{
		DocumentVersionID:      20,
		Fingerprint:            base,
		Candidates:             []ContentFamilyCandidate{{FamilyID: 7, FamilyVersion: 3, RootDocumentVersionID: 10, Fingerprint: base}},
		DecisionProfileVersion: "content-family-decision-v1",
	})
	if err != nil || exact.Action != ContentFamilyActionJoin || exact.Relation != ContentRelationExactCopy || exact.FamilyID != 7 {
		t.Fatalf("exact decision = %#v / %v", exact, err)
	}

	near := base
	near.NormalizedContentSHA256 = strings.Repeat("b", 64)
	near.SimHashHex = flipSimHashBits(t, base.SimHashHex, 3)
	decision, err := DecideContentFamily(ContentFamilyDecisionInput{
		DocumentVersionID:      21,
		Fingerprint:            near,
		Candidates:             []ContentFamilyCandidate{{FamilyID: 7, FamilyVersion: 3, RootDocumentVersionID: 10, Fingerprint: base}},
		DecisionProfileVersion: "content-family-decision-v1",
	})
	if err != nil || decision.Action != ContentFamilyActionJoin || decision.Relation != ContentRelationNearDuplicate || decision.HammingDistance != 3 || decision.MinHashSimilarity != 1 {
		t.Fatalf("near decision = %#v / %v", decision, err)
	}
}

func TestContentFamilyDecisionRecognizesARealLongFormNearDuplicate(t *testing.T) {
	t.Parallel()
	var paragraph strings.Builder
	for index := 0; index < 40; index++ {
		_, _ = fmt.Fprintf(&paragraph, "Section %03d documents PostgreSQL engineering change %03d and its migration evidence. ", index, index)
	}
	left, err := BuildContentFingerprint(paragraph.String()+"Original publication.", "content-fingerprint-v1")
	if err != nil {
		t.Fatal(err)
	}
	right, err := BuildContentFingerprint(paragraph.String()+"Updated publication.", "content-fingerprint-v1")
	if err != nil {
		t.Fatal(err)
	}
	decision, err := DecideContentFamily(ContentFamilyDecisionInput{DocumentVersionID: 32, Fingerprint: right,
		Candidates:             []ContentFamilyCandidate{{FamilyID: 9, FamilyVersion: 1, RootDocumentVersionID: 31, Fingerprint: left}},
		DecisionProfileVersion: "content-family-decision-v1"})
	if err != nil || decision.Action != ContentFamilyActionJoin || decision.Relation != ContentRelationNearDuplicate {
		t.Fatalf("real near-duplicate decision = %#v / %v", decision, err)
	}
}

func TestContentFamilyDecisionFailsClosedForHardConflictAndAmbiguity(t *testing.T) {
	t.Parallel()
	base, _ := BuildContentFingerprint("Alice announced version one in Beijing", "content-fingerprint-v1")
	modified := base
	modified.NormalizedContentSHA256 = strings.Repeat("c", 64)
	modified.SimHashHex = flipSimHashBits(t, base.SimHashHex, 5)
	candidate := ContentFamilyCandidate{FamilyID: 8, FamilyVersion: 2, RootDocumentVersionID: 11, Fingerprint: base}

	review, err := DecideContentFamily(ContentFamilyDecisionInput{
		DocumentVersionID: 22, Fingerprint: modified, Candidates: []ContentFamilyCandidate{candidate},
		DecisionProfileVersion: "content-family-decision-v1",
	})
	if err != nil || review.Action != ContentFamilyActionReview || review.Relation != ContentRelationNearDuplicate {
		t.Fatalf("ambiguous decision = %#v / %v", review, err)
	}

	conflict, err := DecideContentFamily(ContentFamilyDecisionInput{
		DocumentVersionID: 23, Fingerprint: base, Candidates: []ContentFamilyCandidate{candidate},
		DecisionProfileVersion: "content-family-decision-v1", HardConflict: true,
	})
	if err != nil || conflict.Action != ContentFamilyActionCreate || conflict.Relation != ContentRelationUnrelated || conflict.FamilyID != 0 {
		t.Fatalf("hard-conflict decision = %#v / %v", conflict, err)
	}
}

func TestContentLineageRelationsDeclareDirectionality(t *testing.T) {
	t.Parallel()
	for _, relation := range []ContentRelation{ContentRelationExactCopy, ContentRelationNearDuplicate} {
		if !relation.Valid() || !relation.Symmetric() {
			t.Fatalf("relation %q should be valid and symmetric", relation)
		}
	}
	for _, relation := range []ContentRelation{ContentRelationSyndicatedFrom, ContentRelationTranslationOf, ContentRelationRevisionOf} {
		if !relation.Valid() || relation.Symmetric() {
			t.Fatalf("relation %q should be valid and directed", relation)
		}
	}
	if ContentRelation("duplicate").Valid() {
		t.Fatal("unversioned duplicate relation was accepted")
	}
}
