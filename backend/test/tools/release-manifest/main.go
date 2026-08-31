package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const manifestVersion = "hotkey-release-manifest-v1"

var (
	revisionPattern   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256Pattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	repoDigestPattern = regexp.MustCompile(`^[^\s@]+@sha256:[0-9a-f]{64}$`)
	labelPattern      = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	versionPattern    = regexp.MustCompile(`(?m)^version:[[:space:]]*([^[:space:]]+)[[:space:]]*$`)
)

type releaseConfig struct {
	RepositoryRoot            string
	ImageInventoryPath        string
	GitRevision               string
	CIRunID                   string
	Environment               string
	ProductionEgressDisabled  bool
	VulnerabilityGateStatuses map[string]string
}

type releaseManifest struct {
	Version                  string                `json:"version"`
	Status                   string                `json:"status"`
	Approval                 string                `json:"approval"`
	Environment              string                `json:"environment"`
	CIRunID                  string                `json:"ci_run_id"`
	GitRevision              string                `json:"git_revision"`
	ProductionEgressDisabled bool                  `json:"production_egress_disabled"`
	ImageInventorySHA256     string                `json:"image_inventory_sha256"`
	Applications             []imageIdentity       `json:"application_images"`
	UpstreamImages           []imageIdentity       `json:"upstream_images"`
	DependencyLocks          []fileDigest          `json:"dependency_locks"`
	Contracts                contractDigests       `json:"contracts"`
	Configurations           []fileDigest          `json:"configurations"`
	SBOM                     sbomSummary           `json:"sbom"`
	VulnerabilityResults     []vulnerabilityResult `json:"vulnerability_results"`
	Operations               []operationDigest     `json:"operations"`
	Rebuild                  rebuildContract       `json:"rebuild"`
	ReleaseReady             bool                  `json:"release_ready"`
	PendingReleaseApprovals  []string              `json:"pending_release_approvals"`
	Differences              []string              `json:"differences"`
}

type imageInventory struct {
	Version      string          `json:"version"`
	GitRevision  string          `json:"git_revision"`
	Applications []imageIdentity `json:"applications"`
	Upstream     []imageIdentity `json:"upstream"`
	Differences  []string        `json:"differences"`
}

type imageIdentity struct {
	Name        string   `json:"name"`
	Reference   string   `json:"reference"`
	ImageID     string   `json:"image_id"`
	RepoDigests []string `json:"repo_digests"`
}

type fileDigest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type contractDigests struct {
	Schema           fileDigest `json:"schema"`
	OpenAPI          fileDigest `json:"openapi"`
	GeneratedOpenAPI fileDigest `json:"generated_openapi"`
}

type sbomSummary struct {
	Format         string `json:"format"`
	SpecVersion    string `json:"spec_version"`
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	ComponentCount int    `json:"component_count"`
}

type vulnerabilityResult struct {
	Ecosystem string `json:"ecosystem"`
	Gate      string `json:"gate"`
	Scanner   string `json:"scanner"`
	Status    string `json:"status"`
	Passed    bool   `json:"passed"`
}

type operationDigest struct {
	DocNo   string `json:"doc_no"`
	Version string `json:"version"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
}

type rebuildContract struct {
	HashAlgorithm             string `json:"hash_algorithm"`
	DependencyLocksRequired   bool   `json:"dependency_locks_required"`
	ImmutableImageIDsRequired bool   `json:"immutable_image_ids_required"`
	GeneratedContractsPinned  bool   `json:"generated_contracts_pinned"`
	ConfigurationExamplesOnly bool   `json:"configuration_examples_only"`
}

type cyclonedxBOM struct {
	BOMFormat   string          `json:"bomFormat"`
	SpecVersion string          `json:"specVersion"`
	Version     int             `json:"version"`
	Metadata    sbomMetadata    `json:"metadata"`
	Components  []sbomComponent `json:"components"`
}

type sbomMetadata struct {
	Component sbomComponent `json:"component"`
}

type sbomComponent struct {
	Type    string `json:"type"`
	Group   string `json:"group,omitempty"`
	Name    string `json:"name"`
	Version string `json:"version"`
	BOMRef  string `json:"bom-ref"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "release manifest generation failed")
		os.Exit(1)
	}
}

