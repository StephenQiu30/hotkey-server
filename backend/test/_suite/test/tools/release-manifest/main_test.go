package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildReleaseEvidenceBindsImagesLocksContractsSBOMAndVulnerabilityResults(t *testing.T) {
	repository, inventoryPath := releaseFixture(t)
	result, bom, err := buildReleaseEvidence(releaseConfig{
		RepositoryRoot: repository, ImageInventoryPath: inventoryPath,
		GitRevision: strings.Repeat("a", 40), CIRunID: "12345", Environment: "github-actions-release-candidate",
		ProductionEgressDisabled:  true,
		VulnerabilityGateStatuses: map[string]string{"backend": "success", "frontend": "success", "python_agent": "success"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != manifestVersion || result.Status != "verified" || result.ReleaseReady || len(result.Differences) != 0 {
		t.Fatalf("unexpected release result: %+v", result)
	}
	if len(result.Applications) != 3 || len(result.UpstreamImages) != 4 || len(result.DependencyLocks) != 4 || len(result.Configurations) != 5 || len(result.Operations) != 6 {
		t.Fatalf("incomplete release material counts: apps=%d upstream=%d locks=%d configs=%d operations=%d", len(result.Applications), len(result.UpstreamImages), len(result.DependencyLocks), len(result.Configurations), len(result.Operations))
	}
	if result.Contracts.Schema.SHA256 == "" || result.Contracts.OpenAPI.SHA256 == "" || result.SBOM.SHA256 == "" || result.SBOM.ComponentCount < 3 {
		t.Fatalf("contracts or SBOM are incomplete: %+v", result)
	}
	if bom.BOMFormat != "CycloneDX" || bom.SpecVersion != "1.6" || len(bom.Components) != result.SBOM.ComponentCount {
		t.Fatalf("unexpected SBOM: %+v", bom)
	}
	for _, scan := range result.VulnerabilityResults {
		if !scan.Passed || scan.Status != "success" {
			t.Fatalf("vulnerability gate not bound: %+v", scan)
		}
	}
}

func TestBuildReleaseEvidenceRejectsRevisionDriftAndFailedVulnerabilityGate(t *testing.T) {
	repository, inventoryPath := releaseFixture(t)
	config := releaseConfig{
		RepositoryRoot: repository, ImageInventoryPath: inventoryPath,
		GitRevision: strings.Repeat("b", 40), CIRunID: "12345", Environment: "github-actions-release-candidate",
		ProductionEgressDisabled:  true,
		VulnerabilityGateStatuses: map[string]string{"backend": "success", "frontend": "failure", "python_agent": "success"},
	}
	result, _, err := buildReleaseEvidence(config)
	if err == nil {
		t.Fatal("expected release evidence failure")
	}
	joined := strings.Join(result.Differences, ",")
	if !strings.Contains(joined, "image_inventory_revision_mismatch") || !strings.Contains(joined, "frontend_vulnerability_gate_failure") {
		t.Fatalf("differences = %q", joined)
	}
	if result.ReleaseReady {
		t.Fatal("failed material verification must not be release ready")
	}
}

func TestBuildReleaseEvidenceRejectsMismatchedUpstreamRepositoryDigest(t *testing.T) {
	repository, inventoryPath := releaseFixture(t)
	var inventory imageInventory
	if err := json.Unmarshal([]byte(readFile(t, inventoryPath)), &inventory); err != nil {
		t.Fatal(err)
	}
	inventory.Upstream[0].RepoDigests = []string{"unrelated/image@sha256:" + strings.Repeat("f", 64)}
	payload, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inventoryPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	result, _, err := buildReleaseEvidence(releaseConfig{
		RepositoryRoot: repository, ImageInventoryPath: inventoryPath,
		GitRevision: strings.Repeat("a", 40), CIRunID: "12345", Environment: "github-actions-release-candidate",
		ProductionEgressDisabled:  true,
		VulnerabilityGateStatuses: map[string]string{"backend": "success", "frontend": "success", "python_agent": "success"},
	})
	if err == nil || !strings.Contains(strings.Join(result.Differences, ","), "image_inventory_contract_mismatch") {
		t.Fatalf("mismatched upstream repository digest was not rejected: err=%v differences=%v", err, result.Differences)
	}
}

func TestWriteReleaseOutputsCreatesPrivateImmutableManifestSBOMAndDigest(t *testing.T) {
	directory := t.TempDir()
	manifestPath := filepath.Join(directory, "release-manifest.json")
	sbomPath := filepath.Join(directory, "release-sbom.cdx.json")
	digestPath := filepath.Join(directory, "release-manifest.sha256")
	value := releaseManifest{Version: manifestVersion, Status: "verified", Differences: []string{}}
	bom := cyclonedxBOM{BOMFormat: "CycloneDX", SpecVersion: "1.6", Version: 1, Components: []sbomComponent{}}
	if err := writeReleaseOutputs(manifestPath, sbomPath, digestPath, value, bom); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{manifestPath, sbomPath, digestPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o, want 600", filepath.Base(path), info.Mode().Perm())
		}
	}
	manifestSHA256, err := hashFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if digest := strings.Fields(readFile(t, digestPath))[0]; len(digest) != 64 || digest != manifestSHA256 {
		t.Fatalf("detached manifest digest = %q", digest)
	}
	if err := writeReleaseOutputs(manifestPath, sbomPath, digestPath, value, bom); err == nil {
		t.Fatal("expected exclusive-create failure")
	}
}

func releaseFixture(t *testing.T) (string, string) {
	t.Helper()
	repository := t.TempDir()
	files := map[string]string{
		"backend/go.mod":             "module example.test/hotkey\n\ngo 1.26.6\n",
		"backend/go.sum":             "example.test/dependency v1.2.3 h1:fixture\nexample.test/dependency v1.2.3/go.mod h1:fixture\n",
		"frontend/package-lock.json": `{"name":"hotkey-web","version":"0.2.0","lockfileVersion":3,"packages":{"":{"name":"hotkey-web","version":"0.2.0"},"node_modules/react":{"version":"19.2.8","integrity":"sha512-fixture"}}}`,
		"agent/uv.lock":              "version = 1\n\n[[package]]\nname = \"hotkey-agent\"\nversion = \"0.1.0\"\n\n[[package]]\nname = \"fastapi\"\nversion = \"1.0.0\"\n",
		"backend/db/schema.sql":      "CREATE TABLE release_fixture(id bigint);\n",
		"docs/openapi/swagger.json":  "{\"swagger\":\"2.0\"}\n",
		"backend/openapi/docs.go":    "package openapi\n",
		"docker-compose.yml":         "services: {}\n", "docker-compose-prod.yml": "services: {}\n",
		".env.example": "SAFE=value\n", ".env.prod.example": "SAFE=value\n", "backend/.env.example": "SAFE=value\n",
	}
	for number := 1; number <= 6; number++ {
		files["docs/operations/00"+string(rune('0'+number))+"-fixture.md"] = "---\nversion: v1.0\nstatus: planned\n---\n"
	}
	for path, body := range files {
		writeFixtureFile(t, repository, path, body)
	}
	inventory := imageInventory{
		Version: "hotkey-release-image-inventory-v1", GitRevision: strings.Repeat("a", 40),
		Applications: []imageIdentity{
			image("backend", "hotkey-server:env", '1'), image("frontend", "hotkey-web:env", '2'), image("python_agent", "hotkey-agent:env", '3'),
		},
		Upstream: []imageIdentity{
			image("postgres", "pgvector/pgvector:pg16", '4'), image("redis", "redis:latest", '5'), image("minio", "minio/minio:latest", '6'), image("minio_client", "minio/mc:latest", '7'),
		}, Differences: []string{},
	}
	payload, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	inventoryPath := filepath.Join(repository, "images.json")
	if err := os.WriteFile(inventoryPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return repository, inventoryPath
}

func image(name, reference string, digit byte) imageIdentity {
	return imageIdentity{Name: name, Reference: reference, ImageID: "sha256:" + strings.Repeat(string(digit), 64), RepoDigests: []string{imageRepository(reference) + "@sha256:" + strings.Repeat(string(digit), 64)}}
}

func writeFixtureFile(t *testing.T, root, path, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
