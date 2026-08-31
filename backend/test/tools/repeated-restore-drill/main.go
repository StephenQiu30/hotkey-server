package main

import (
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
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/bootstrap"
	knowledgedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	platformconfig "github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	platformdatabase "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/observability"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	reportVersion        = "hotkey-repeated-restore-rehearsal-v1"
	backupVersion        = "hotkey-repeated-restore-backup-v1"
	candidateRPO         = 15 * time.Minute
	candidateRTO         = 2 * time.Hour
	composeRuntimeImage  = "pgvector/pgvector:pg16"
	composeStartupWindow = 90 * time.Second
)

var assetNames = []string{
	"postgres_facts",
	"minio_evidence",
	"vault_all_files",
	"vault_manual_regions",
	"river_jobs_attempts",
}

type config struct {
	Output, Environment, Hardware, GitRevision, ComposeFile string
	ProductionEgressDisabled                                bool
}

type runtimeFacts struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	CPUs   int    `json:"cpus"`
}

type inventory struct {
	Count   int64  `json:"count"`
	SHA256  string `json:"sha256"`
	Version int64  `json:"versioned_count,omitempty"`
}

type assetComparison struct {
	Name                   string `json:"name"`
	ExpectedCount          int64  `json:"expected_count"`
	ActualCount            int64  `json:"actual_count"`
	ExpectedSHA256         string `json:"expected_sha256"`
	ActualSHA256           string `json:"actual_sha256"`
	ExpectedVersionedCount int64  `json:"expected_versioned_count,omitempty"`
	ActualVersionedCount   int64  `json:"actual_versioned_count,omitempty"`
}

type restoreResult struct {
	Role                      string                    `json:"role"`
	IndependentComposeProject bool                      `json:"independent_compose_project"`
	NewVolumes                []string                  `json:"new_volumes"`
	SameBackupSHA256          string                    `json:"same_backup_sha256"`
	SchemaCompatible          bool                      `json:"schema_compatible"`
	OpenAPICompatible         bool                      `json:"openapi_compatible"`
	RPOSeconds                float64                   `json:"rpo_seconds"`
	RTOSeconds                float64                   `json:"rto_seconds"`
	CandidateRPOMet           bool                      `json:"candidate_rpo_met"`
	CandidateRTOMet           bool                      `json:"candidate_rto_met"`
	CutoverPermitted          bool                      `json:"cutover_permitted"`
	Assets                    []assetComparison         `json:"assets"`
	ApplicationRollback       applicationRollbackResult `json:"application_rollback"`
	Differences               []string                  `json:"differences"`
}

type readinessFixtureResult struct {
	Contract                 string `json:"contract"`
	ReadinessStatus          int    `json:"readiness_status"`
	AdmittedBusinessRequests int    `json:"admitted_business_requests"`
	MutationStarted          bool   `json:"mutation_started"`
}

type applicationRollbackResult struct {
	IncompatibleInstances              []readinessFixtureResult `json:"incompatible_instances"`
	CompatibleReadinessStatus          int                      `json:"compatible_readiness_status"`
	CompatibleAdmittedBusinessRequests int                      `json:"compatible_admitted_business_requests"`
	Assets                             []assetComparison        `json:"assets"`
	Differences                        []string                 `json:"differences"`
}

type failureResult struct {
	Case                      string   `json:"case"`
	FailureCode               string   `json:"failure_code"`
	TargetCreated             bool     `json:"target_created"`
	MutationStarted           bool     `json:"mutation_started"`
	StoppedBeforeMutation     bool     `json:"stopped_before_mutation"`
	StoppedBeforeCutover      bool     `json:"stopped_before_cutover"`
	ExistingTargetOverwritten bool     `json:"existing_target_overwritten"`
	CutoverPermitted          bool     `json:"cutover_permitted"`
	Differences               []string `json:"differences"`
}

type report struct {
	Version                  string          `json:"version"`
	Status                   string          `json:"status"`
	Approval                 string          `json:"approval"`
	Environment              string          `json:"environment"`
	Hardware                 string          `json:"hardware"`
	GitRevision              string          `json:"git_revision"`
	Runtime                  runtimeFacts    `json:"runtime"`
	Isolated                 bool            `json:"isolated"`
	ProductionEgressDisabled bool            `json:"production_egress_disabled"`
	RootComposeContract      bool            `json:"root_compose_contract"`
	SameBackupSHA256         string          `json:"same_backup_sha256"`
	SchemaSHA256             string          `json:"schema_sha256"`
	OpenAPISHA256            string          `json:"openapi_sha256"`
	RecoveryPointAt          time.Time       `json:"recovery_point_at"`
	IncidentCutoffAt         time.Time       `json:"incident_cutoff_at"`
	Restores                 []restoreResult `json:"restores"`
	FailureStops             []failureResult `json:"failure_stops"`
	Differences              []string        `json:"differences"`
	Exclusions               []string        `json:"exclusions"`
}

type backupManifest struct {
	Version            string               `json:"version"`
	GitRevision        string               `json:"git_revision,omitempty"`
	SchemaSHA256       string               `json:"schema_sha256"`
	OpenAPISHA256      string               `json:"openapi_sha256"`
	PostgresDumpSHA256 string               `json:"postgres_dump_sha256"`
	MinIOFiles         inventory            `json:"minio_files"`
	VaultFiles         inventory            `json:"vault_files"`
	Assets             map[string]inventory `json:"assets,omitempty"`
	RecoveryPointAt    time.Time            `json:"recovery_point_at,omitempty"`
	PackageSHA256      string               `json:"package_sha256"`
}

type codedError struct {
	code string
}

func (err codedError) Error() string { return err.code }

