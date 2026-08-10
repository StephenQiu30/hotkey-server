package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	canonicaldb "github.com/StephenQiu30/hotkey-server/backend/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	CanonicalUpgradeTarget = "032"
	schemaUpgradeLock      = "hotkey-schema-upgrade-evidence-lineage-v1"
)

// UpgradeInspection is a read-only, credential-free summary suitable for a
// guarded dry run. It never contains row values, DSNs or object locations.
type UpgradeInspection struct {
	Target                       string
	ServerVersion                int
	CurrentCatalogFingerprint    string
	TargetSchemaSHA256           string
	CurrentTables                []string
	TargetTables                 []string
	MissingTables                []string
	UnexpectedTables             []string
	MissingIndexCount            int
	EstimatedRows                int64
	CurrentTableBytes            int64
	CurrentIndexBytes            int64
	EstimatedIndexWorkspaceBytes int64
	IndexEstimateVersion         string
	LockRisk                     string
	Extensions                   []UpgradeExtension
	Blockers                     []string
}

type UpgradeExtension struct {
	Name    string
	Version string
}

func (inspection UpgradeInspection) CanApply() bool { return len(inspection.Blockers) == 0 }

func InspectCanonicalUpgrade(ctx context.Context, pool *pgxpool.Pool, target string) (UpgradeInspection, error) {
	if pool == nil {
		return UpgradeInspection{}, errors.New("database pool is required")
	}
	if target != CanonicalUpgradeTarget {
		return UpgradeInspection{}, fmt.Errorf("unsupported canonical upgrade target %q", target)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return UpgradeInspection{}, fmt.Errorf("begin canonical upgrade inspection: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	inspection, err := inspectCanonicalUpgradeTransaction(ctx, tx, target)
	if err != nil {
		return UpgradeInspection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UpgradeInspection{}, fmt.Errorf("complete canonical upgrade inspection: %w", err)
	}
	return inspection, nil
}

// ApplyCanonicalUpgrade applies only the embedded canonical Schema. The
// catalog is verified inside the same advisory-locked transaction, so a
// failed constraint, preflight or fingerprint check cannot leave a partial
// target catalog visible to the application.
func ApplyCanonicalUpgrade(ctx context.Context, pool *pgxpool.Pool, target string) (Verification, error) {
	if pool == nil {
		return Verification{}, errors.New("database pool is required")
	}
	if target != CanonicalUpgradeTarget {
		return Verification{}, fmt.Errorf("unsupported canonical upgrade target %q", target)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Verification{}, fmt.Errorf("begin canonical schema upgrade: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", schemaUpgradeLock); err != nil {
		return Verification{}, fmt.Errorf("lock canonical schema upgrade: %w", err)
	}
	inspection, err := inspectCanonicalUpgradeTransaction(ctx, tx, target)
	if err != nil {
		return Verification{}, err
	}
	if !inspection.CanApply() {
		return Verification{}, fmt.Errorf("canonical schema upgrade blocked: %v", inspection.Blockers)
	}
	if _, err := tx.Exec(ctx, canonicaldb.SchemaSQL); err != nil {
		return Verification{}, fmt.Errorf("apply embedded canonical schema for target %s: %w", target, err)
	}
	verification, err := verifyCanonicalTransaction(ctx, tx)
	if err != nil {
		return Verification{}, fmt.Errorf("verify canonical schema upgrade target %s: %w", target, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Verification{}, fmt.Errorf("commit canonical schema upgrade target %s: %w", target, err)
	}
	return verification, nil
}

func inspectCanonicalUpgradeTransaction(ctx context.Context, tx pgx.Tx, target string) (UpgradeInspection, error) {
	var serverVersion int
	if err := tx.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&serverVersion); err != nil {
		return UpgradeInspection{}, fmt.Errorf("read PostgreSQL version: %w", err)
	}
	expectedContract, err := canonicalCatalogContract()
	if err != nil {
		return UpgradeInspection{}, fmt.Errorf("derive embedded schema contract: %w", err)
	}
	targetTables := expectedContract.TableNames()
	currentTables, err := publicTableNames(ctx, tx)
	if err != nil {
		return UpgradeInspection{}, err
	}
	fingerprint, err := catalogFingerprint(ctx, tx)
	if err != nil {
		return UpgradeInspection{}, err
	}
	actualContract, err := databaseCatalogContract(ctx, tx, targetTables)
	if err != nil {
		return UpgradeInspection{}, fmt.Errorf("read current canonical contract: %w", err)
	}
	var estimatedRows float64
	var currentTableBytes, currentIndexBytes int64
	if err := tx.QueryRow(ctx, `
SELECT COALESCE(sum(GREATEST(relation.reltuples,0)),0),
       COALESCE(sum(pg_relation_size(relation.oid)),0),
       COALESCE(sum(pg_indexes_size(relation.oid)),0)
FROM pg_class AS relation
JOIN pg_namespace AS namespace ON namespace.oid=relation.relnamespace
WHERE namespace.nspname='public' AND relation.relkind='r'`).Scan(&estimatedRows, &currentTableBytes, &currentIndexBytes); err != nil {
		return UpgradeInspection{}, fmt.Errorf("estimate canonical upgrade storage: %w", err)
	}
	extensions, err := canonicalUpgradeExtensions(ctx, tx)
	if err != nil {
		return UpgradeInspection{}, err
	}
	schemaDigest := sha256.Sum256([]byte(canonicaldb.SchemaSQL))
	inspection := UpgradeInspection{
		Target: target, ServerVersion: serverVersion,
		CurrentCatalogFingerprint:    fingerprint,
		TargetSchemaSHA256:           hex.EncodeToString(schemaDigest[:]),
		CurrentTables:                currentTables,
		TargetTables:                 targetTables,
		MissingTables:                difference(targetTables, currentTables),
		UnexpectedTables:             difference(currentTables, targetTables),
		MissingIndexCount:            len(difference(expectedContract.Indexes, actualContract.Indexes)),
		EstimatedRows:                int64(estimatedRows),
		CurrentTableBytes:            currentTableBytes,
		CurrentIndexBytes:            currentIndexBytes,
		EstimatedIndexWorkspaceBytes: currentIndexBytes + currentTableBytes/4,
		IndexEstimateVersion:         "postgres-index-workspace-v1",
		LockRisk:                     "access_exclusive_transactional",
		Extensions:                   extensions,
	}
	if serverVersion < 160000 {
		inspection.Blockers = append(inspection.Blockers, "postgresql_below_16")
	}
	if len(currentTables) == 0 {
		inspection.Blockers = append(inspection.Blockers, "public_schema_empty_use_db_init")
	}
	if len(inspection.UnexpectedTables) != 0 {
		inspection.Blockers = append(inspection.Blockers, "unexpected_public_tables")
	}
	for _, required := range []string{"users", "source_connections", "contents"} {
		if !slices.Contains(currentTables, required) {
			inspection.Blockers = append(inspection.Blockers, "missing_baseline_"+required)
		}
	}
	slices.Sort(inspection.Blockers)
	return inspection, nil
}

func canonicalUpgradeExtensions(ctx context.Context, tx pgx.Tx) ([]UpgradeExtension, error) {
	rows, err := tx.Query(ctx, `
SELECT extname,extversion FROM pg_extension
WHERE extname IN ('pg_trgm','vector') ORDER BY extname`)
	if err != nil {
		return nil, fmt.Errorf("read canonical upgrade extensions: %w", err)
	}
	defer rows.Close()
	extensions := make([]UpgradeExtension, 0, 2)
	for rows.Next() {
		var extension UpgradeExtension
		if err := rows.Scan(&extension.Name, &extension.Version); err != nil {
			return nil, fmt.Errorf("scan canonical upgrade extension: %w", err)
		}
		extensions = append(extensions, extension)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate canonical upgrade extensions: %w", err)
	}
	return extensions, nil
}
