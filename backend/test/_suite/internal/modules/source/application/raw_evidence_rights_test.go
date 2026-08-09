package application

import (
	"strings"
	"testing"
	"time"
)

func TestCurrentRawEvidenceRightsQueryValidatesBoundedExactSubjects(t *testing.T) {
	t.Parallel()
	subject := RawEvidenceRightsSubjectDTO{EvidenceKey: strings.Repeat("a", 64), PayloadSHA256: strings.Repeat("b", 64)}
	query := CurrentRawEvidenceRightsQuery{SourceConnectionID: 7, DecisionAt: time.Now().UTC(), Subjects: []RawEvidenceRightsSubjectDTO{subject}}
	if err := query.Validate(); err != nil {
		t.Fatalf("valid rights query rejected: %v", err)
	}
	query.Subjects = append(query.Subjects, subject)
	if err := query.Validate(); err == nil {
		t.Fatal("duplicate raw evidence rights subject was accepted")
	}
	query.Subjects = []RawEvidenceRightsSubjectDTO{{EvidenceKey: strings.Repeat("A", 64), PayloadSHA256: strings.Repeat("b", 64)}}
	if err := query.Validate(); err == nil {
		t.Fatal("non-canonical evidence key was accepted")
	}
}