type composeProject struct {
	cfg                                    config
	name, role, prefix, postgresPassword   string
	minioAccessKey, minioSecretKey, bucket string
	postgresPort, minioPort                int
	postgresContainer, minioContainer      string
	vaultVolume, dsn                       string
	minioClient                            *minio.Client
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
	ctx, cancel := context.WithTimeout(parent, 20*time.Minute)
	defer cancel()
	workRoot, err := os.MkdirTemp("", "hotkey-repeated-restore-")
	if err != nil {
		return errors.New("create repeated restore workspace")
	}
	defer func() { _ = os.RemoveAll(workRoot) }()
	runID, err := randomIdentifier()
	if err != nil {
		return err
	}
	repoRoot := filepath.Dir(cfg.ComposeFile)
	schemaSHA, err := fileSHA256(filepath.Join(repoRoot, "backend", "db", "schema.sql"))
	if err != nil {
		return errors.New("hash canonical schema")
	}
	openAPISHA, err := fileSHA256(filepath.Join(repoRoot, "docs", "openapi", "swagger.json"))
	if err != nil {
		return errors.New("hash canonical OpenAPI")
	}

	source, err := startComposeProject(ctx, cfg, runID, "source")
	if err != nil {
		return err
	}
	defer source.stop(ctx)
	if err := createSourceFixture(ctx, source, filepath.Join(workRoot, "source-vault")); err != nil {
		return err
	}
	recoveryPointAt := time.Now().UTC()
	expected, err := collectAssets(ctx, source, filepath.Join(workRoot, "source-vault-export"))
	if err != nil {
		return err
	}
	backupRoot := filepath.Join(workRoot, "backup")
	if err := createBackup(ctx, source, backupRoot); err != nil {
		return err
	}
	manifest, err := createBackupManifest(backupRoot, schemaSHA, openAPISHA)
	if err != nil {
		return err
	}
	manifest.GitRevision = cfg.GitRevision
	manifest.RecoveryPointAt = recoveryPointAt
	manifest.Assets = cloneInventories(expected)
	sealBackupManifest(&manifest)
	incidentCutoffAt := time.Now().UTC()
	source.stop(ctx)
	source = &composeProject{}

	failureStops, err := preflightFailureEvidence(workRoot, backupRoot, manifest, schemaSHA, openAPISHA)
	if err != nil {
		return err
	}
	restores := make([]restoreResult, 0, 2)
	for _, role := range []string{"restore-a", "restore-b"} {
		result, err := executeRestore(ctx, cfg, runID, role, backupRoot, manifest, schemaSHA, openAPISHA, incidentCutoffAt)
		if err != nil {
			return err
		}
		restores = append(restores, result)
	}
	reconcileFailure, err := executeReconciliationFailure(ctx, cfg, runID, backupRoot, manifest, schemaSHA, openAPISHA)
	if err != nil {
		return err
	}
	failureStops = append(failureStops, reconcileFailure)

	result := report{
		Version: reportVersion, Status: "verified", Approval: "automated_isolated_fixture",
		Environment: cfg.Environment, Hardware: cfg.Hardware, GitRevision: cfg.GitRevision,
		Runtime:  runtimeFacts{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, CPUs: runtime.NumCPU()},
		Isolated: true, ProductionEgressDisabled: cfg.ProductionEgressDisabled, RootComposeContract: true,
		SameBackupSHA256: manifest.PackageSHA256, SchemaSHA256: schemaSHA, OpenAPISHA256: openAPISHA,
		RecoveryPointAt: recoveryPointAt, IncidentCutoffAt: incidentCutoffAt,
		Restores: restores, FailureStops: failureStops, Differences: []string{},
		Exclusions: []string{"production_data", "production_cutover", "external_connectors", "notification_delivery", "redis_ephemeral_state"},
	}
	if err := writeExclusiveJSON(cfg.Output, result); err != nil {
		return err
	}
	fmt.Printf("repeated restore evidence written (restores=%d failures=%d)\n", len(restores), len(failureStops))
	return nil
}

func loadConfig() (config, error) {
	composeFile := strings.TrimSpace(os.Getenv("HOTKEY_REPEATED_RESTORE_COMPOSE_FILE"))
	if composeFile == "" {
		composeFile = filepath.Join("..", "docker-compose.yml")
	}
	absoluteCompose, err := filepath.Abs(composeFile)
	if err != nil {
		return config{}, errors.New("resolve root Compose file")
	}
	result := config{
		Output:                   strings.TrimSpace(os.Getenv("HOTKEY_REPEATED_RESTORE_OUTPUT")),
		Environment:              strings.TrimSpace(os.Getenv("HOTKEY_REPEATED_RESTORE_ENVIRONMENT")),
		Hardware:                 strings.TrimSpace(os.Getenv("HOTKEY_REPEATED_RESTORE_HARDWARE")),
		GitRevision:              strings.TrimSpace(os.Getenv("HOTKEY_REPEATED_RESTORE_GIT_REVISION")),
		ComposeFile:              absoluteCompose,
		ProductionEgressDisabled: strings.EqualFold(strings.TrimSpace(os.Getenv("HOTKEY_REPEATED_RESTORE_PRODUCTION_EGRESS_DISABLED")), "true"),
	}
	if result.Output == "" || result.Environment == "" || result.Hardware == "" {
		return config{}, errors.New("repeated restore output, environment and hardware metadata are required")
	}
	if len(result.GitRevision) != 40 || strings.Trim(result.GitRevision, "0123456789abcdef") != "" {
		return config{}, errors.New("HOTKEY_REPEATED_RESTORE_GIT_REVISION must be a 40-character lowercase commit SHA")
	}
	if !result.ProductionEgressDisabled {
		return config{}, errors.New("repeated restore requires HOTKEY_REPEATED_RESTORE_PRODUCTION_EGRESS_DISABLED=true")
	}
	info, err := os.Lstat(result.ComposeFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || filepath.Base(result.ComposeFile) != "docker-compose.yml" {
		return config{}, errors.New("HOTKEY_REPEATED_RESTORE_COMPOSE_FILE must be the regular root docker-compose.yml")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return config{}, errors.New("docker is required for repeated restore")
	}
	return result, nil
}

func composeProjectName(runID, role string) (string, error) {
	if len(runID) != 12 || strings.Trim(runID, "0123456789abcdef") != "" || role == "" {
		return "", errors.New("invalid repeated restore identity")
	}
	for _, character := range role {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return "", errors.New("invalid repeated restore role")
		}
	}
	name := "hkr-" + runID + "-" + role
	if name == "hotkey" || len(name) > 63 {
		return "", errors.New("unsafe repeated restore Compose project name")
	}
	return name, nil
}