func run() error {
	config := releaseConfig{
		RepositoryRoot:           strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_REPOSITORY_ROOT")),
		ImageInventoryPath:       strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_IMAGE_INVENTORY")),
		GitRevision:              strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_GIT_REVISION")),
		CIRunID:                  strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_CI_RUN_ID")),
		Environment:              strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_ENVIRONMENT")),
		ProductionEgressDisabled: strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_PRODUCTION_EGRESS_DISABLED")) == "true",
		VulnerabilityGateStatuses: map[string]string{
			"backend":      strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_GATE_BACKEND_VULNERABILITY")),
			"frontend":     strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_GATE_FRONTEND_VULNERABILITY")),
			"python_agent": strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_GATE_AGENT_VULNERABILITY")),
		},
	}
	manifestPath := strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_MANIFEST_OUTPUT"))
	sbomPath := strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_SBOM_OUTPUT"))
	digestPath := strings.TrimSpace(os.Getenv("HOTKEY_RELEASE_MANIFEST_DIGEST_OUTPUT"))
	if err := validateConfig(config, manifestPath, sbomPath, digestPath); err != nil {
		return err
	}
	result, bom, assessmentErr := buildReleaseEvidence(config)
	result.SBOM.Path = filepath.Base(sbomPath)
	bomPayload, err := marshalJSON(bom)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(bomPayload)
	result.SBOM.SHA256 = hex.EncodeToString(digest[:])
	if err := writeReleaseOutputs(manifestPath, sbomPath, digestPath, result, bom); err != nil {
		return err
	}
	return assessmentErr
}

func validateConfig(config releaseConfig, outputs ...string) error {
	if !filepath.IsAbs(config.RepositoryRoot) || !filepath.IsAbs(config.ImageInventoryPath) || !revisionPattern.MatchString(config.GitRevision) ||
		!labelPattern.MatchString(config.CIRunID) || !labelPattern.MatchString(config.Environment) || !config.ProductionEgressDisabled {
		return errors.New("complete isolated release configuration is required")
	}
	seen := map[string]bool{}
	for _, output := range outputs {
		if !filepath.IsAbs(output) || seen[output] {
			return errors.New("three distinct absolute release output paths are required")
		}
		seen[output] = true
	}
	return nil
}

