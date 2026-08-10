package postgres

import (
	"context"
	"database/sql"
	"errors"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
)

type sourcePartyRecord struct {
	ID                 int64
	SourceConnectionID int64
	Kind               string
	IdentityNamespace  string
	ExternalID         string
}

const sourcePartyColumns = `id,source_connection_id,party_kind,identity_namespace,external_id`

func scanSourcePartyRecord(scanner evidenceSnapshotScanner) (sourcePartyRecord, error) {
	var record sourcePartyRecord
	err := scanner.Scan(&record.ID, &record.SourceConnectionID, &record.Kind, &record.IdentityNamespace, &record.ExternalID)
	return record, err
}

type sourceObservationPartyRecord struct {
	ID                  int64
	SourceConnectionID  int64
	SourceObservationID int64
	SourcePartyID       int64
	EvidenceReferenceID int64
	Role                string
	DisplayNameSnapshot string
	HomepageURLSnapshot sql.NullString
}

const sourceObservationPartyColumns = `
id,source_connection_id,source_observation_id,source_party_id,evidence_reference_id,
role,display_name_snapshot,homepage_url_snapshot`

func scanSourceObservationPartyRecord(scanner evidenceSnapshotScanner) (sourceObservationPartyRecord, error) {
	var record sourceObservationPartyRecord
	err := scanner.Scan(
		&record.ID, &record.SourceConnectionID, &record.SourceObservationID, &record.SourcePartyID,
		&record.EvidenceReferenceID, &record.Role, &record.DisplayNameSnapshot, &record.HomepageURLSnapshot,
	)
	return record, err
}

func appendObservationParties(
	ctx context.Context,
	executor evidenceSnapshotExecutor,
	sourceConnectionID, observationID, evidenceReferenceID int64,
	parties []sourceapplication.SourceObservationPartyDTO,
) error {
	for _, party := range parties {
		storedParty, err := scanSourcePartyRecord(executor.QueryRowContext(ctx, `
INSERT INTO source_parties (source_connection_id,party_kind,identity_namespace,external_id)
VALUES ($1,$2,$3,$4)
ON CONFLICT (source_connection_id,identity_namespace,external_id) DO NOTHING
RETURNING `+sourcePartyColumns,
			sourceConnectionID, party.Kind, party.IdentityNamespace, party.ExternalID))
		if errors.Is(err, sql.ErrNoRows) {
			storedParty, err = scanSourcePartyRecord(executor.QueryRowContext(ctx, `
SELECT `+sourcePartyColumns+`
FROM source_parties
WHERE source_connection_id=$1 AND identity_namespace=$2 AND external_id=$3
FOR KEY SHARE`, sourceConnectionID, party.IdentityNamespace, party.ExternalID))
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		if storedParty.SourceConnectionID != sourceConnectionID || storedParty.Kind != party.Kind ||
			storedParty.IdentityNamespace != party.IdentityNamespace || storedParty.ExternalID != party.ExternalID {
			return evidenceConflict("source party identity has different immutable facts")
		}

		storedRelation, err := scanSourceObservationPartyRecord(executor.QueryRowContext(ctx, `
INSERT INTO source_observation_parties (
  source_connection_id,source_observation_id,source_party_id,evidence_reference_id,
  role,display_name_snapshot,homepage_url_snapshot
) VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (source_observation_id,role,source_party_id) DO NOTHING
RETURNING `+sourceObservationPartyColumns,
			sourceConnectionID, observationID, storedParty.ID, evidenceReferenceID, party.Role,
			party.DisplayName, nullString(party.HomepageURL)))
		if errors.Is(err, sql.ErrNoRows) {
			storedRelation, err = scanSourceObservationPartyRecord(executor.QueryRowContext(ctx, `
SELECT `+sourceObservationPartyColumns+`
FROM source_observation_parties
WHERE source_observation_id=$1 AND role=$2 AND source_party_id=$3
FOR KEY SHARE`, observationID, party.Role, storedParty.ID))
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		if storedRelation.SourceConnectionID != sourceConnectionID || storedRelation.SourceObservationID != observationID ||
			storedRelation.SourcePartyID != storedParty.ID || storedRelation.Role != party.Role ||
			storedRelation.DisplayNameSnapshot != party.DisplayName || !nullStringEquals(storedRelation.HomepageURLSnapshot, party.HomepageURL) {
			return evidenceConflict("source observation party has different immutable facts")
		}
	}
	return verifyObservationParties(ctx, executor, sourceConnectionID, observationID, parties)
}

func verifyObservationParties(
	ctx context.Context,
	executor evidenceSnapshotExecutor,
	sourceConnectionID, observationID int64,
	parties []sourceapplication.SourceObservationPartyDTO,
) error {
	return verifyObservationPartyRows(ctx, executor, sourceConnectionID, observationID, parties)
}

func verifyObservationPartyRows(
	ctx context.Context,
	executor evidenceSnapshotExecutor,
	sourceConnectionID, observationID int64,
	parties []sourceapplication.SourceObservationPartyDTO,
) error {
	rows, err := queryObservationPartyDTOs(ctx, executor, sourceConnectionID, observationID)
	if err != nil {
		return err
	}
	if !sameSourceObservationPartyDTOs(rows, parties) {
		return evidenceConflict("source observation has a different explicit party set")
	}
	return nil
}

func queryObservationPartyDTOs(ctx context.Context, executor evidenceSnapshotExecutor, sourceConnectionID, observationID int64) ([]sourceapplication.SourceObservationPartyDTO, error) {
	rows, err := executor.QueryContext(ctx, `
SELECT relation.role,party.party_kind,party.identity_namespace,party.external_id,
       relation.display_name_snapshot,COALESCE(relation.homepage_url_snapshot,'')
FROM source_observation_parties AS relation
JOIN source_parties AS party
  ON party.id=relation.source_party_id AND party.source_connection_id=relation.source_connection_id
WHERE relation.source_connection_id=$1 AND relation.source_observation_id=$2
ORDER BY CASE relation.role WHEN 'publisher' THEN 0 WHEN 'content_origin' THEN 1 WHEN 'distributor' THEN 2 ELSE 3 END,
         party.identity_namespace,party.external_id`, sourceConnectionID, observationID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	result := make([]sourceapplication.SourceObservationPartyDTO, 0)
	for rows.Next() {
		var value sourceapplication.SourceObservationPartyDTO
		if err := rows.Scan(&value.Role, &value.Kind, &value.IdentityNamespace, &value.ExternalID, &value.DisplayName, &value.HomepageURL); err != nil {
			return nil, databaserepository.MapError(err)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return result, nil
}

func sameSourceObservationPartyDTOs(left, right []sourceapplication.SourceObservationPartyDTO) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