func startComposeProject(ctx context.Context, cfg config, runID, role string) (*composeProject, error) {
	name, err := composeProjectName(runID, role)
	if err != nil {
		return nil, err
	}
	postgresPort, err := freePort(ctx)
	if err != nil {
		return nil, err
	}
	minioPort, err := freePort(ctx)
	if err != nil {
		return nil, err
	}
	project := &composeProject{
		cfg: cfg, name: name, role: role, prefix: name,
		postgresPassword: "postgres-" + runID, minioAccessKey: "minio" + runID,
		minioSecretKey: "minio-secret-" + runID + "-isolated", bucket: "hotkey-recovery",
		postgresPort: postgresPort, minioPort: minioPort, vaultVolume: name + "_vault_data",
	}
	environment := append(os.Environ(),
		"HOTKEY_CONTAINER_PREFIX="+project.prefix,
		"POSTGRES_PASSWORD="+project.postgresPassword,
		"MINIO_ROOT_USER="+project.minioAccessKey,
		"MINIO_ROOT_PASSWORD="+project.minioSecretKey,
		"HOTKEY_MINIO_BUCKET="+project.bucket,
	)
	project.postgresContainer, err = composeRunContainer(ctx, project, environment, "postgres", postgresPort, 5432)
	if err != nil {
		project.stop(ctx)
		return nil, err
	}
	project.minioContainer, err = composeRunContainer(ctx, project, environment, "minio", minioPort, 9000)
	if err != nil {
		project.stop(ctx)
		return nil, err
	}
	if err := waitContainerHealthy(ctx, project.postgresContainer); err != nil {
		project.stop(ctx)
		return nil, err
	}
	if err := waitContainerHealthy(ctx, project.minioContainer); err != nil {
		project.stop(ctx)
		return nil, err
	}
	project.dsn = fmt.Sprintf("postgres://hotkey:%s@127.0.0.1:%d/hotkey?sslmode=disable", url.QueryEscape(project.postgresPassword), postgresPort)
	client, err := minio.New(fmt.Sprintf("127.0.0.1:%d", minioPort), &minio.Options{
		Creds:  credentials.NewStaticV4(project.minioAccessKey, project.minioSecretKey, ""),
		Secure: false, Region: "us-east-1", BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		project.stop(ctx)
		return nil, errors.New("create repeated restore MinIO client")
	}
	project.minioClient = client
	if err := client.MakeBucket(ctx, project.bucket, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		project.stop(ctx)
		return nil, errors.New("create repeated restore MinIO bucket")
	}
	if err := client.SetBucketVersioning(ctx, project.bucket, minio.BucketVersioningConfiguration{Status: "Enabled"}); err != nil {
		project.stop(ctx)
		return nil, errors.New("enable repeated restore MinIO versioning")
	}
	volume := exec.CommandContext(ctx, "docker", "volume", "create",
		"--label", "com.docker.compose.project="+name,
		"--label", "com.docker.compose.volume=vault_data", project.vaultVolume)
	if output, err := volume.CombinedOutput(); err != nil {
		_ = output
		project.stop(ctx)
		return nil, errors.New("create repeated restore Vault volume")
	}
	return project, nil
}

func composeRunContainer(ctx context.Context, project *composeProject, environment []string, service string, hostPort, containerPort int) (string, error) {
	args := []string{"compose", "--file", project.cfg.ComposeFile, "--project-name", project.name,
		"run", "-T", "--detach", "--no-deps", "--publish", fmt.Sprintf("127.0.0.1:%d:%d", hostPort, containerPort), service}
	command := exec.CommandContext(ctx, "docker", args...)
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		_ = output
		return "", fmt.Errorf("start isolated %s Compose service", service)
	}
	resolve := exec.CommandContext(ctx, "docker", "compose", "--file", project.cfg.ComposeFile, "--project-name", project.name, "ps", "--all", "--quiet", service)
	resolve.Env = environment
	resolved, err := resolve.Output()
	if err != nil {
		return "", fmt.Errorf("resolve isolated %s Compose container", service)
	}
	containerIDs := strings.Fields(string(resolved))
	if len(containerIDs) != 1 {
		return "", fmt.Errorf("resolve isolated %s Compose container", service)
	}
	return containerIDs[0], nil
}

func waitContainerHealthy(ctx context.Context, containerID string) error {
	deadline := time.Now().Add(composeStartupWindow)
	for time.Now().Before(deadline) {
		command := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}", containerID)
		output, err := command.Output()
		if err == nil && strings.TrimSpace(string(output)) == "healthy" {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("wait for isolated Compose service")
		case <-time.After(500 * time.Millisecond):
		}
	}
	return errors.New("isolated Compose service did not become healthy")
}