func buildReleaseEvidence(config releaseConfig) (releaseManifest, cyclonedxBOM, error) {
	result := releaseManifest{
		Version: manifestVersion, Status: "verified", Approval: "automated_material_verification_not_release_approval",
		Environment: config.Environment, CIRunID: config.CIRunID, GitRevision: config.GitRevision,
		ProductionEgressDisabled: config.ProductionEgressDisabled,
		Applications:             []imageIdentity{}, UpstreamImages: []imageIdentity{}, DependencyLocks: []fileDigest{},
		Configurations: []fileDigest{}, VulnerabilityResults: []vulnerabilityResult{}, Operations: []operationDigest{},
		Rebuild:                 rebuildContract{HashAlgorithm: "sha256", DependencyLocksRequired: true, ImmutableImageIDsRequired: true, GeneratedContractsPinned: true, ConfigurationExamplesOnly: true},
		ReleaseReady:            false,
		PendingReleaseApprovals: []string{"remaining_p0_acceptance", "upper_bound_capacity", "uat_signoff", "release_observation"},
		Differences:             []string{},
	}

	inventory, inventorySHA, err := readImageInventory(config.ImageInventoryPath)
	if err != nil {
		result.Differences = append(result.Differences, "image_inventory_invalid")
	} else {
		result.ImageInventorySHA256 = inventorySHA
		result.Applications = inventory.Applications
		result.UpstreamImages = inventory.Upstream
		if inventory.GitRevision != config.GitRevision {
			result.Differences = append(result.Differences, "image_inventory_revision_mismatch")
		}
		if inventory.Version != "hotkey-release-image-inventory-v1" || len(inventory.Differences) != 0 ||
			!validImages(inventory.Applications, expectedApplicationImages(), false) || !validImages(inventory.Upstream, expectedUpstreamImages(), true) {
			result.Differences = append(result.Differences, "image_inventory_contract_mismatch")
		}
	}

	for _, path := range []string{"backend/go.mod", "backend/go.sum", "frontend/package-lock.json", "agent/uv.lock"} {
		appendDigest(config.RepositoryRoot, path, &result.DependencyLocks, &result.Differences)
	}
	result.Contracts.Schema = requiredDigest(config.RepositoryRoot, "backend/db/schema.sql", &result.Differences)
	result.Contracts.OpenAPI = requiredDigest(config.RepositoryRoot, "docs/openapi/swagger.json", &result.Differences)
	result.Contracts.GeneratedOpenAPI = requiredDigest(config.RepositoryRoot, "backend/openapi/docs.go", &result.Differences)
	for _, path := range []string{"docker-compose.yml", "docker-compose-prod.yml", ".env.example", ".env.prod.example", "backend/.env.example"} {
		appendDigest(config.RepositoryRoot, path, &result.Configurations, &result.Differences)
	}

	bom, sbomErr := buildSBOM(config.RepositoryRoot, config.GitRevision)
	if sbomErr != nil {
		result.Differences = append(result.Differences, "sbom_generation_failed")
		bom = cyclonedxBOM{BOMFormat: "CycloneDX", SpecVersion: "1.6", Version: 1, Components: []sbomComponent{}}
	}
	bomPayload, marshalErr := marshalJSON(bom)
	if marshalErr != nil {
		result.Differences = append(result.Differences, "sbom_encoding_failed")
	} else {
		digest := sha256.Sum256(bomPayload)
		result.SBOM = sbomSummary{Format: "CycloneDX", SpecVersion: "1.6", Path: "release-sbom.cdx.json", SHA256: hex.EncodeToString(digest[:]), ComponentCount: len(bom.Components)}
	}

	scanners := map[string]string{"backend": "govulncheck@v1.6.0", "frontend": "npm-audit-production", "python_agent": "pip-audit-locked"}
	for _, ecosystem := range []string{"backend", "frontend", "python_agent"} {
		status := config.VulnerabilityGateStatuses[ecosystem]
		passed := status == "success"
		result.VulnerabilityResults = append(result.VulnerabilityResults, vulnerabilityResult{Ecosystem: ecosystem, Gate: ecosystem + "_vulnerability", Scanner: scanners[ecosystem], Status: status, Passed: passed})
		if !passed {
			result.Differences = append(result.Differences, ecosystem+"_vulnerability_gate_"+safeStatus(status))
		}
	}

	operations, operationsErr := operationDigests(config.RepositoryRoot)
	if operationsErr != nil {
		result.Differences = append(result.Differences, "operations_versions_incomplete")
	} else {
		result.Operations = operations
	}
	if len(result.Differences) > 0 {
		result.Status = "blocked"
		return result, bom, errors.New("release materials contain differences")
	}
	return result, bom, nil
}

func readImageInventory(path string) (imageInventory, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return imageInventory{}, "", err
	}
	defer func() { _ = file.Close() }()
	limited := io.LimitReader(file, 128*1024+1)
	payload, err := io.ReadAll(limited)
	if err != nil || len(payload) > 128*1024 {
		return imageInventory{}, "", errors.New("image inventory is unreadable or too large")
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	var inventory imageInventory
	if err := decoder.Decode(&inventory); err != nil {
		return imageInventory{}, "", err
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return imageInventory{}, "", errors.New("image inventory contains trailing content")
	}
	digest := sha256.Sum256(payload)
	return inventory, hex.EncodeToString(digest[:]), nil
}

