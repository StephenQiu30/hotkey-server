package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	knowledgedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	platformdatabase "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	candidateRPO = 15 * time.Minute
	candidateRTO = 2 * time.Hour
	toolVersion  = "hotkey-joint-recovery-v1"
)

type config struct {
	DSN, MinIOEndpoint, MinIOAccessKey, MinIOSecretKey, MinIOBucket string
	Output, Environment, Hardware, GitRevision                      string
	MinIOUseSSL, ProductionEgressDisabled                           bool
}

type report struct {
	Version                   string        `json:"version"`
	Status                    string        `json:"status"`
	GitRevision               string        `json:"git_revision"`
	Environment               string        `json:"environment"`
	Hardware                  string        `json:"hardware"`
	Runtime                   runtimeFacts  `json:"runtime"`
	Isolated                  bool          `json:"isolated"`
	ProductionEgressDisabled  bool          `json:"production_egress_disabled"`
	RecoveryPointAt           time.Time     `json:"recovery_point_at"`
	IncidentCutoffAt          time.Time     `json:"incident_cutoff_at"`
	DrillStartedAt            time.Time     `json:"drill_started_at"`
	ServicesReadableAt        time.Time     `json:"services_readable_at"`
	ReconciliationCompletedAt time.Time     `json:"reconciliation_completed_at"`
	RPOMillis                 int64         `json:"rpo_millis"`
	RTOMillis                 int64         `json:"rto_millis"`
	CandidateRPOMet           bool          `json:"candidate_rpo_met"`
	CandidateRTOMet           bool          `json:"candidate_rto_met"`
	Assets                    []assetResult `json:"assets"`
	Differences               []string      `json:"differences"`
	Exclusions                []string      `json:"exclusions"`
}

type runtimeFacts struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	CPUs   int    `json:"cpus"`
}

type assetResult struct {
	Name                   string `json:"name"`
	ExpectedCount          int64  `json:"expected_count"`
	ActualCount            int64  `json:"actual_count"`
	ExpectedSHA256         string `json:"expected_sha256"`
	ActualSHA256           string `json:"actual_sha256"`
	ExpectedVersionedCount int64  `json:"expected_versioned_count,omitempty"`
	ActualVersionedCount   int64  `json:"actual_versioned_count,omitempty"`
}

type inventory struct {
	Count, VersionedCount int64
	SHA256                string
}

type objectBackup struct {
	Key, ContentType, SourceVersionID string
	Body                              []byte
	UserMetadata                      map[string]string
}

type drill struct {
	cfg                                    config
	workRoot                               string
	sourceDSN, restoreDSN                  string
	sourceDatabase, restoreDatabase        string
	minioClient                            *minio.Client
	sourceBucket, restoreBucket            string
	backupObjects                          []objectBackup
	sourceVault, backupVault, restoreVault string
	expected                               map[string]inventory
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(parent context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()
	workRoot, err := os.MkdirTemp("", "hotkey-joint-recovery-")
	if err != nil {
		return errors.New("create isolated recovery workspace")
	}
	defer os.RemoveAll(workRoot)

	recovery := &drill{cfg: cfg, workRoot: workRoot, expected: make(map[string]inventory)}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		recovery.cleanup(cleanupCtx)
	}()
	if err := recovery.prepare(ctx); err != nil {
		return err
	}

	if err := recovery.createSourceFixture(ctx); err != nil {
		return err
	}
	recoveryPointAt := time.Now().UTC()
	if err := recovery.backup(ctx); err != nil {
		return err
	}
	incidentCutoffAt := time.Now().UTC()
	drillStartedAt := incidentCutoffAt
	if err := recovery.restore(ctx); err != nil {
		return err
	}
	servicesReadableAt := time.Now().UTC()
	assets, differences, err := recovery.reconcile(ctx)
	if err != nil {
		return err
	}
	reconciliationCompletedAt := time.Now().UTC()
	rpo := incidentCutoffAt.Sub(recoveryPointAt)
	rto := servicesReadableAt.Sub(drillStartedAt)
	result := report{
		Version: "hotkey-joint-recovery-v1", Status: "reconciled", GitRevision: cfg.GitRevision,
		Environment: cfg.Environment, Hardware: cfg.Hardware,
		Runtime:  runtimeFacts{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, CPUs: runtime.NumCPU()},
		Isolated: true, ProductionEgressDisabled: cfg.ProductionEgressDisabled,
		RecoveryPointAt: recoveryPointAt, IncidentCutoffAt: incidentCutoffAt,
		DrillStartedAt: drillStartedAt, ServicesReadableAt: servicesReadableAt,
		ReconciliationCompletedAt: reconciliationCompletedAt,
		RPOMillis:                 rpo.Milliseconds(), RTOMillis: rto.Milliseconds(),
		CandidateRPOMet: rpo <= candidateRPO, CandidateRTOMet: rto <= candidateRTO,
		Assets: assets, Differences: differences,
		Exclusions: []string{"production_traffic", "external_connectors", "notification_delivery", "redis_ephemeral_state"},
	}
	if len(differences) != 0 {
		result.Status = "failed"
	}
	if err := writeExclusiveJSON(cfg.Output, result); err != nil {
		return err
	}
	if len(differences) != 0 {
		return fmt.Errorf("joint recovery completed with %d unexplained differences", len(differences))
	}
	fmt.Printf("joint recovery evidence written to %s (RPO=%dms, RTO=%dms)\n", cfg.Output, result.RPOMillis, result.RTOMillis)
	return nil
}