func (project *composeProject) stop(ctx context.Context) {
	if project == nil || project.name == "" || project.name == "hotkey" || !strings.HasPrefix(project.name, "hkr-") {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	for _, containerID := range []string{project.postgresContainer, project.minioContainer} {
		if containerID != "" {
			_ = exec.CommandContext(cleanupCtx, "docker", "rm", "--force", containerID).Run()
		}
	}
	command := exec.CommandContext(cleanupCtx, "docker", "compose", "--file", project.cfg.ComposeFile, "--project-name", project.name, "down", "--volumes", "--remove-orphans")
	command.Env = append(os.Environ(), "HOTKEY_CONTAINER_PREFIX="+project.prefix)
	_ = command.Run()
	if project.vaultVolume != "" && strings.HasPrefix(project.vaultVolume, "hkr-") {
		_ = exec.CommandContext(cleanupCtx, "docker", "volume", "rm", project.vaultVolume).Run()
	}
	project.name = ""
}

func createSourceFixture(ctx context.Context, project *composeProject, vaultStaging string) error {
	database, err := platformdatabase.Open(ctx, project.dsn)
	if err != nil {
		return errors.New("open isolated source PostgreSQL")
	}
	defer func() { _ = database.Close() }()
	if err := platformdatabase.InitializeEmpty(ctx, database.Pool); err != nil {
		return errors.New("initialize isolated source PostgreSQL")
	}
	if _, err := database.SQL.ExecContext(ctx, `
INSERT INTO source_connections (source_type,name,endpoint)
VALUES ('rss','repeated-restore-source','https://fixture.invalid/recovery');
INSERT INTO contents (source_connection_id,external_id,content_type,canonical_url,published_at,fetched_at,dedupe_key)
SELECT id,'repeated-restore-content','article','https://fixture.invalid/recovery/1',
       '2026-08-30T00:00:00Z'::timestamptz,'2026-08-30T00:01:00Z'::timestamptz,repeat('a',64)
FROM source_connections WHERE name='repeated-restore-source';
WITH inserted AS (
  INSERT INTO river_job (kind,args,state,attempt,max_attempts,priority,scheduled_at,attempted_at,unique_key)
  VALUES ('generate_source_document','{"entity_id":1,"entity_version":1}'::jsonb,'available',1,3,1,
          '2026-08-30T00:02:00Z'::timestamptz,'2026-08-30T00:03:00Z'::timestamptz,decode(repeat('b',64),'hex'))
  RETURNING id
)
INSERT INTO river_job_attempt (job_id,attempt,error,created_at)
SELECT id,1,'lease_expired','2026-08-30T00:04:00Z'::timestamptz FROM inserted;`); err != nil {
		return errors.New("create repeated restore PostgreSQL fixture")
	}
	for _, object := range []struct{ key, body string }{
		{"raw/v1/repeated-restore/evidence.json", `{"fixture":"repeated-restore","sequence":1}`},
		{"knowledge/v1/17/3.md", "# Protected snapshot\n\nmanual bytes survive\n"},
	} {
		if _, err := project.minioClient.PutObject(ctx, project.bucket, object.key, strings.NewReader(object.body), int64(len(object.body)), minio.PutObjectOptions{ContentType: "application/octet-stream", DisableMultipart: true}); err != nil {
			return errors.New("create repeated restore MinIO fixture")
		}
	}
	if err := os.MkdirAll(vaultStaging, 0o750); err != nil {
		return errors.New("create repeated restore Vault staging")
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
			return errors.New("render repeated restore Vault fixture")
		}
		manual := knowledgedomain.HumanRegionBegin + "\n" + fixture.human + "\n" + knowledgedomain.HumanRegionEnd
		content = strings.Replace(content, knowledgedomain.HumanRegionBegin+"\n"+knowledgedomain.HumanRegionEnd, manual, 1)
		path := filepath.Join(vaultStaging, filepath.FromSlash(fixture.path))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return errors.New("create repeated restore Vault parent")
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return errors.New("write repeated restore Vault fixture")
		}
	}
	return copyDirectoryToVolume(ctx, vaultStaging, project.vaultVolume)
}

func createBackup(ctx context.Context, source *composeProject, root string) error {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return errors.New("create repeated restore backup root")
	}
	dumpPath := filepath.Join(root, "postgres.dump")
	file, err := os.OpenFile(dumpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("create repeated restore PostgreSQL backup")
	}
	command := exec.CommandContext(ctx, "docker", "exec", source.postgresContainer,
		"pg_dump", "--username=hotkey", "--dbname=hotkey", "--format=custom", "--no-owner", "--no-acl")
	command.Stdout = file
	command.Stderr = io.Discard
	runErr := command.Run()
	closeErr := file.Close()
	if runErr != nil || closeErr != nil {
		return errors.New("create repeated restore PostgreSQL backup")
	}
	minioRoot := filepath.Join(root, "minio")
	if err := exportMinIO(ctx, source, minioRoot); err != nil {
		return err
	}
	return copyVolumeToDirectory(ctx, source.vaultVolume, filepath.Join(root, "vault"))
}

func executeRestore(ctx context.Context, cfg config, runID, role, backupRoot string, manifest backupManifest, schemaSHA, openAPISHA string, incidentCutoffAt time.Time) (restoreResult, error) {
	startedAt := time.Now().UTC()
	project, err := startComposeProject(ctx, cfg, runID, role)
	if err != nil {
		return restoreResult{}, err
	}
	defer project.stop(ctx)
	if err := validateBackupPackage(backupRoot, manifest, schemaSHA, openAPISHA); err != nil {
		return restoreResult{}, err
	}
	if err := ensureEmptyTarget(ctx, project); err != nil {
		return restoreResult{}, err
	}
	if err := restoreBackup(ctx, project, backupRoot); err != nil {
		return restoreResult{}, err
	}
	if err := verifyRestoredSchema(ctx, project); err != nil {
		return restoreResult{}, err
	}
	actual, err := collectAssets(ctx, project, filepath.Join(filepath.Dir(backupRoot), role+"-vault-export"))
	if err != nil {
		return restoreResult{}, err
	}
	assets, differences := compareAssets(manifest.Assets, actual)
	completedAt := time.Now().UTC()
	if len(differences) != 0 {
		return restoreResult{}, fmt.Errorf("%s restore has unexplained differences", role)
	}
	rollback, err := executeApplicationRollbackDrill(
		ctx,
		project,
		actual,
		filepath.Join(filepath.Dir(backupRoot), role+"-rollback-vault-export"),
	)
	if err != nil {
		return restoreResult{}, err
	}
	rpo := incidentCutoffAt.Sub(manifest.RecoveryPointAt)
	rto := completedAt.Sub(startedAt)
	return restoreResult{
		Role: role, IndependentComposeProject: true,
		NewVolumes: []string{"postgres_data", "minio_data", "vault_data"}, SameBackupSHA256: manifest.PackageSHA256,
		SchemaCompatible: true, OpenAPICompatible: true, RPOSeconds: seconds(rpo), RTOSeconds: seconds(rto),
		CandidateRPOMet: rpo <= candidateRPO, CandidateRTOMet: rto <= candidateRTO,
		CutoverPermitted: true, Assets: assets, ApplicationRollback: rollback, Differences: []string{},
	}, nil
}

