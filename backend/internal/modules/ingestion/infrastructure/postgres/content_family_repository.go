package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type ContentFamilyRepository struct{ runtime *database.Runtime }

var _ ingestionapplication.ContentFamilyRepository = (*ContentFamilyRepository)(nil)

func NewContentFamilyRepository(runtime *database.Runtime) (*ContentFamilyRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("content family database runtime is required")
	}
	return &ContentFamilyRepository{runtime: runtime}, nil
}

func (repository *ContentFamilyRepository) FindContentFamilyCandidates(ctx context.Context, query ingestionapplication.FindContentFamilyCandidatesQuery) ([]ingestionapplication.ContentFamilyCandidateDTO, error) {
	if repository == nil || repository.runtime == nil || query.DocumentVersionID <= 0 || query.Limit <= 0 || query.Limit > 100 ||
		!validContentFingerprintDTO(query.Fingerprint) || query.DecisionAt.IsZero() {
		return nil, ingestionapplication.ErrInvalidContentFamilyContract
	}
	rows, err := repository.queryExecutor(ctx).QueryContext(ctx, `
SELECT family.id,family.version,family.root_document_version_id,
       fingerprint.profile_version,btrim(fingerprint.normalized_content_sha256),btrim(fingerprint.simhash_hex),fingerprint.minhash
FROM content_family_members AS member
JOIN content_families AS family ON family.id=member.family_id AND family.status IN ('active','review_pending')
JOIN content_fingerprints AS fingerprint ON fingerprint.id=member.fingerprint_id
WHERE member.active AND member.document_version_id<>$1 AND fingerprint.profile_version=$2
  AND fingerprint.lifecycle_state='active' AND fingerprint.retention_until>$5
  AND current_rights_action_allowed(fingerprint.store_derived_rights_decision_id,fingerprint.source_connection_id,
      'document_version',fingerprint.document_version_id::text,(SELECT content_sha256 FROM document_versions WHERE id=fingerprint.document_version_id),'store_derived',$5)
  AND current_rights_action_allowed(fingerprint.retain_rights_decision_id,fingerprint.source_connection_id,
      'document_version',fingerprint.document_version_id::text,(SELECT content_sha256 FROM document_versions WHERE id=fingerprint.document_version_id),'retain',$5)
ORDER BY (fingerprint.normalized_content_sha256=$3) DESC,
         bit_count((('x'||btrim(fingerprint.simhash_hex))::bit(64)) # (('x'||$4)::bit(64))) ASC,
         family.id ASC
LIMIT $6`, query.DocumentVersionID, query.Fingerprint.ProfileVersion, query.Fingerprint.NormalizedContentSHA256,
		query.Fingerprint.SimHashHex, query.DecisionAt.UTC(), query.Limit)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	result := make([]ingestionapplication.ContentFamilyCandidateDTO, 0, query.Limit)
	for rows.Next() {
		var record contentFamilyCandidateRecord
		if err := rows.Scan(&record.familyID, &record.familyVersion, &record.rootDocumentVersionID,
			&record.profileVersion, &record.normalizedSHA256, &record.simHashHex, &record.minHashBytes); err != nil {
			return nil, databaserepository.MapError(err)
		}
		value, mapErr := record.dto()
		if mapErr != nil {
			return nil, fmt.Errorf("stored content family candidate: %w", mapErr)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return result, nil
}

func (repository *ContentFamilyRepository) CommitContentFamilyDecision(ctx context.Context, command ingestionapplication.CommitContentFamilyDecisionCommand) (ingestionapplication.ContentFamilyDecisionDTO, error) {
	if repository == nil || repository.runtime == nil || validateContentFamilyCommit(command) != nil {
		return ingestionapplication.ContentFamilyDecisionDTO{}, ingestionapplication.ErrInvalidContentFamilyContract
	}
	var result ingestionapplication.ContentFamilyDecisionDTO
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		replayed, found, replayErr := readContentFamilyDecision(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if replayErr != nil {
			return replayErr
		}
		if found {
			if replayed.commandFingerprint != command.CommandFingerprint {
				return sharedrepository.ErrConflict
			}
			result, replayErr = replayed.dto()
			result.Reused = true
			return replayErr
		}
		fingerprintID, fingerprintErr := persistContentFingerprint(transactionCtx, transaction.SQL, command)
		if fingerprintErr != nil {
			return fingerprintErr
		}
		familyID, familyVersion, rootDocumentVersionID, familyErr := resolveContentFamily(transactionCtx, transaction.SQL, command)
		if familyErr != nil {
			return familyErr
		}
		reasons, marshalErr := json.Marshal(command.ReasonCodes)
		if marshalErr != nil {
			return ingestionapplication.ErrInvalidContentFamilyContract
		}
		var decisionID int64
		var candidateRoot any
		if command.Action != "create" {
			candidateRoot = rootDocumentVersionID
		}
		if err := transaction.SQL.QueryRowContext(transactionCtx, `
INSERT INTO content_lineage_decisions (
  document_version_id,fingerprint_id,family_id,result_family_version,candidate_root_document_version_id,
  action,relation,hamming_distance,minhash_similarity,decision_profile_version,reason_codes,
  decision_origin,idempotency_key,command_fingerprint
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,'automatic',$12,$13)
RETURNING id`, command.DocumentVersionID, fingerprintID, familyID, familyVersion, candidateRoot,
			command.Action, command.Relation, command.HammingDistance, command.MinHashSimilarity,
			command.DecisionProfileVersion, string(reasons), command.IdempotencyKey, command.CommandFingerprint).Scan(&decisionID); err != nil {
			return databaserepository.MapError(err)
		}
		if command.Action != "review" {
			var parent any
			if command.Action == "join" {
				parent = rootDocumentVersionID
			}
			if _, err := transaction.SQL.ExecContext(transactionCtx, `
INSERT INTO content_family_members (
  family_id,document_version_id,fingerprint_id,lineage_decision_id,lineage_profile_version,relation,parent_document_version_id
) VALUES ($1,$2,$3,$4,$5,$6,$7)`, familyID, command.DocumentVersionID, fingerprintID, decisionID,
				command.DecisionProfileVersion, command.Relation, parent); err != nil {
				return databaserepository.MapError(err)
			}
		}
		stored, found, readErr := readContentFamilyDecision(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if readErr != nil || !found {
			return readErr
		}
		result, readErr = stored.dto()
		result.Reused = false
		return readErr
	})
	if err != nil {
		return ingestionapplication.ContentFamilyDecisionDTO{}, err
	}
	return result, nil
}

func persistContentFingerprint(ctx context.Context, executor *sql.Tx, command ingestionapplication.CommitContentFamilyDecisionCommand) (int64, error) {
	encoded, err := encodeContentMinHash(command.Fingerprint.MinHash)
	if err != nil {
		return 0, ingestionapplication.ErrInvalidContentFamilyContract
	}
	var id int64
	err = executor.QueryRowContext(ctx, `
INSERT INTO content_fingerprints (
  source_connection_id,document_version_id,derived_artifact_id,store_derived_rights_decision_id,retain_rights_decision_id,
  profile_version,normalized_content_sha256,simhash_hex,minhash,retention_until
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (document_version_id,profile_version) DO NOTHING RETURNING id`,
		command.SourceConnectionID, command.DocumentVersionID, command.DerivedArtifactID,
		command.StoreDerivedRightsDecisionID, command.RetainRightsDecisionID, command.Fingerprint.ProfileVersion,
		command.Fingerprint.NormalizedContentSHA256, command.Fingerprint.SimHashHex, encoded, command.RetentionUntil.UTC()).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, databaserepository.MapError(err)
	}
	var sourceConnectionID, artifactID, storeDecisionID, retainDecisionID int64
	var normalizedSHA, simHash string
	var minHash []byte
	var retentionUntil time.Time
	if err := executor.QueryRowContext(ctx, `SELECT id,source_connection_id,derived_artifact_id,store_derived_rights_decision_id,
retain_rights_decision_id,btrim(normalized_content_sha256),btrim(simhash_hex),minhash,retention_until
FROM content_fingerprints WHERE document_version_id=$1 AND profile_version=$2 AND lifecycle_state='active' FOR UPDATE`, command.DocumentVersionID,
		command.Fingerprint.ProfileVersion).Scan(&id, &sourceConnectionID, &artifactID, &storeDecisionID, &retainDecisionID,
		&normalizedSHA, &simHash, &minHash, &retentionUntil); err != nil {
		return 0, databaserepository.MapError(err)
	}
	if sourceConnectionID != command.SourceConnectionID || artifactID != command.DerivedArtifactID ||
		storeDecisionID != command.StoreDerivedRightsDecisionID || retainDecisionID != command.RetainRightsDecisionID ||
		normalizedSHA != command.Fingerprint.NormalizedContentSHA256 || simHash != command.Fingerprint.SimHashHex ||
		string(minHash) != string(encoded) || !retentionUntil.Equal(command.RetentionUntil) {
		return 0, sharedrepository.ErrConflict
	}
	return id, nil
}

func resolveContentFamily(ctx context.Context, executor *sql.Tx, command ingestionapplication.CommitContentFamilyDecisionCommand) (int64, int64, int64, error) {
	if command.Action == "create" {
		var id, version int64
		err := executor.QueryRowContext(ctx, `INSERT INTO content_families (root_document_version_id,lineage_profile_version)
VALUES ($1,$2) RETURNING id,version`, command.DocumentVersionID, command.DecisionProfileVersion).Scan(&id, &version)
		if err != nil {
			return 0, 0, 0, databaserepository.MapError(err)
		}
		return id, version, command.DocumentVersionID, nil
	}
	status := "active"
	if command.Action == "review" {
		status = "review_pending"
	}
	var version, root int64
	err := executor.QueryRowContext(ctx, `UPDATE content_families SET version=version+1,status=$3,updated_at=now()
WHERE id=$1 AND version=$2 AND status IN ('active','review_pending')
RETURNING version,root_document_version_id`, command.FamilyID, command.ExpectedFamilyVersion, status).Scan(&version, &root)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, 0, sharedrepository.ErrConflict
	}
	if err != nil {
		return 0, 0, 0, databaserepository.MapError(err)
	}
	if root != command.RootDocumentVersionID {
		return 0, 0, 0, sharedrepository.ErrConflict
	}
	return command.FamilyID, version, root, nil
}