func loadConfig() (config, error) {
	result := config{
		DSN:                      strings.TrimSpace(os.Getenv("HOTKEY_TEST_DSN")),
		MinIOEndpoint:            strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_ENDPOINT")),
		MinIOAccessKey:           strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_ACCESS_KEY")),
		MinIOSecretKey:           strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_SECRET_KEY")),
		MinIOBucket:              strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_BUCKET")),
		Output:                   strings.TrimSpace(os.Getenv("HOTKEY_JOINT_RECOVERY_OUTPUT")),
		Environment:              strings.TrimSpace(os.Getenv("HOTKEY_JOINT_RECOVERY_ENVIRONMENT")),
		Hardware:                 strings.TrimSpace(os.Getenv("HOTKEY_JOINT_RECOVERY_HARDWARE")),
		GitRevision:              strings.TrimSpace(os.Getenv("HOTKEY_JOINT_RECOVERY_GIT_REVISION")),
		MinIOUseSSL:              strings.EqualFold(strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_USE_SSL")), "true"),
		ProductionEgressDisabled: strings.EqualFold(strings.TrimSpace(os.Getenv("HOTKEY_JOINT_RECOVERY_PRODUCTION_EGRESS_DISABLED")), "true"),
	}
	if result.DSN == "" || result.MinIOEndpoint == "" || result.MinIOAccessKey == "" || result.MinIOSecretKey == "" || result.MinIOBucket == "" ||
		result.Output == "" || result.Environment == "" || result.Hardware == "" {
		return config{}, errors.New("joint recovery database, MinIO, output, environment and hardware metadata are required")
	}
	if len(result.GitRevision) != 40 || strings.Trim(result.GitRevision, "0123456789abcdef") != "" {
		return config{}, errors.New("HOTKEY_JOINT_RECOVERY_GIT_REVISION must be a 40-character lowercase commit SHA")
	}
	if !result.ProductionEgressDisabled {
		return config{}, errors.New("joint recovery requires HOTKEY_JOINT_RECOVERY_PRODUCTION_EGRESS_DISABLED=true")
	}
	if _, err := databaseDSN(result.DSN, "hotkey_joint_contract"); err != nil {
		return config{}, err
	}
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return config{}, errors.New("pg_dump is required for joint recovery")
	}
	if _, err := exec.LookPath("pg_restore"); err != nil {
		return config{}, errors.New("pg_restore is required for joint recovery")
	}
	return result, nil
}