func executeApplicationRollbackDrill(ctx context.Context, project *composeProject, before map[string]inventory, vaultExport string) (applicationRollbackResult, error) {
	gin.SetMode(gin.ReleaseMode)
	runtime, err := platformdatabase.Open(ctx, project.dsn)
	if err != nil {
		return applicationRollbackResult{}, errors.New("open rollback readiness database")
	}
	defer func() { _ = runtime.Close() }()

	cfg := platformconfig.Default()
	cfg.Environment = "testing"
	cfg.Role = "api"
	cfg.DatabaseURL = project.dsn
	cfg.Authentication.JWTSecret = "rollback-jwt-secret-0123456789abcdef"
	cfg.Authentication.VerificationHMACSecret = "rollback-hmac-secret-0123456789abcdef"
	cfg.Authentication.AllowedOrigins = []string{"http://127.0.0.1:8010"}
	checks := map[string]bootstrap.RuntimeCompatibilityCheck{
		"configuration": bootstrap.RuntimeConfigurationCompatibilityCheck(cfg),
		"schema": func(ctx context.Context) error {
			_, err := platformdatabase.Verify(ctx, runtime.Pool)
			return err
		},
		"openapi": func(context.Context) error {
			return bootstrap.VerifyEmbeddedOpenAPICompatibility()
		},
	}
	fixtures := make([]readinessFixtureResult, 0, 3)
	for _, contract := range []string{"schema", "openapi", "configuration"} {
		incompatible := cloneRuntimeCompatibilityChecks(checks)
		incompatible[contract] = func(context.Context) error {
			return errors.New("compatibility sentinel must not leak")
		}
		status, admitted, err := probeCompatibilityAdmission(ctx, cfg, incompatible)
		if err != nil {
			return applicationRollbackResult{}, err
		}
		fixtures = append(fixtures, readinessFixtureResult{
			Contract: contract, ReadinessStatus: status, AdmittedBusinessRequests: admitted, MutationStarted: false,
		})
	}
	compatibleStatus, compatibleAdmitted, err := probeCompatibilityAdmission(ctx, cfg, checks)
	if err != nil {
		return applicationRollbackResult{}, err
	}
	after, err := collectAssets(ctx, project, vaultExport)
	if err != nil {
		return applicationRollbackResult{}, err
	}
	assets, differences := compareAssets(before, after)
	result := applicationRollbackResult{
		IncompatibleInstances:              fixtures,
		CompatibleReadinessStatus:          compatibleStatus,
		CompatibleAdmittedBusinessRequests: compatibleAdmitted,
		Assets:                             assets,
		Differences:                        differences,
	}
	if err := validateApplicationRollbackEvidence(result); err != nil {
		return applicationRollbackResult{}, err
	}
	return result, nil
}

func cloneRuntimeCompatibilityChecks(source map[string]bootstrap.RuntimeCompatibilityCheck) map[string]bootstrap.RuntimeCompatibilityCheck {
	result := make(map[string]bootstrap.RuntimeCompatibilityCheck, len(source))
	for name, check := range source {
		result[name] = check
	}
	return result
}

// contextcheck follows the Gin router factory into request middleware even
// though every synthetic request below is created with ctx.
//
//nolint:contextcheck
func probeCompatibilityAdmission(ctx context.Context, cfg platformconfig.Config, checks map[string]bootstrap.RuntimeCompatibilityCheck) (int, int, error) {
	metrics, err := observability.NewMetrics()
	if err != nil {
		return 0, 0, errors.New("create rollback readiness metrics")
	}
	telemetry, err := observability.NewTelemetryWithContext(ctx, cfg)
	if err != nil {
		return 0, 0, errors.New("create rollback readiness telemetry")
	}
	defer func() { _ = telemetry.Shutdown(ctx) }()
	readiness := bootstrap.NewRuntimeCompatibilityReadiness(
		httptransport.ReadinessFunc(func(context.Context) error { return nil }),
		checks,
	)
	router := newCompatibilityProbeRouter(readiness, metrics, telemetry, cfg)
	readyResponse := httptest.NewRecorder()
	router.ServeHTTP(readyResponse, httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil))
	if strings.Contains(readyResponse.Body.String(), "compatibility sentinel") {
		return 0, 0, errors.New("rollback readiness leaked an internal compatibility error")
	}
	admitted := 0
	if readyResponse.Code == http.StatusOK {
		businessResponse := httptest.NewRecorder()
		router.ServeHTTP(businessResponse, httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/capabilities", nil))
		if businessResponse.Code != http.StatusOK {
			return 0, 0, errors.New("compatible rollback business request failed")
		}
		admitted = 1
	}
	return readyResponse.Code, admitted, nil
}

func newCompatibilityProbeRouter(
	readiness httptransport.Readiness,
	metrics *observability.Metrics,
	telemetry *observability.Telemetry,
	cfg platformconfig.Config,
) *gin.Engine {
	return httptransport.NewRouter(readiness, metrics, telemetry, zap.NewNop(), cfg)
}

func validateApplicationRollbackEvidence(result applicationRollbackResult) error {
	wanted := map[string]bool{"schema": false, "openapi": false, "configuration": false}
	if len(result.IncompatibleInstances) != len(wanted) {
		return errors.New("rollback readiness fixture matrix is incomplete")
	}
	for _, fixture := range result.IncompatibleInstances {
		seen, found := wanted[fixture.Contract]
		if !found || seen || fixture.ReadinessStatus != http.StatusServiceUnavailable || fixture.AdmittedBusinessRequests != 0 || fixture.MutationStarted {
			return errors.New("incompatible instance was not stopped before traffic")
		}
		wanted[fixture.Contract] = true
	}
	if result.CompatibleReadinessStatus != http.StatusOK || result.CompatibleAdmittedBusinessRequests != 1 || len(result.Differences) != 0 {
		return errors.New("compatible application rollback did not recover cleanly")
	}
	if len(result.Assets) != len(assetNames) {
		return errors.New("application rollback asset matrix is incomplete")
	}
	for index, name := range assetNames {
		asset := result.Assets[index]
		if asset.Name != name || asset.ExpectedCount != asset.ActualCount || asset.ExpectedSHA256 == "" || asset.ExpectedSHA256 != asset.ActualSHA256 || asset.ExpectedVersionedCount != asset.ActualVersionedCount {
			return errors.New("application rollback changed protected assets")
		}
	}
	return nil
}

