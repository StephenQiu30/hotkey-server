package domain

import (
	"fmt"
	"time"
	"unicode/utf8"
)

type DocumentState string

const (
	DocumentStateActive     DocumentState = "active"
	DocumentStateWithdrawn  DocumentState = "withdrawn"
	DocumentStateTombstoned DocumentState = "tombstoned"
)

func (state DocumentState) Valid() bool {
	switch state {
	case DocumentStateActive, DocumentStateWithdrawn, DocumentStateTombstoned:
		return true
	default:
		return false
	}
}

// DocumentIdentity is the stable container identity resolved before
// body normalization. ExternalWorkID is nil when Source cannot prove that two
// observations represent the same published work; in that case DocumentKey is
// observation-scoped and prevents an unsafe merge.
type DocumentIdentity struct {
	SourceConnectionID int64
	DocumentKey        string
	ExternalWorkID     *string
}

func (identity DocumentIdentity) Validate() error {
	if identity.SourceConnectionID <= 0 || !validDocumentSHA256(identity.DocumentKey) {
		return fmt.Errorf("document identity is invalid")
	}
	if identity.ExternalWorkID == nil {
		return nil
	}
	externalWorkID := NormalizeExternalID(*identity.ExternalWorkID)
	if externalWorkID == "" || utf8.RuneCountInString(externalWorkID) > 512 || externalWorkID != *identity.ExternalWorkID {
		return fmt.Errorf("external work identity is invalid")
	}
	return nil
}

type Document struct {
	ID                   int64
	Version              int64
	SourceConnectionID   int64
	DocumentKey          string
	ExternalWorkID       *string
	CurrentVersionID     *int64
	State                DocumentState
	CreatedAt, UpdatedAt time.Time
}

type DocumentVersion struct {
	ID                             int64
	Version                        int64
	DocumentID                     int64
	SourceObservationID            int64
	RevisionNo                     int64
	VersionKey                     string
	BodyOrigin                     BodyOrigin
	Completeness                   BodyCompleteness
	WordCount                      int
	Language                       string
	Truncated                      bool
	QualityScore                   *float64
	QualityWarnings                []string
	ContentSHA256                  string
	ExtractorVersion               string
	ExtractorProfileVersion        string
	ExtractorProfileSHA256         string
	DisplayPrivateRightsDecisionID *int64
	LifecycleState                 DocumentLifecycleState
	CapturedAt                     time.Time
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

type DocumentVersionTransition struct {
	DocumentVersionID              int64
	ExpectedVersion                int64
	To                             DocumentLifecycleState
	DisplayPrivateRightsDecisionID *int64
}

func (transition DocumentVersionTransition) Validate() error {
	if transition.DocumentVersionID <= 0 || transition.ExpectedVersion <= 0 || !DocumentVersionLifecycleStateValid(transition.To) {
		return fmt.Errorf("document version lifecycle CAS is invalid")
	}
	if transition.To == DocumentReadable {
		if transition.DisplayPrivateRightsDecisionID == nil || *transition.DisplayPrivateRightsDecisionID <= 0 {
			return fmt.Errorf("readable document requires a display-private rights decision")
		}
		return nil
	}
	if transition.DisplayPrivateRightsDecisionID != nil {
		return fmt.Errorf("display-private rights decision is only valid for readable transition")
	}
	return nil
}

func DocumentVersionLifecycleStateValid(state DocumentLifecycleState) bool {
	switch state {
	case DocumentPolicyPending, DocumentPolicyBlocked, DocumentDerivedPending,
		DocumentDerivedAvailable, DocumentDerivedFailed, DocumentReadable,
		DocumentRetentionBlocked, DocumentQuarantined, DocumentTombstoned:
		return true
	default:
		return false
	}
}