func (recovery *drill) prepare(ctx context.Context) error {
	runID, err := randomIdentifier()
	if err != nil {
		return err
	}
	recovery.sourceDatabase = "hotkey_joint_source_" + runID
	recovery.restoreDatabase = "hotkey_joint_restore_" + runID
	if recovery.sourceDSN, err = databaseDSN(recovery.cfg.DSN, recovery.sourceDatabase); err != nil {
		return err
	}
	if recovery.restoreDSN, err = databaseDSN(recovery.cfg.DSN, recovery.restoreDatabase); err != nil {
		return err
	}
	if err := createDatabase(ctx, recovery.cfg.DSN, recovery.sourceDatabase); err != nil {
		return err
	}
	if err := createDatabase(ctx, recovery.cfg.DSN, recovery.restoreDatabase); err != nil {
		return err
	}
	client, err := minio.New(recovery.cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(recovery.cfg.MinIOAccessKey, recovery.cfg.MinIOSecretKey, ""),
		Secure: recovery.cfg.MinIOUseSSL, Region: "us-east-1", BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return errors.New("create isolated recovery MinIO client")
	}
	recovery.minioClient = client
	recovery.sourceBucket = recoveryBucketName(recovery.cfg.MinIOBucket, "source", runID)
	recovery.restoreBucket = recoveryBucketName(recovery.cfg.MinIOBucket, "restore", runID)
	for _, bucket := range []string{recovery.sourceBucket, recovery.restoreBucket} {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
			return errors.New("create isolated recovery MinIO bucket")
		}
		if err := client.SetBucketVersioning(ctx, bucket, minio.BucketVersioningConfiguration{Status: "Enabled"}); err != nil {
			return errors.New("enable isolated recovery MinIO versioning")
		}
	}
	recovery.sourceVault = filepath.Join(recovery.workRoot, "vault-source")
	recovery.backupVault = filepath.Join(recovery.workRoot, "backup", "vault")
	recovery.restoreVault = filepath.Join(recovery.workRoot, "vault-restored")
	return nil
}

func (recovery *drill) createSourceFixture(ctx context.Context) error {
	runtimeDB, err := platformdatabase.Open(ctx, recovery.sourceDSN)
	if err != nil {
		return errors.New("open isolated source PostgreSQL")
	}
	defer runtimeDB.Close()
	if err := platformdatabase.InitializeEmpty(ctx, runtimeDB.Pool); err != nil {
		return errors.New("initialize isolated source PostgreSQL")
	}
	if _, err := runtimeDB.SQL.ExecContext(ctx, `
INSERT INTO source_connections (source_type,name,endpoint)
VALUES ('rss','joint-recovery-source','https://fixture.invalid/recovery');
INSERT INTO contents (source_connection_id,external_id,content_type,canonical_url,published_at,fetched_at,dedupe_key)
SELECT id,'joint-recovery-content','article','https://fixture.invalid/recovery/1',
       '2026-08-29T00:00:00Z'::timestamptz,'2026-08-29T00:01:00Z'::timestamptz,
       repeat('a',64)
FROM source_connections WHERE name='joint-recovery-source';
WITH inserted AS (
  INSERT INTO river_job (kind,args,state,attempt,max_attempts,priority,scheduled_at,attempted_at,unique_key)
  VALUES ('generate_source_document','{"entity_id":1,"entity_version":1}'::jsonb,'available',1,3,1,
          '2026-08-29T00:02:00Z'::timestamptz,'2026-08-29T00:03:00Z'::timestamptz,
          decode(repeat('b',64),'hex'))
  RETURNING id
)
INSERT INTO river_job_attempt (job_id,attempt,error,created_at)
SELECT id,1,'lease_expired','2026-08-29T00:04:00Z'::timestamptz FROM inserted;`); err != nil {
		return fmt.Errorf("create isolated PostgreSQL and River fixture: %w", err)
	}
	postgresFacts, err := postgresInventory(ctx, runtimeDB.SQL)
	if err != nil {
		return err
	}
	riverFacts, err := riverInventory(ctx, runtimeDB.SQL)
	if err != nil {
		return err
	}
	recovery.expected["postgres_facts"] = postgresFacts
	recovery.expected["river_jobs_attempts"] = riverFacts

	objects := []struct {
		key, body, contentType string
	}{
		{"raw/v1/joint-recovery/evidence.json", `{"fixture":"joint-recovery","sequence":1}`, "application/json"},
		{"knowledge/v1/17/3.md", "# Protected snapshot\n\nmanual bytes survive\n", "text/markdown; charset=utf-8"},
	}
	for _, object := range objects {
		options := minio.PutObjectOptions{ContentType: object.contentType, DisableMultipart: true, UserMetadata: map[string]string{"fixture-version": "v1"}}
		if _, err := recovery.minioClient.PutObject(ctx, recovery.sourceBucket, object.key, strings.NewReader(object.body), int64(len(object.body)), options); err != nil {
			return errors.New("create isolated MinIO fixture")
		}
	}

	for _, fixture := range []struct {
		path  string
		input knowledgedomain.VaultDocumentRenderInput
		human string
	}{
		{"reports/17.md", knowledgedomain.VaultDocumentRenderInput{DocumentID: 17, RevisionNo: 3, Type: knowledgedomain.DocumentReport, SourceID: 91, Title: "Recovery report", Generated: "approved generated report"}, "operator report note  \n\n- [ ] preserve spacing"},
		{"topics/23.md", knowledgedomain.VaultDocumentRenderInput{DocumentID: 23, RevisionNo: 2, Type: knowledgedomain.DocumentTopic, SourceID: 92, Title: "Recovery topic", Generated: "approved generated topic"}, "curator topic note\n\nkeep verbatim"},
	} {
		content, err := knowledgedomain.RenderVaultDocument(fixture.input)
		if err != nil {
			return errors.New("render isolated Vault fixture")
		}
		human := knowledgedomain.HumanRegionBegin + "\n" + fixture.human + "\n" + knowledgedomain.HumanRegionEnd
		content = strings.Replace(content, knowledgedomain.HumanRegionBegin+"\n"+knowledgedomain.HumanRegionEnd, human, 1)
		path := filepath.Join(recovery.sourceVault, filepath.FromSlash(fixture.path))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return errors.New("create isolated Vault fixture directory")
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return errors.New("write isolated Vault fixture")
		}
	}
	return nil
}