func executeReconciliationFailure(ctx context.Context, cfg config, runID, backupRoot string, manifest backupManifest, schemaSHA, openAPISHA string) (failureResult, error) {
	tampered := manifest
	tampered.Assets = cloneInventories(manifest.Assets)
	value := tampered.Assets["minio_evidence"]
	value.Count++
	tampered.Assets["minio_evidence"] = value
	sealBackupManifest(&tampered)
	project, err := startComposeProject(ctx, cfg, runID, "failure")
	if err != nil {
		return failureResult{}, err
	}
	defer project.stop(ctx)
	if err := validateBackupPackage(backupRoot, tampered, schemaSHA, openAPISHA); err != nil {
		return failureResult{}, err
	}
	if err := ensureEmptyTarget(ctx, project); err != nil {
		return failureResult{}, err
	}
	if err := restoreBackup(ctx, project, backupRoot); err != nil {
		return failureResult{}, err
	}
	if err := verifyRestoredSchema(ctx, project); err != nil {
		return failureResult{}, err
	}
	actual, err := collectAssets(ctx, project, filepath.Join(filepath.Dir(backupRoot), "failure-vault-export"))
	if err != nil {
		return failureResult{}, err
	}
	_, differences := compareAssets(tampered.Assets, actual)
	if len(differences) == 0 {
		return failureResult{}, errors.New("reconciliation mismatch fixture did not stop cutover")
	}
	return failureResult{
		Case: "reconciliation_mismatch", FailureCode: "reconciliation_failed", TargetCreated: true,
		MutationStarted: true, StoppedBeforeMutation: false, StoppedBeforeCutover: true,
		ExistingTargetOverwritten: false, CutoverPermitted: false, Differences: differences,
	}, nil
}

func preflightFailureEvidence(workRoot, backupRoot string, manifest backupManifest, schemaSHA, openAPISHA string) ([]failureResult, error) {
	missingRoot := filepath.Join(workRoot, "missing-package")
	if err := os.MkdirAll(missingRoot, 0o750); err != nil {
		return nil, err
	}
	corruptRoot := filepath.Join(workRoot, "corrupt-package")
	if err := os.MkdirAll(filepath.Join(corruptRoot, "minio"), 0o750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(corruptRoot, "vault"), 0o750); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(corruptRoot, "postgres.dump"), []byte("corrupt"), 0o600); err != nil {
		return nil, err
	}
	cases := []struct {
		name, expectedCode, root, schema, openAPI string
	}{
		{"missing_backup", "backup_missing", missingRoot, schemaSHA, openAPISHA},
		{"corrupt_backup", "backup_checksum_mismatch", corruptRoot, schemaSHA, openAPISHA},
		{"schema_incompatible", "schema_incompatible", backupRoot, strings.Repeat("f", 64), openAPISHA},
	}
	results := make([]failureResult, 0, len(cases))
	for _, fixture := range cases {
		err := validateBackupPackage(fixture.root, manifest, fixture.schema, fixture.openAPI)
		if failureCode(err) != fixture.expectedCode {
			return nil, fmt.Errorf("%s returned %q", fixture.name, failureCode(err))
		}
		results = append(results, failureResult{
			Case: fixture.name, FailureCode: fixture.expectedCode, TargetCreated: false,
			MutationStarted: false, StoppedBeforeMutation: true, StoppedBeforeCutover: true,
			ExistingTargetOverwritten: false, CutoverPermitted: false, Differences: []string{},
		})
	}
	return results, nil
}