func validImages(images []imageIdentity, expected map[string]string, requireRepoDigest bool) bool {
	if len(images) != len(expected) {
		return false
	}
	seen := map[string]bool{}
	for _, image := range images {
		if seen[image.Name] || expected[image.Name] != image.Reference || !sha256Pattern.MatchString(image.ImageID) {
			return false
		}
		seen[image.Name] = true
		if requireRepoDigest && len(image.RepoDigests) == 0 {
			return false
		}
		for _, digest := range image.RepoDigests {
			if !repoDigestPattern.MatchString(digest) || !strings.HasPrefix(digest, imageRepository(image.Reference)+"@sha256:") {
				return false
			}
		}
	}
	return true
}

func imageRepository(reference string) string {
	lastSlash := strings.LastIndex(reference, "/")
	lastColon := strings.LastIndex(reference, ":")
	if lastColon > lastSlash {
		return reference[:lastColon]
	}
	return reference
}

func expectedApplicationImages() map[string]string {
	return map[string]string{"backend": "hotkey-server:env", "frontend": "hotkey-web:env", "python_agent": "hotkey-agent:env"}
}

func expectedUpstreamImages() map[string]string {
	return map[string]string{"postgres": "pgvector/pgvector:pg16", "redis": "redis:latest", "minio": "minio/minio:latest", "minio_client": "minio/mc:latest"}
}

func appendDigest(root, path string, values *[]fileDigest, differences *[]string) {
	value := requiredDigest(root, path, differences)
	if value.SHA256 != "" {
		*values = append(*values, value)
	}
}

func requiredDigest(root, path string, differences *[]string) fileDigest {
	digest, err := hashFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		*differences = append(*differences, "missing_or_unreadable:"+path)
		return fileDigest{Path: path}
	}
	return fileDigest{Path: path, SHA256: digest}
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func operationDigests(root string) ([]operationDigest, error) {
	values := make([]operationDigest, 0, 6)
	for number := 1; number <= 6; number++ {
		docNo := fmt.Sprintf("%03d", number)
		matches, err := filepath.Glob(filepath.Join(root, "docs", "operations", docNo+"-*.md"))
		if err != nil || len(matches) != 1 {
			return nil, errors.New("operation document set is incomplete")
		}
		payload, err := os.ReadFile(matches[0])
		if err != nil {
			return nil, err
		}
		match := versionPattern.FindStringSubmatch(string(payload))
		if len(match) != 2 {
			return nil, errors.New("operation version is missing")
		}
		digest := sha256.Sum256(payload)
		relative, err := filepath.Rel(root, matches[0])
		if err != nil {
			return nil, err
		}
		values = append(values, operationDigest{DocNo: docNo, Version: match[1], Path: filepath.ToSlash(relative), SHA256: hex.EncodeToString(digest[:])})
	}
	return values, nil
}

