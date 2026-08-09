package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

type rightsDecisionRecord struct {
	ID                   int64
	SourceConnectionID   int64
	PolicyID             int64
	PolicyRevision       int64
	PolicyScopeType      string
	PolicyScopeSubject   string
	PriorityRank         int16
	BasisSummary         string
	TermsURL             sql.NullString
	LicenseURI           sql.NullString
	SubjectType          string
	SubjectKey           string
	InputDigest          string
	Action               string
	Decision             string
	ReasonCodesJSON      []byte
	Evaluator            string
	EvaluatedAt          time.Time
	EffectiveFrom        time.Time
	ExpiresAt            sql.NullTime
	RetentionDays        sql.NullInt64
	SupersedesDecisionID sql.NullInt64
}

type rightsDecisionScanner interface {
	Scan(...any) error
}

func scanRightsDecisionRecord(scanner rightsDecisionScanner) (rightsDecisionRecord, error) {
	var record rightsDecisionRecord
	err := scanner.Scan(
		&record.ID, &record.SourceConnectionID, &record.PolicyID, &record.PolicyRevision,
		&record.PolicyScopeType, &record.PolicyScopeSubject, &record.PriorityRank,
		&record.BasisSummary, &record.TermsURL, &record.LicenseURI,
		&record.SubjectType, &record.SubjectKey, &record.InputDigest,
		&record.Action, &record.Decision, &record.ReasonCodesJSON, &record.Evaluator,
		&record.EvaluatedAt, &record.EffectiveFrom, &record.ExpiresAt,
		&record.RetentionDays, &record.SupersedesDecisionID,
	)
	return record, err
}

func (record rightsDecisionRecord) entity() (domain.RightsDecision, error) {
	reasonCodes := make([]string, 0)
	if err := json.Unmarshal(record.ReasonCodesJSON, &reasonCodes); err != nil {
		return domain.RightsDecision{}, fmt.Errorf("decode rights decision reason codes: %w", err)
	}
	decision := domain.RightsDecision{
		ID: record.ID, SourceConnectionID: record.SourceConnectionID,
		Scope:    domain.RightsScope{Type: domain.RightsScopeType(record.PolicyScopeType), SubjectID: record.PolicyScopeSubject},
		PolicyID: record.PolicyID, PolicyRevision: record.PolicyRevision, Priority: domain.RightsPriority(record.PriorityRank),
		Basis:       domain.RightsBasis{Summary: record.BasisSummary, TermsURL: record.TermsURL.String, LicenseURI: record.LicenseURI.String},
		SubjectType: domain.RightsSubjectType(record.SubjectType), SubjectKey: strings.TrimSpace(record.SubjectKey),
		InputDigest: strings.TrimSpace(record.InputDigest), Action: domain.RightsAction(record.Action), Decision: domain.RightsState(record.Decision),
		ReasonCodes: reasonCodes, Evaluator: record.Evaluator, EvaluatedAt: record.EvaluatedAt.UTC(), EffectiveFrom: record.EffectiveFrom.UTC(),
	}
	if record.ExpiresAt.Valid {
		value := record.ExpiresAt.Time.UTC()
		decision.ExpiresAt = &value
	}
	if record.RetentionDays.Valid {
		if record.RetentionDays.Int64 < 1 || record.RetentionDays.Int64 > 3650 {
			return domain.RightsDecision{}, fmt.Errorf("rights decision retention duration is invalid")
		}
		value := int(record.RetentionDays.Int64)
		decision.RetentionDays = &value
	}
	if record.SupersedesDecisionID.Valid {
		value := record.SupersedesDecisionID.Int64
		decision.SupersedesDecisionID = &value
	}
	if err := decision.Validate(); err != nil {
		return domain.RightsDecision{}, fmt.Errorf("map rights decision entity: %w", err)
	}
	return decision, nil
}

func rawEvidenceRightsDecisionDTO(entity domain.RightsDecision) sourceapplication.RawEvidenceRightsDecisionDTO {
	return sourceapplication.RawEvidenceRightsDecisionDTO{
		ID: entity.ID, PolicyID: entity.PolicyID, PolicyRevision: entity.PolicyRevision,
		SourceConnectionID: entity.SourceConnectionID, SubjectType: string(entity.SubjectType), SubjectKey: entity.SubjectKey,
		InputDigest: entity.InputDigest, Action: string(entity.Action), Decision: string(entity.Decision),
		EffectiveFrom: entity.EffectiveFrom, ExpiresAt: copyTimePointer(entity.ExpiresAt), RetentionDays: copyIntPointer(entity.RetentionDays),
		SupersedesDecisionID: copyInt64Pointer(entity.SupersedesDecisionID),
	}
}

func copyTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func copyIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