func ensureEmptyTarget(ctx context.Context, project *composeProject) error {
	database, err := sql.Open("pgx", project.dsn)
	if err != nil {
		return errors.New("open repeated restore target")
	}
	defer func() { _ = database.Close() }()
	var tables int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname='public'`).Scan(&tables); err != nil || tables != 0 {
		return errors.New("repeated restore target PostgreSQL is not empty")
	}
	objects := project.minioClient.ListObjects(ctx, project.bucket, minio.ListObjectsOptions{Recursive: true})
	for object := range objects {
		if object.Err != nil || object.Key != "" {
			return errors.New("repeated restore target MinIO is not empty")
		}
	}
	return nil
}

func restoreBackup(ctx context.Context, project *composeProject, root string) error {
	backup, err := os.Open(filepath.Join(root, "postgres.dump"))
	if err != nil {
		return codedError{code: "backup_missing"}
	}
	defer func() { _ = backup.Close() }()
	command := exec.CommandContext(ctx, "docker", "exec", "--interactive", project.postgresContainer,
		"pg_restore", "--username=hotkey", "--dbname=hotkey", "--no-owner", "--no-acl", "--exit-on-error")
	command.Stdin = backup
	if output, err := command.CombinedOutput(); err != nil {
		_ = output
		return errors.New("restore repeated PostgreSQL backup")
	}
	if err := importMinIO(ctx, project, filepath.Join(root, "minio")); err != nil {
		return err
	}
	return copyDirectoryToVolume(ctx, filepath.Join(root, "vault"), project.vaultVolume)
}

func verifyRestoredSchema(ctx context.Context, project *composeProject) error {
	database, err := platformdatabase.Open(ctx, project.dsn)
	if err != nil {
		return errors.New("open repeated restored PostgreSQL")
	}
	defer func() { _ = database.Close() }()
	verification, err := platformdatabase.Verify(ctx, database.Pool)
	if err != nil || verification.CatalogFingerprint == "" || len(verification.Tables) == 0 {
		return errors.New("verify repeated restored PostgreSQL schema")
	}
	return nil
}

func createBackupManifest(root, schemaSHA, openAPISHA string) (backupManifest, error) {
	postgresSHA, err := fileSHA256(filepath.Join(root, "postgres.dump"))
	if err != nil {
		return backupManifest{}, codedError{code: "backup_missing"}
	}
	minioFiles, err := directoryInventory(filepath.Join(root, "minio"))
	if err != nil {
		return backupManifest{}, codedError{code: "backup_missing"}
	}
	vaultFiles, err := directoryInventory(filepath.Join(root, "vault"))
	if err != nil {
		return backupManifest{}, codedError{code: "backup_missing"}
	}
	manifest := backupManifest{
		Version: backupVersion, SchemaSHA256: schemaSHA, OpenAPISHA256: openAPISHA,
		PostgresDumpSHA256: postgresSHA, MinIOFiles: minioFiles, VaultFiles: vaultFiles,
	}
	sealBackupManifest(&manifest)
	return manifest, nil
}

func sealBackupManifest(manifest *backupManifest) {
	manifest.PackageSHA256 = ""
	payload, _ := json.Marshal(manifest)
	digest := sha256.Sum256(payload)
	manifest.PackageSHA256 = hex.EncodeToString(digest[:])
}

func validateBackupPackage(root string, manifest backupManifest, schemaSHA, openAPISHA string) error {
	if manifest.Version != backupVersion || manifest.SchemaSHA256 != schemaSHA {
		return codedError{code: "schema_incompatible"}
	}
	if manifest.OpenAPISHA256 != openAPISHA {
		return codedError{code: "openapi_incompatible"}
	}
	postgresSHA, err := fileSHA256(filepath.Join(root, "postgres.dump"))
	if err != nil {
		return codedError{code: "backup_missing"}
	}
	if postgresSHA != manifest.PostgresDumpSHA256 {
		return codedError{code: "backup_checksum_mismatch"}
	}
	minioFiles, err := directoryInventory(filepath.Join(root, "minio"))
	if err != nil {
		return codedError{code: "backup_missing"}
	}
	vaultFiles, err := directoryInventory(filepath.Join(root, "vault"))
	if err != nil {
		return codedError{code: "backup_missing"}
	}
	if minioFiles != manifest.MinIOFiles || vaultFiles != manifest.VaultFiles || minioFiles.Count == 0 || vaultFiles.Count == 0 {
		return codedError{code: "backup_checksum_mismatch"}
	}
	expected := manifest.PackageSHA256
	sealBackupManifest(&manifest)
	if expected == "" || expected != manifest.PackageSHA256 {
		return codedError{code: "backup_manifest_mismatch"}
	}
	return nil
}

func failureCode(err error) string {
	var coded codedError
	if errors.As(err, &coded) {
		return coded.code
	}
	return ""
}

func collectAssets(ctx context.Context, project *composeProject, vaultExport string) (map[string]inventory, error) {
	database, err := sql.Open("pgx", project.dsn)
	if err != nil {
		return nil, errors.New("open repeated restore database for reconciliation")
	}
	defer func() { _ = database.Close() }()
	postgresFacts, err := postgresInventory(ctx, database)
	if err != nil {
		return nil, err
	}
	riverFacts, err := riverInventory(ctx, database)
	if err != nil {
		return nil, err
	}
	minioFacts, err := minioInventory(ctx, project)
	if err != nil {
		return nil, err
	}
	if err := copyVolumeToDirectory(ctx, project.vaultVolume, vaultExport); err != nil {
		return nil, err
	}
	vaultAll, vaultManual, err := vaultInventories(vaultExport)
	if err != nil {
		return nil, err
	}
	return map[string]inventory{
		"postgres_facts": postgresFacts, "minio_evidence": minioFacts,
		"vault_all_files": vaultAll, "vault_manual_regions": vaultManual,
		"river_jobs_attempts": riverFacts,
	}, nil
}

func compareAssets(expected, actual map[string]inventory) ([]assetComparison, []string) {
	assets := make([]assetComparison, 0, len(assetNames))
	differences := make([]string, 0)
	for _, name := range assetNames {
		want, got := expected[name], actual[name]
		assets = append(assets, assetComparison{
			Name: name, ExpectedCount: want.Count, ActualCount: got.Count,
			ExpectedSHA256: want.SHA256, ActualSHA256: got.SHA256,
			ExpectedVersionedCount: want.Version, ActualVersionedCount: got.Version,
		})
		if want != got {
			differences = append(differences, name+":count_hash_or_version_mismatch")
		}
	}
	return assets, differences
}

func cloneInventories(source map[string]inventory) map[string]inventory {
	result := make(map[string]inventory, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func postgresInventory(ctx context.Context, database *sql.DB) (inventory, error) {
	return queryInventory(ctx, database, `
SELECT line FROM (
  SELECT 'source_connection|' || id || '|' || source_type || '|' || name || '|' || endpoint AS line
  FROM source_connections WHERE name='repeated-restore-source'
  UNION ALL
  SELECT 'content|' || contents.id || '|' || external_id || '|' || content_type || '|' || canonical_url || '|' || dedupe_key
  FROM contents JOIN source_connections ON source_connections.id=contents.source_connection_id
  WHERE source_connections.name='repeated-restore-source'
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
		return inventory{}, errors.New("query repeated restore reconciliation facts")
	}
	defer func() { _ = rows.Close() }()
	records := make([]string, 0)
	for rows.Next() {
		var record string
		if err := rows.Scan(&record); err != nil {
			return inventory{}, errors.New("scan repeated restore reconciliation facts")
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return inventory{}, errors.New("read repeated restore reconciliation facts")
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

func exportMinIO(ctx context.Context, project *composeProject, root string) error {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return errors.New("create repeated restore MinIO backup directory")
	}
	count := 0
	for listed := range project.minioClient.ListObjects(ctx, project.bucket, minio.ListObjectsOptions{Recursive: true}) {
		if listed.Err != nil {
			return errors.New("list repeated restore MinIO objects")
		}
		path, err := safeObjectPath(root, listed.Key)
		if err != nil {
			return err
		}
		object, err := project.minioClient.GetObject(ctx, project.bucket, listed.Key, minio.GetObjectOptions{})
		if err != nil {
			return errors.New("open repeated restore MinIO object")
		}
		body, readErr := io.ReadAll(io.LimitReader(object, 4<<20))
		closeErr := object.Close()
		if readErr != nil || closeErr != nil {
			return errors.New("read repeated restore MinIO object")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return errors.New("create repeated restore MinIO object parent")
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			return errors.New("write repeated restore MinIO backup object")
		}
		count++
	}
	if count == 0 {
		return errors.New("repeated restore MinIO backup is empty")
	}
	return nil
}

func importMinIO(ctx context.Context, project *composeProject, root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("walk repeated restore MinIO backup")
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return errors.New("repeated restore MinIO backup contains a non-regular file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("resolve repeated restore MinIO object path")
		}
		file, err := os.Open(path)
		if err != nil {
			return errors.New("open repeated restore MinIO backup object")
		}
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return errors.New("inspect repeated restore MinIO backup object")
		}
		_, putErr := project.minioClient.PutObject(ctx, project.bucket, filepath.ToSlash(relative), file, info.Size(), minio.PutObjectOptions{ContentType: "application/octet-stream", DisableMultipart: true})
		closeErr := file.Close()
		if putErr != nil || closeErr != nil {
			return errors.New("restore repeated MinIO object")
		}
		return nil
	})
}