func (recovery *drill) backup(ctx context.Context) error {
	backupPath := filepath.Join(recovery.workRoot, "backup", "postgres.dump")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o750); err != nil {
		return errors.New("create PostgreSQL backup directory")
	}
	commandDSN, commandEnvironment, err := postgresCommandConnection(recovery.sourceDSN)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "pg_dump", "--dbname="+commandDSN, "--format=custom", "--no-owner", "--no-acl", "--file="+backupPath)
	command.Env = append(os.Environ(), commandEnvironment...)
	if output, err := command.CombinedOutput(); err != nil {
		_ = output
		return errors.New("create isolated PostgreSQL backup")
	}
	objects, objectInventory, err := backupMinIO(ctx, recovery.minioClient, recovery.sourceBucket)
	if err != nil {
		return err
	}
	recovery.backupObjects = objects
	recovery.expected["minio_evidence"] = objectInventory
	if err := copyVaultTree(recovery.sourceVault, recovery.backupVault); err != nil {
		return err
	}
	allFiles, manualRegions, err := vaultInventories(recovery.backupVault)
	if err != nil {
		return err
	}
	recovery.expected["vault_all_files"] = allFiles
	recovery.expected["vault_manual_regions"] = manualRegions
	return nil
}

func (recovery *drill) restore(ctx context.Context) error {
	backupPath := filepath.Join(recovery.workRoot, "backup", "postgres.dump")
	commandDSN, commandEnvironment, err := postgresCommandConnection(recovery.restoreDSN)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "pg_restore", "--dbname="+commandDSN, "--no-owner", "--no-acl", "--exit-on-error", backupPath)
	command.Env = append(os.Environ(), commandEnvironment...)
	if output, err := command.CombinedOutput(); err != nil {
		_ = output
		return errors.New("restore isolated PostgreSQL backup")
	}
	for _, object := range recovery.backupObjects {
		options := minio.PutObjectOptions{ContentType: object.ContentType, DisableMultipart: true, UserMetadata: object.UserMetadata}
		if _, err := recovery.minioClient.PutObject(ctx, recovery.restoreBucket, object.Key, bytes.NewReader(object.Body), int64(len(object.Body)), options); err != nil {
			return errors.New("restore isolated MinIO backup")
		}
	}
	if err := copyVaultTree(recovery.backupVault, recovery.restoreVault); err != nil {
		return err
	}
	database, err := sql.Open("pgx", recovery.restoreDSN)
	if err != nil {
		return errors.New("open restored PostgreSQL")
	}
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		return errors.New("restored PostgreSQL is not readable")
	}
	if _, err := recovery.minioClient.StatObject(ctx, recovery.restoreBucket, recovery.backupObjects[0].Key, minio.StatObjectOptions{}); err != nil {
		return errors.New("restored MinIO is not readable")
	}
	if _, err := os.Stat(recovery.restoreVault); err != nil {
		return errors.New("restored Vault is not readable")
	}
	return nil
}