func buildSBOM(root, revision string) (cyclonedxBOM, error) {
	components := map[string]sbomComponent{}
	add := func(ecosystem, componentType, name, version string) {
		name, version = strings.TrimSpace(name), strings.TrimSpace(version)
		if name == "" || version == "" {
			return
		}
		ref := ecosystem + ":" + name + "@" + version
		components[ref] = sbomComponent{Type: componentType, Group: ecosystem, Name: name, Version: version, BOMRef: ref}
	}

	goSum, err := os.Open(filepath.Join(root, "backend", "go.sum"))
	if err != nil {
		return cyclonedxBOM{}, err
	}
	scanner := bufio.NewScanner(goSum)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			add("golang", "library", fields[0], strings.TrimSuffix(fields[1], "/go.mod"))
		}
	}
	closeErr := goSum.Close()
	if scanner.Err() != nil {
		return cyclonedxBOM{}, scanner.Err()
	}
	if closeErr != nil {
		return cyclonedxBOM{}, closeErr
	}

	npmPayload, err := os.ReadFile(filepath.Join(root, "frontend", "package-lock.json"))
	if err != nil {
		return cyclonedxBOM{}, err
	}
	var npmLock struct {
		Packages map[string]struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(npmPayload, &npmLock); err != nil {
		return cyclonedxBOM{}, err
	}
	for location, pkg := range npmLock.Packages {
		name := pkg.Name
		if name == "" && location != "" {
			if index := strings.LastIndex(location, "node_modules/"); index >= 0 {
				name = location[index+len("node_modules/"):]
			}
		}
		componentType := "library"
		if location == "" {
			componentType = "application"
		}
		add("npm", componentType, name, pkg.Version)
	}

	uvPayload, err := os.ReadFile(filepath.Join(root, "agent", "uv.lock"))
	if err != nil {
		return cyclonedxBOM{}, err
	}
	var name, version string
	flush := func() { add("pypi", "library", name, version); name, version = "", "" }
	for _, line := range strings.Split(string(uvPayload), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[[package]]" {
			flush()
			continue
		}
		if strings.HasPrefix(trimmed, "name = \"") && strings.HasSuffix(trimmed, "\"") {
			name = strings.TrimSuffix(strings.TrimPrefix(trimmed, "name = \""), "\"")
		}
		if strings.HasPrefix(trimmed, "version = \"") && strings.HasSuffix(trimmed, "\"") {
			version = strings.TrimSuffix(strings.TrimPrefix(trimmed, "version = \""), "\"")
		}
	}
	flush()

	ordered := make([]sbomComponent, 0, len(components))
	for _, component := range components {
		ordered = append(ordered, component)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].BOMRef < ordered[j].BOMRef })
	if len(ordered) == 0 {
		return cyclonedxBOM{}, errors.New("SBOM contains no components")
	}
	return cyclonedxBOM{
		BOMFormat: "CycloneDX", SpecVersion: "1.6", Version: 1,
		Metadata:   sbomMetadata{Component: sbomComponent{Type: "application", Group: "hotkey", Name: "hotkey-single-host-compose", Version: revision, BOMRef: "hotkey:compose@" + revision}},
		Components: ordered,
	}, nil
}

func safeStatus(status string) string {
	if labelPattern.MatchString(status) {
		return status
	}
	return "missing"
}

func writeReleaseOutputs(manifestPath, sbomPath, digestPath string, manifest releaseManifest, bom cyclonedxBOM) error {
	paths := []string{manifestPath, sbomPath, digestPath}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return errors.New("release output paths must be absolute")
		}
		if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
			return errors.New("release output already exists or cannot be inspected")
		}
	}
	manifestPayload, err := marshalJSON(manifest)
	if err != nil {
		return err
	}
	sbomPayload, err := marshalJSON(bom)
	if err != nil {
		return err
	}
	manifestDigest := sha256.Sum256(manifestPayload)
	digestPayload := []byte(hex.EncodeToString(manifestDigest[:]) + "  " + filepath.Base(manifestPath) + "\n")

	created := []string{}
	complete := false
	defer func() {
		if !complete {
			for _, path := range created {
				_ = os.Remove(path)
			}
		}
	}()
	for _, item := range []struct {
		path    string
		payload []byte
	}{{sbomPath, sbomPayload}, {manifestPath, manifestPayload}, {digestPath, digestPayload}} {
		if err := writePrivateExclusive(item.path, item.payload); err != nil {
			return err
		}
		created = append(created, item.path)
	}
	complete = true
	return nil
}

func marshalJSON(value any) ([]byte, error) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func writePrivateExclusive(path string, payload []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(payload); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	remove = false
	return nil
}