func readContentFamilyDecision(ctx context.Context, executor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, idempotencyKey string) (contentFamilyDecisionRecord, bool, error) {
	var record contentFamilyDecisionRecord
	err := executor.QueryRowContext(ctx, `
SELECT decision.id,decision.family_id,decision.result_family_version,decision.document_version_id,
       family.root_document_version_id,decision.action,decision.relation,decision.hamming_distance,
       decision.minhash_similarity::float8,decision.decision_profile_version,decision.reason_codes,
       btrim(decision.command_fingerprint)
FROM content_lineage_decisions AS decision JOIN content_families AS family ON family.id=decision.family_id
WHERE decision.idempotency_key=$1`, idempotencyKey).Scan(&record.decisionID, &record.familyID, &record.familyVersion,
		&record.documentVersionID, &record.rootDocumentVersionID, &record.action, &record.relation,
		&record.hammingDistance, &record.minHashSimilarity, &record.decisionProfileVersion,
		&record.reasonCodesJSON, &record.commandFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return contentFamilyDecisionRecord{}, false, nil
	}
	if err != nil {
		return contentFamilyDecisionRecord{}, false, databaserepository.MapError(err)
	}
	return record, true, nil
}

func validateContentFamilyCommit(command ingestionapplication.CommitContentFamilyDecisionCommand) error {
	if command.SourceConnectionID <= 0 || command.DocumentVersionID <= 0 || command.DerivedArtifactID <= 0 ||
		command.StoreDerivedRightsDecisionID <= 0 || command.RetainRightsDecisionID <= 0 || command.DecisionAt.IsZero() ||
		!command.RetentionUntil.After(command.DecisionAt) || !validContentFingerprintDTO(command.Fingerprint) ||
		!validContentFamilyAction(command.Action) || !validContentRelation(command.Relation) ||
		command.HammingDistance < 0 || command.HammingDistance > 64 || math.IsNaN(command.MinHashSimilarity) ||
		math.IsInf(command.MinHashSimilarity, 0) || command.MinHashSimilarity < 0 || command.MinHashSimilarity > 1 ||
		strings.TrimSpace(command.DecisionProfileVersion) == "" || len(command.ReasonCodes) == 0 ||
		strings.TrimSpace(command.IdempotencyKey) == "" || !validContentFamilySHA(command.CommandFingerprint) {
		return ingestionapplication.ErrInvalidContentFamilyContract
	}
	if command.Action == "create" {
		if command.FamilyID != 0 || command.ExpectedFamilyVersion != 0 || command.RootDocumentVersionID != 0 || command.Relation != "unrelated" {
			return ingestionapplication.ErrInvalidContentFamilyContract
		}
	} else if command.FamilyID <= 0 || command.ExpectedFamilyVersion <= 0 || command.RootDocumentVersionID <= 0 || command.Relation == "unrelated" {
		return ingestionapplication.ErrInvalidContentFamilyContract
	}
	return nil
}

func validContentFingerprintDTO(value ingestionapplication.ContentFingerprintDTO) bool {
	return strings.TrimSpace(value.ProfileVersion) != "" && len(value.ProfileVersion) <= 64 &&
		validContentFamilySHA(value.NormalizedContentSHA256) && len(value.SimHashHex) == 16 &&
		strings.ToLower(value.SimHashHex) == value.SimHashHex && len(value.MinHash) == 64
}

func validContentFamilyAction(value string) bool {
	return value == "create" || value == "join" || value == "review"
}
func validContentRelation(value string) bool {
	switch value {
	case "exact_copy", "near_duplicate", "syndicated_from", "translation_of", "revision_of", "unrelated":
		return true
	default:
		return false
	}
}
func validContentFamilySHA(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

type contentFamilyQueryExecutor interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (repository *ContentFamilyRepository) queryExecutor(ctx context.Context) contentFamilyQueryExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}

func (repository *ContentFamilyRepository) withTransaction(ctx context.Context, operation func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return operation(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, operation)
}