func (recovery *drill) reconcile(ctx context.Context) ([]assetResult, []string, error) {
	database, err := sql.Open("pgx", recovery.restoreDSN)
	if err != nil {
		return nil, nil, errors.New("open restored PostgreSQL for reconciliation")
	}
	defer database.Close()
	actual := make(map[string]inventory, 5)
	if actual["postgres_facts"], err = postgresInventory(ctx, database); err != nil {
		return nil, nil, err
	}
	if actual["river_jobs_attempts"], err = riverInventory(ctx, database); err != nil {
		return nil, nil, err
	}
	if actual["minio_evidence"], err = minioInventory(ctx, recovery.minioClient, recovery.restoreBucket); err != nil {
		return nil, nil, err
	}
	if actual["vault_all_files"], actual["vault_manual_regions"], err = vaultInventories(recovery.restoreVault); err != nil {
		return nil, nil, err
	}
	names := []string{"postgres_facts", "minio_evidence", "vault_all_files", "vault_manual_regions", "river_jobs_attempts"}
	assets := make([]assetResult, 0, len(names))
	differences := make([]string, 0)
	for _, name := range names {
		expected := recovery.expected[name]
		observed := actual[name]
		assets = append(assets, assetResult{
			Name: name, ExpectedCount: expected.Count, ActualCount: observed.Count,
			ExpectedSHA256: expected.SHA256, ActualSHA256: observed.SHA256,
			ExpectedVersionedCount: expected.VersionedCount, ActualVersionedCount: observed.VersionedCount,
		})
		if expected != observed {
			differences = append(differences, name+":count_hash_or_version_mismatch")
		}
	}
	return assets, differences, nil
}

func (recovery *drill) cleanup(ctx context.Context) {
	if recovery.minioClient != nil {
		for _, bucket := range []string{recovery.sourceBucket, recovery.restoreBucket} {
			if bucket != "" {
				removeBucketWithVersions(ctx, recovery.minioClient, bucket)
			}
		}
	}
	for _, database := range []string{recovery.sourceDatabase, recovery.restoreDatabase} {
		if database != "" {
			dropDatabase(ctx, recovery.cfg.DSN, database)
		}
	}
}

func createDatabase(ctx context.Context, maintenanceDSN, name string) error {
	database, err := sql.Open("pgx", maintenanceDSN)
	if err != nil {
		return errors.New("open PostgreSQL maintenance connection")
	}
	defer database.Close()
	if _, err := database.ExecContext(ctx, "CREATE DATABASE "+name+" TEMPLATE template0"); err != nil {
		return errors.New("create isolated recovery PostgreSQL database")
	}
	return nil
}

func dropDatabase(ctx context.Context, maintenanceDSN, name string) {
	database, err := sql.Open("pgx", maintenanceDSN)
	if err != nil {
		return
	}
	defer database.Close()
	_, _ = database.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
}

func databaseDSN(raw, database string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || strings.ContainsAny(database, "\"' /\\") {
		return "", errors.New("HOTKEY_TEST_DSN must be a PostgreSQL URL for an isolated database")
	}
	for _, character := range database {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return "", errors.New("isolated recovery database name is invalid")
		}
	}
	parsed.Path = "/" + database
	parsed.RawPath = ""
	return parsed.String(), nil
}

