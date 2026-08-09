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
	Target                    string
	ServerVersion             int
	CurrentCatalogFingerprint string
	TargetSchemaSHA256        string
	CurrentTables             []string
	TargetTables              []string
	MissingTables             []string
	UnexpectedTables          []string
	Blockers                  []string
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
	schemaDigest := sha256.Sum256([]byte(canonicaldb.SchemaSQL))
	inspection := UpgradeInspection{
		Target: target, ServerVersion: serverVersion,
		CurrentCatalogFingerprint: fingerprint,
		TargetSchemaSHA256:        hex.EncodeToString(schemaDigest[:]),
		CurrentTables:             currentTables,
		TargetTables:              targetTables,
		MissingTables:             difference(targetTables, currentTables),
		UnexpectedTables:          difference(currentTables, targetTables),
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