func minioInventory(ctx context.Context, project *composeProject) (inventory, error) {
	records := make([]string, 0)
	versioned := int64(0)
	for listed := range project.minioClient.ListObjects(ctx, project.bucket, minio.ListObjectsOptions{Recursive: true}) {
		if listed.Err != nil {
			return inventory{}, errors.New("list repeated restore MinIO reconciliation objects")
		}
		object, err := project.minioClient.GetObject(ctx, project.bucket, listed.Key, minio.GetObjectOptions{})
		if err != nil {
			return inventory{}, errors.New("open repeated restore MinIO reconciliation object")
		}
		body, readErr := io.ReadAll(io.LimitReader(object, 4<<20))
		closeErr := object.Close()
		if readErr != nil || closeErr != nil {
			return inventory{}, errors.New("read repeated restore MinIO reconciliation object")
		}
		digest := sha256.Sum256(body)
		records = append(records, fmt.Sprintf("%s|%d|%s", listed.Key, len(body), hex.EncodeToString(digest[:])))
		if listed.VersionID != "" && listed.VersionID != "null" {
			versioned++
		}
	}
	result := recordsInventory(records)
	result.Version = versioned
	return result, nil
}

func safeObjectPath(root, key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return "", errors.New("unsafe repeated restore MinIO object key")
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("unsafe repeated restore MinIO object key")
	}
	return filepath.Join(root, clean), nil
}

func copyDirectoryToVolume(ctx context.Context, source, volume string) error {
	if _, err := directoryInventory(source); err != nil {
		return errors.New("inspect repeated restore Vault source")
	}
	containerID, err := createVolumeCopyContainer(ctx, volume)
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanupCtx, "docker", "rm", "--force", containerID).Run()
	}()
	command := exec.CommandContext(ctx, "docker", "cp", filepath.Clean(source)+string(filepath.Separator)+".", containerID+":/vault")
	if output, err := command.CombinedOutput(); err != nil {
		_ = output
		return errors.New("copy repeated restore Vault into isolated volume")
	}
	return nil
}

func copyVolumeToDirectory(ctx context.Context, volume, destination string) error {
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return errors.New("create repeated restore Vault export directory")
	}
	containerID, err := createVolumeCopyContainer(ctx, volume)
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanupCtx, "docker", "rm", "--force", containerID).Run()
	}()
	command := exec.CommandContext(ctx, "docker", "cp", containerID+":/vault/.", destination)
	if output, err := command.CombinedOutput(); err != nil {
		_ = output
		return errors.New("copy repeated restore Vault from isolated volume")
	}
	return nil
}

func createVolumeCopyContainer(ctx context.Context, volume string) (string, error) {
	if !strings.HasPrefix(volume, "hkr-") {
		return "", errors.New("unsafe repeated restore Vault volume")
	}
	command := exec.CommandContext(ctx, "docker", "create", "--volume", volume+":/vault", composeRuntimeImage, "true")
	output, err := command.CombinedOutput()
	if err != nil {
		_ = output
		return "", errors.New("create repeated restore Vault copy container")
	}
	containerID := strings.TrimSpace(string(output))
	if len(containerID) < 12 || strings.ContainsAny(containerID, " \t\r\n") {
		return "", errors.New("resolve repeated restore Vault copy container")
	}
	return containerID, nil
}

func vaultInventories(root string) (inventory, inventory, error) {
	allRecords := make([]string, 0)
	manualRecords := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("walk repeated restore Vault tree")
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return errors.New("repeated restore Vault tree contains a non-regular file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return errors.New("resolve repeated restore Vault path")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return errors.New("read repeated restore Vault file")
		}
		digest := sha256.Sum256(body)
		stablePath := filepath.ToSlash(relative)
		allRecords = append(allRecords, fmt.Sprintf("%s|%d|%s", stablePath, len(body), hex.EncodeToString(digest[:])))
		manualSHA, err := knowledgedomain.VaultHumanRegionSHA256(string(body))
		if err != nil {
			return errors.New("verify repeated restore Vault manual region")
		}
		manualRecords = append(manualRecords, stablePath+"|"+manualSHA)
		return nil
	})
	if err != nil {
		return inventory{}, inventory{}, err
	}
	return recordsInventory(allRecords), recordsInventory(manualRecords), nil
}

func directoryInventory(root string) (inventory, error) {
	records := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return errors.New("backup directory contains a non-regular file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("resolve backup directory path")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(body)
		records = append(records, fmt.Sprintf("%s|%d|%s", filepath.ToSlash(relative), len(body), hex.EncodeToString(digest[:])))
		return nil
	})
	if err != nil {
		return inventory{}, err
	}
	return recordsInventory(records), nil
}

func fileSHA256(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("backup file is missing or not regular")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	if _, err := io.Copy(digest, io.LimitReader(file, 256<<20)); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func freePort(ctx context.Context) (int, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, errors.New("reserve repeated restore host port")
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func randomIdentifier() (string, error) {
	payload := make([]byte, 6)
	if _, err := rand.Read(payload); err != nil {
		return "", errors.New("create repeated restore identifier")
	}
	return hex.EncodeToString(payload), nil
}

func seconds(value time.Duration) float64 {
	return float64(value.Milliseconds()) / 1000
}

func writeExclusiveJSON(path string, value any) error {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return errors.New("repeated restore output path is invalid")
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o750); err != nil {
		return errors.New("create repeated restore evidence directory")
	}
	file, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("repeated restore evidence already exists or cannot be created")
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(value)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		return errors.New("write repeated restore evidence")
	}
	return nil
}