func postgresCommandConnection(raw string) (string, []string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
		return "", nil, errors.New("PostgreSQL command connection is invalid")
	}
	username := ""
	password := ""
	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
		if username == "" {
			parsed.User = nil
		} else {
			parsed.User = url.User(username)
		}
	}
	environment := make([]string, 0, 1)
	if password != "" {
		environment = append(environment, "PGPASSWORD="+password)
	}
	return parsed.String(), environment, nil
}

func randomIdentifier() (string, error) {
	payload := make([]byte, 6)
	if _, err := rand.Read(payload); err != nil {
		return "", errors.New("create isolated recovery identifier")
	}
	return hex.EncodeToString(payload), nil
}

func recoveryBucketName(base, role, runID string) string {
	base = strings.ToLower(base)
	var normalized strings.Builder
	for _, character := range base {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
			normalized.WriteRune(character)
		} else {
			normalized.WriteByte('-')
		}
	}
	prefix := strings.Trim(normalized.String(), "-")
	if prefix == "" {
		prefix = "hotkey-recovery"
	}
	suffix := "-" + role + "-" + runID
	if len(prefix)+len(suffix) > 63 {
		prefix = strings.TrimRight(prefix[:63-len(suffix)], "-")
	}
	return prefix + suffix
}

func postgresInventory(ctx context.Context, database *sql.DB) (inventory, error) {
	return queryInventory(ctx, database, `
SELECT line FROM (
  SELECT 'source_connection|' || id || '|' || source_type || '|' || name || '|' || endpoint AS line
  FROM source_connections WHERE name='joint-recovery-source'
  UNION ALL
  SELECT 'content|' || contents.id || '|' || external_id || '|' || content_type || '|' || canonical_url || '|' || dedupe_key
  FROM contents JOIN source_connections ON source_connections.id=contents.source_connection_id
  WHERE source_connections.name='joint-recovery-source'
) facts ORDER BY line`)
}

func riverInventory(ctx context.Context, database *sql.DB) (inventory, error) {
	return queryInventory(ctx, database, `
SELECT line FROM (
  SELECT 'job|' || id || '|' || kind || '|' || state || '|' || attempt || '|' || max_attempts || '|' || args::text AS line
  FROM river_job WHERE kind='generate_source_document' AND unique_key=decode(repeat('b',64),'hex')
  UNION ALL
  SELECT 'attempt|' || river_job_attempt.id || '|' || job_id || '|' || river_job_attempt.attempt || '|' || coalesce(error,'')
  FROM river_job_attempt JOIN river_job ON river_job.id=river_job_attempt.job_id
  WHERE river_job.kind='generate_source_document' AND river_job.unique_key=decode(repeat('b',64),'hex')
) facts ORDER BY line`)
}

func queryInventory(ctx context.Context, database *sql.DB, query string) (inventory, error) {
	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		return inventory{}, errors.New("query recovery reconciliation facts")
	}
	defer rows.Close()
	records := make([]string, 0)
	for rows.Next() {
		var record string
		if err := rows.Scan(&record); err != nil {
			return inventory{}, errors.New("scan recovery reconciliation facts")
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return inventory{}, errors.New("read recovery reconciliation facts")
	}
	return recordsInventory(records), nil
}

func recordsInventory(records []string) inventory {
	stable := append([]string(nil), records...)
	sort.Strings(stable)
	digest := sha256.New()
	for _, record := range stable {
		_, _ = io.WriteString(digest, record)
		_, _ = io.WriteString(digest, "\n")
	}
	return inventory{Count: int64(len(stable)), SHA256: hex.EncodeToString(digest.Sum(nil))}
}

