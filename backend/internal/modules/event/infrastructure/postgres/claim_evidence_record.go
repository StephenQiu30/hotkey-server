package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
)

type claimRecord struct {
	id, version, microEventID, microEventVersion int64
	claimHash, subject, predicate, object        string
	qualifiersJSON                               []byte
	createdAt                                    time.Time
}

func (record claimRecord) dto() (eventapplication.ClaimDTO, error) {
	qualifiers := []eventapplication.ClaimQualifierDTO{}
	if err := json.Unmarshal(record.qualifiersJSON, &qualifiers); err != nil {
		return eventapplication.ClaimDTO{}, fmt.Errorf("invalid claim qualifiers: %w", err)
	}
	return eventapplication.ClaimDTO{ID: record.id, Version: record.version, MicroEventID: record.microEventID,
		MicroEventVersion: record.microEventVersion, ClaimHash: strings.TrimSpace(record.claimHash), Subject: record.subject,
		Predicate: record.predicate, Object: record.object, Qualifiers: qualifiers, CreatedAt: record.createdAt.UTC()}, nil
}

type claimEvidenceVersionRecord struct {
	id, version, claimID, documentVersionID, selectorID, familyID, lineageRootID int64
	relation, quoteSHA, plaintextSHA, selectorVersion, extractionVersion, origin string
	sourceRecordURL, canonicalURL, publisherName, contentOriginName              sql.NullString
	publisherPartyID, contentOriginPartyID, modelRunID, actorUserID              sql.NullInt64
	modelRelationScore                                                           sql.NullFloat64
	publishedAt                                                                  sql.NullTime
	capturedAt, retentionUntil, createdAt                                        time.Time
}

func (record claimEvidenceVersionRecord) dto() eventapplication.ClaimEvidenceVersionDTO {
	return eventapplication.ClaimEvidenceVersionDTO{ID: record.id, Version: record.version, ClaimID: record.claimID,
		DocumentVersionID: record.documentVersionID, TextQuoteSelectorID: record.selectorID,
		ContentFamilyID: record.familyID, LineageRootID: record.lineageRootID, Relation: record.relation,
		ExtractionSchemaVersion: record.extractionVersion, Origin: record.origin,
		ModelRunID: nullableClaimEvidenceInt64(record.modelRunID), ActorUserID: nullableClaimEvidenceInt64(record.actorUserID),
		ModelRelationScore: nullableClaimEvidenceFloat64(record.modelRelationScore), QuoteSHA256: strings.TrimSpace(record.quoteSHA),
		PlaintextSHA256: strings.TrimSpace(record.plaintextSHA), SelectorVersion: record.selectorVersion,
		SourceRecordURL: nullableClaimEvidenceString(record.sourceRecordURL), CanonicalURL: nullableClaimEvidenceString(record.canonicalURL),
		PublisherPartyID: nullableClaimEvidenceInt64(record.publisherPartyID), PublisherName: nullableClaimEvidenceString(record.publisherName),
		ContentOriginPartyID: nullableClaimEvidenceInt64(record.contentOriginPartyID), ContentOriginName: nullableClaimEvidenceString(record.contentOriginName),
		PublishedAt: nullableClaimEvidenceTime(record.publishedAt), CapturedAt: record.capturedAt.UTC(),
		RetentionUntil: record.retentionUntil.UTC(), CreatedAt: record.createdAt.UTC()}
}

type evidenceStateSnapshotRecord struct {
	id, version, microEventID, eventVersion, profileID int64
	algorithmVersion, evidenceSetHash, state           string
	independentOrigins                                 int
	reasonCodesJSON                                    []byte
	calculatedAt                                       time.Time
}

func (record evidenceStateSnapshotRecord) dto(itemIDs []int64, created bool) (eventapplication.EvidenceStateSnapshotDTO, error) {
	reasons := []string{}
	if err := json.Unmarshal(record.reasonCodesJSON, &reasons); err != nil || len(reasons) == 0 {
		return eventapplication.EvidenceStateSnapshotDTO{}, fmt.Errorf("invalid evidence state reasons")
	}
	return eventapplication.EvidenceStateSnapshotDTO{ID: record.id, Version: record.version,
		MicroEventID: record.microEventID, EventVersion: record.eventVersion, ProfileID: record.profileID,
		AlgorithmVersion: record.algorithmVersion, EvidenceSetHash: strings.TrimSpace(record.evidenceSetHash),
		State: record.state, IndependentOriginCount: record.independentOrigins, ReasonCodes: reasons,
		ClaimEvidenceVersionIDs: append([]int64(nil), itemIDs...), CalculatedAt: record.calculatedAt.UTC(), Created: created}, nil
}

func nullableClaimEvidenceString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
func nullableClaimEvidenceInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
func nullableClaimEvidenceFloat64(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}
func nullableClaimEvidenceTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