func backupMinIO(ctx context.Context, client *minio.Client, bucket string) ([]objectBackup, inventory, error) {
	objects := make([]objectBackup, 0)
	for listed := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
		if listed.Err != nil {
			return nil, inventory{}, errors.New("list isolated MinIO backup objects")
		}
		info, err := client.StatObject(ctx, bucket, listed.Key, minio.StatObjectOptions{VersionID: listed.VersionID})
		if err != nil {
			return nil, inventory{}, errors.New("inspect isolated MinIO backup object")
		}
		object, err := client.GetObject(ctx, bucket, listed.Key, minio.GetObjectOptions{VersionID: info.VersionID})
		if err != nil {
			return nil, inventory{}, errors.New("open isolated MinIO backup object")
		}
		body, readErr := io.ReadAll(io.LimitReader(object, 4<<20))
		closeErr := object.Close()
		if readErr != nil || closeErr != nil || int64(len(body)) != info.Size {
			return nil, inventory{}, errors.New("read isolated MinIO backup object")
		}
		metadata := make(map[string]string, len(info.UserMetadata))
		for key, value := range info.UserMetadata {
			metadata[key] = value
		}
		objects = append(objects, objectBackup{Key: listed.Key, ContentType: info.ContentType, SourceVersionID: info.VersionID, Body: body, UserMetadata: metadata})
	}
	if len(objects) == 0 {
		return nil, inventory{}, errors.New("isolated MinIO backup contains no objects")
	}
	result := objectBackupsInventory(objects)
	return objects, result, nil
}

func minioInventory(ctx context.Context, client *minio.Client, bucket string) (inventory, error) {
	objects, result, err := backupMinIO(ctx, client, bucket)
	if err != nil {
		return inventory{}, err
	}
	_ = objects
	return result, nil
}

func objectBackupsInventory(objects []objectBackup) inventory {
	records := make([]string, 0, len(objects))
	versioned := int64(0)
	for _, object := range objects {
		digest := sha256.Sum256(object.Body)
		metadata := make([]string, 0, len(object.UserMetadata))
		for key, value := range object.UserMetadata {
			metadata = append(metadata, strings.ToLower(key)+"="+value)
		}
		sort.Strings(metadata)
		records = append(records, fmt.Sprintf("%s|%d|%s|%s|%s", object.Key, len(object.Body), hex.EncodeToString(digest[:]), object.ContentType, strings.Join(metadata, ",")))
		if object.SourceVersionID != "" && object.SourceVersionID != "null" {
			versioned++
		}
	}
	result := recordsInventory(records)
	result.VersionedCount = versioned
	return result
}

func removeBucketWithVersions(ctx context.Context, client *minio.Client, bucket string) {
	for object := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: true}) {
		if object.Err == nil {
			_ = client.RemoveObject(ctx, bucket, object.Key, minio.RemoveObjectOptions{VersionID: object.VersionID, GovernanceBypass: true})
		}
	}
	_ = client.RemoveBucket(ctx, bucket)
}

func copyVaultTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("walk isolated Vault backup")
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("resolve isolated Vault backup path")
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("isolated Vault backup contains a symlink")
		}
		if entry.IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return errors.New("create restored Vault directory")
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("isolated Vault backup contains a non-regular file")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return errors.New("read isolated Vault backup file")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return errors.New("create restored Vault parent")
		}
		if err := os.WriteFile(target, body, 0o600); err != nil {
			return errors.New("write restored Vault file")
		}
		return nil
	})
}

func vaultInventories(root string) (inventory, inventory, error) {
	allRecords := make([]string, 0)
	manualRecords := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("walk Vault reconciliation tree")
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return errors.New("Vault reconciliation tree contains a non-regular file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return errors.New("resolve Vault reconciliation path")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return errors.New("read Vault reconciliation file")
		}
		digest := sha256.Sum256(body)
		stablePath := filepath.ToSlash(relative)
		allRecords = append(allRecords, fmt.Sprintf("%s|%d|%s", stablePath, len(body), hex.EncodeToString(digest[:])))
		humanSHA, err := knowledgedomain.VaultHumanRegionSHA256(string(body))
		if err != nil {
			return errors.New("verify protected Vault manual region")
		}
		manualRecords = append(manualRecords, stablePath+"|"+humanSHA)
		return nil
	})
	if err != nil {
		return inventory{}, inventory{}, err
	}
	return recordsInventory(allRecords), recordsInventory(manualRecords), nil
}

func writeExclusiveJSON(path string, value any) error {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return errors.New("joint recovery output path is invalid")
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o750); err != nil {
		return errors.New("create joint recovery evidence directory")
	}
	file, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("joint recovery evidence file already exists or cannot be created")
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(value)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		return errors.New("write joint recovery evidence")
	}
	return nil
}
