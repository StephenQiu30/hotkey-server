package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const reportVersion = "hotkey-rc-candidate-assessment-v1"

var (
	revisionPattern   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	runIDPattern      = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
	markdownLink      = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)
	errTechnicalGate  = errors.New("one or more required technical gates failed")
	requiredGateOrder = []string{
		"backend_static",
		"backend_runtime",
		"backend_vulnerability",
		"worker_recovery",
		"frontend",
		"python_agent",
		"compose",
		"browser_business_flow",
	}
	deferredCapabilities = []string{
		"comments",
		"weekly_reports",
		"additional_authorized_sources",
		"expanded_email_delivery",
		"high_availability",
	}
)

type config struct {
	RepositoryRoot           string
	OutputPath               string
	Environment              string
	Hardware                 string
	GitRevision              string
	CIRunID                  string
	ProductionEgressDisabled bool
	RequiredGates            map[string]string
}

type report struct {
	Version                  string               `json:"version"`
	Status                   string               `json:"status"`
	Approval                 string               `json:"approval"`
	Environment              string               `json:"environment"`
	Hardware                 string               `json:"hardware"`
	GitRevision              string               `json:"git_revision"`
	CIRunID                  string               `json:"ci_run_id"`
	ProductionEgressDisabled bool                 `json:"production_egress_disabled"`
	RequiredGates            []gateResult         `json:"required_gates"`
	Coverage                 coverage             `json:"coverage"`
	Documents                []documentAssessment `json:"documents"`
	P0Scope                  scopeAssessment      `json:"p0_scope"`
	Defects                  []defect             `json:"defects"`
	ReleaseReady             bool                 `json:"release_ready"`
	Differences              []string             `json:"differences"`
}

type gateResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Passed bool   `json:"passed"`
}

type coverage struct {
	FullCI            bool `json:"full_ci"`
	E2E               bool `json:"e2e"`
	Performance       bool `json:"performance"`
	Security          bool `json:"security"`
	DocumentIntegrity bool `json:"document_integrity"`
}

type documentAssessment struct {
	DocNo                string   `json:"doc_no"`
	DesignStatus         string   `json:"design_status"`
	PRDStatus            string   `json:"prd_status"`
	PlanStatus           string   `json:"plan_status"`
	AcceptanceStatus     string   `json:"acceptance_status"`
	TraceabilityComplete bool     `json:"traceability_complete"`
	BlockingACs          []string `json:"blocking_acs"`
}

type scopeAssessment struct {
	Passed                             bool     `json:"passed"`
	Baseline                           string   `json:"baseline"`
	DeferredCapabilities               []string `json:"deferred_capabilities"`
	ForbiddenInfrastructureDetected    []string `json:"forbidden_infrastructure_detected"`
	UsedDeferredCapabilitiesAsEvidence bool     `json:"used_deferred_capabilities_as_p0_evidence"`
}

type defect struct {
	ID          string   `json:"id"`
	Severity    string   `json:"severity"`
	Code        string   `json:"code"`
	Document    string   `json:"document"`
	BlockingACs []string `json:"blocking_acs"`
}

type frontmatter struct {
	DocNo         string
	Status        string
	CanonicalPath string
}

func main() {
	cfg, err := loadConfig(os.Getenv)
	if err == nil {
		err = run(cfg)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "RC candidate assessment failed:", err)
		os.Exit(1)
	}
}

func loadConfig(getenv func(string) string) (config, error) {
	gates := make(map[string]string, len(requiredGateOrder))
	for _, name := range requiredGateOrder {
		environmentName := "HOTKEY_RC_GATE_" + strings.ToUpper(name)
		gates[name] = strings.TrimSpace(getenv(environmentName))
	}
	cfg := config{
		RepositoryRoot:           strings.TrimSpace(getenv("HOTKEY_RC_REPOSITORY_ROOT")),
		OutputPath:               strings.TrimSpace(getenv("HOTKEY_RC_ASSESSMENT_OUTPUT")),
		Environment:              strings.TrimSpace(getenv("HOTKEY_RC_ASSESSMENT_ENVIRONMENT")),
		Hardware:                 strings.TrimSpace(getenv("HOTKEY_RC_ASSESSMENT_HARDWARE")),
		GitRevision:              strings.TrimSpace(getenv("HOTKEY_RC_ASSESSMENT_GIT_REVISION")),
		CIRunID:                  strings.TrimSpace(getenv("HOTKEY_RC_ASSESSMENT_CI_RUN_ID")),
		ProductionEgressDisabled: strings.TrimSpace(getenv("HOTKEY_RC_ASSESSMENT_PRODUCTION_EGRESS_DISABLED")) == "true",
		RequiredGates:            gates,
	}
	if err := validateConfig(cfg); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func validateConfig(cfg config) error {
	if !filepath.IsAbs(cfg.RepositoryRoot) || !filepath.IsAbs(cfg.OutputPath) {
		return errors.New("absolute repository and output paths are required")
	}
	info, err := os.Stat(cfg.RepositoryRoot)
	if err != nil || !info.IsDir() {
		return errors.New("repository root must be an existing directory")
	}
	if cfg.Environment == "" || cfg.Hardware == "" || len(cfg.Environment) > 128 || len(cfg.Hardware) > 256 || strings.ContainsAny(cfg.Environment+cfg.Hardware, "\r\n") {
		return errors.New("bounded environment and hardware labels are required")
	}
	if !revisionPattern.MatchString(cfg.GitRevision) || !runIDPattern.MatchString(cfg.CIRunID) {
		return errors.New("a complete revision and numeric CI run ID are required")
	}
	if !cfg.ProductionEgressDisabled {
		return errors.New("RC assessment requires production egress to be disabled")
	}
	if len(cfg.RequiredGates) != len(requiredGateOrder) {
		return errors.New("the exact required technical gate set is required")
	}
	for _, name := range requiredGateOrder {
		status, ok := cfg.RequiredGates[name]
		if !ok {
			return fmt.Errorf("required technical gate %s is missing", name)
		}
		switch status {
		case "success", "failure", "cancelled", "skipped":
		default:
			return fmt.Errorf("required technical gate %s has invalid status", name)
		}
	}
	return nil
}

func run(cfg config) error {
	result, err := assess(cfg)
	if err != nil {
		return err
	}
	if err := writeExclusiveJSON(cfg.OutputPath, result); err != nil {
		return err
	}
	if result.Status == "failed" {
		return errTechnicalGate
	}
	return nil
}

func assess(cfg config) (report, error) {
	if err := validateConfig(cfg); err != nil {
		return report{}, err
	}
	gates, technicalGreen := assessGates(cfg.RequiredGates)
	documents, defects, err := inspectDocuments(cfg.RepositoryRoot)
	if err != nil {
		return report{}, fmt.Errorf("document integrity: %w", err)
	}
	scope, err := inspectScope(cfg.RepositoryRoot)
	if err != nil {
		return report{}, fmt.Errorf("P0 scope integrity: %w", err)
	}

	coverage := coverage{
		FullCI:            technicalGreen,
		E2E:               cfg.RequiredGates["browser_business_flow"] == "success",
		Performance:       cfg.RequiredGates["browser_business_flow"] == "success",
		Security:          cfg.RequiredGates["backend_vulnerability"] == "success" && cfg.RequiredGates["python_agent"] == "success" && cfg.RequiredGates["browser_business_flow"] == "success",
		DocumentIntegrity: true,
	}
	if !technicalGreen {
		for _, gate := range gates {
			if !gate.Passed {
				defects = append(defects, defect{
					ID: "RC-CI-" + strings.ToUpper(strings.ReplaceAll(gate.Name, "_", "-")), Severity: "P0",
					Code: "technical_gate_not_success", Document: ".github/workflows/ci.yml", BlockingACs: []string{"AC-005-008"},
				})
			}
		}
	}
	sort.Slice(defects, func(left, right int) bool { return defects[left].ID < defects[right].ID })
	releaseReady := technicalGreen && scope.Passed && len(defects) == 0
	status := "candidate"
	if !technicalGreen || !scope.Passed {
		status = "failed"
	} else if !releaseReady {
		status = "blocked"
	}
	return report{
		Version: reportVersion, Status: status, Approval: "automated_assessment_not_release_approval",
		Environment: cfg.Environment, Hardware: cfg.Hardware, GitRevision: cfg.GitRevision, CIRunID: cfg.CIRunID,
		ProductionEgressDisabled: true, RequiredGates: gates, Coverage: coverage, Documents: documents,
		P0Scope: scope, Defects: defects, ReleaseReady: releaseReady, Differences: []string{},
	}, nil
}

func assessGates(statuses map[string]string) ([]gateResult, bool) {
	results := make([]gateResult, 0, len(requiredGateOrder))
	allPassed := true
	for _, name := range requiredGateOrder {
		passed := statuses[name] == "success"
		allPassed = allPassed && passed
		results = append(results, gateResult{Name: name, Status: statuses[name], Passed: passed})
	}
	return results, allPassed
}

func inspectDocuments(repositoryRoot string) ([]documentAssessment, []defect, error) {
	documents := make([]documentAssessment, 0, 5)
	defects := make([]defect, 0, 5)
	for number := 1; number <= 5; number++ {
		docNo := fmt.Sprintf("%03d", number)
		design, designMeta, designBody, err := requiredDocument(repositoryRoot, "design", docNo)
		if err != nil {
			return nil, nil, err
		}
		prd, prdMeta, prdBody, err := requiredDocument(repositoryRoot, "prd", docNo)
		if err != nil {
			return nil, nil, err
		}
		plan, planMeta, planBody, err := requiredDocument(repositoryRoot, "plans", docNo)
		if err != nil {
			return nil, nil, err
		}
		_ = design
		_ = designBody

		acceptance, acceptanceMeta, acceptanceBody, found, err := optionalDocument(repositoryRoot, "acceptance", docNo)
		if err != nil {
			return nil, nil, err
		}
		traceability := strings.Contains(prdBody, "`AC-"+docNo+"-")
		for _, prefix := range []string{"`TASK-" + docNo + "-", "`SPEC-" + docNo + "-", "`CHK-" + docNo + "-", "`AC-" + docNo + "-"} {
			traceability = traceability && strings.Contains(planBody, prefix)
		}
		acceptanceStatus := "missing"
		blockingACs := []string{}
		if found {
			acceptanceStatus = acceptanceMeta.Status
			traceability = traceability && strings.Contains(acceptanceBody, "`AC-"+docNo+"-")
			blockingACs = blockingAcceptanceIDs(docNo, acceptanceBody)
		} else {
			traceability = false
		}
		documents = append(documents, documentAssessment{
			DocNo: docNo, DesignStatus: designMeta.Status, PRDStatus: prdMeta.Status, PlanStatus: planMeta.Status,
			AcceptanceStatus: acceptanceStatus, TraceabilityComplete: traceability, BlockingACs: blockingACs,
		})
		switch {
		case !found:
			defects = append(defects, defect{
				ID: "RC-" + docNo + "-ACCEPTANCE-MISSING", Severity: "P0", Code: "acceptance_missing",
				Document: filepath.ToSlash(filepath.Join("docs", "acceptance", docNo+"-*.md")), BlockingACs: []string{},
			})
		case acceptanceMeta.Status != "passed":
			defects = append(defects, defect{
				ID: "RC-" + docNo + "-ACCEPTANCE-NOT-PASSED", Severity: "P0", Code: "acceptance_not_passed",
				Document: relativePath(repositoryRoot, acceptance), BlockingACs: blockingACs,
			})
		}
		if found && acceptanceMeta.DocNo != docNo {
			return nil, nil, fmt.Errorf("%s has mismatched doc_no", relativePath(repositoryRoot, acceptance))
		}
		if designMeta.Status != "accepted" || (prdMeta.Status != "approved" && prdMeta.Status != "implemented") {
			return nil, nil, fmt.Errorf("%s design/PRD lifecycle is not implementation-ready", docNo)
		}
		if planMeta.Status != "in_progress" && planMeta.Status != "completed" {
			return nil, nil, fmt.Errorf("%s plan lifecycle is not executable", docNo)
		}
		if found && acceptanceMeta.Status == "passed" && !traceability {
			return nil, nil, fmt.Errorf("%s passed acceptance has incomplete AC/TASK/SPEC/CHK traceability", docNo)
		}
		_ = prd
		_ = plan
	}
	return documents, defects, nil
}

func requiredDocument(repositoryRoot, category, docNo string) (string, frontmatter, string, error) {
	path, meta, body, found, err := optionalDocument(repositoryRoot, category, docNo)
	if err != nil {
		return "", frontmatter{}, "", err
	}
	if !found {
		return "", frontmatter{}, "", fmt.Errorf("missing docs/%s/%s document", category, docNo)
	}
	return path, meta, body, nil
}

func optionalDocument(repositoryRoot, category, docNo string) (string, frontmatter, string, bool, error) {
	pattern := filepath.Join(repositoryRoot, "docs", category, docNo+"-*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", frontmatter{}, "", false, err
	}
	if len(matches) == 0 {
		return "", frontmatter{}, "", false, nil
	}
	if len(matches) != 1 {
		return "", frontmatter{}, "", false, fmt.Errorf("docs/%s must contain exactly one %s document", category, docNo)
	}
	path := matches[0]
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", frontmatter{}, "", false, err
	}
	body := string(payload)
	meta, err := parseAndValidateDocument(repositoryRoot, path, body, docNo)
	if err != nil {
		return "", frontmatter{}, "", false, err
	}
	return path, meta, body, true, nil
}

func parseAndValidateDocument(repositoryRoot, path, body, docNo string) (frontmatter, error) {
	lines := strings.Split(body, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return frontmatter{}, fmt.Errorf("%s is missing frontmatter", relativePath(repositoryRoot, path))
	}
	values := map[string]string{}
	frontmatterEnd := -1
	for index, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			frontmatterEnd = index + 1
			break
		}
		key, value, found := strings.Cut(line, ":")
		if found {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	if frontmatterEnd < 0 {
		return frontmatter{}, fmt.Errorf("%s has unclosed frontmatter", relativePath(repositoryRoot, path))
	}
	relative := relativePath(repositoryRoot, path)
	meta := frontmatter{DocNo: values["doc_no"], Status: values["status"], CanonicalPath: values["canonical_path"]}
	if meta.DocNo != docNo || meta.Status == "" || meta.CanonicalPath != relative {
		return frontmatter{}, fmt.Errorf("%s has invalid doc_no, status, or canonical_path", relative)
	}
	if err := validateMarkdown(repositoryRoot, path, lines[frontmatterEnd+1:]); err != nil {
		return frontmatter{}, err
	}
	return meta, nil
}

func validateMarkdown(repositoryRoot, path string, lines []string) error {
	fences := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fences++
		}
	}
	if fences%2 != 0 {
		return fmt.Errorf("%s has an unclosed code fence", relativePath(repositoryRoot, path))
	}
	body := strings.Join(lines, "\n")
	for _, match := range markdownLink.FindAllStringSubmatch(body, -1) {
		target := strings.TrimSpace(strings.Trim(match[1], "<>"))
		if space := strings.IndexAny(target, " \t"); space >= 0 {
			target = target[:space]
		}
		target = strings.SplitN(target, "#", 2)[0]
		if target == "" || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "app://") {
			continue
		}
		candidate := target
		if strings.HasPrefix(target, "docs/") {
			candidate = filepath.Join(repositoryRoot, filepath.FromSlash(target))
		} else if !filepath.IsAbs(target) {
			candidate = filepath.Join(filepath.Dir(path), filepath.FromSlash(target))
		}
		if _, err := os.Stat(candidate); err != nil {
			return fmt.Errorf("%s links to missing local target %s", relativePath(repositoryRoot, path), target)
		}
	}
	return nil
}

func blockingAcceptanceIDs(docNo, body string) []string {
	pattern := regexp.MustCompile(`(?m)^\|\s*` + "`" + `(AC-` + regexp.QuoteMeta(docNo) + `-[0-9]{3})` + "`" + `\s*\|\s*` + "`?" + `(partial|failed|blocked)` + "`?" + `\s*\|`)
	matches := pattern.FindAllStringSubmatch(body, -1)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		result = append(result, match[1]+":"+match[2])
	}
	sort.Strings(result)
	return result
}

func inspectScope(repositoryRoot string) (scopeAssessment, error) {
	forbidden := map[string][]string{
		"backend/go.mod": {
			"github.com/segmentio/kafka-go", "github.com/IBM/sarama", "go.temporal.io/sdk",
			"github.com/elastic/go-elasticsearch", "github.com/opensearch-project/opensearch-go",
		},
		"frontend/package.json": {"kafkajs", "@temporalio/", "@elastic/elasticsearch", "@opensearch-project/"},
		"agent/pyproject.toml":  {"kafka-python", "aiokafka", "temporalio", "opensearch-py", `"elasticsearch`},
	}
	detected := []string{}
	for relative, tokens := range forbidden {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relative)))
		if err != nil {
			return scopeAssessment{}, fmt.Errorf("read %s: %w", relative, err)
		}
		for _, token := range tokens {
			if strings.Contains(string(payload), token) {
				detected = append(detected, relative+":"+token)
			}
		}
	}
	for _, relative := range []string{"docker-compose.yml", "docker-compose-prod.yml"} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, relative))
		if err != nil {
			return scopeAssessment{}, fmt.Errorf("read %s: %w", relative, err)
		}
		services := composeServices(string(payload))
		for _, service := range services {
			switch service {
			case "kafka", "zookeeper", "temporal", "elasticsearch", "opensearch", "keycloak":
				detected = append(detected, relative+":"+service)
			}
		}
	}
	sort.Strings(detected)
	if len(detected) > 0 {
		return scopeAssessment{}, fmt.Errorf("forbidden P1 infrastructure detected: %s", strings.Join(detected, ", "))
	}
	return scopeAssessment{
		Passed: true, Baseline: "single_host_compose_without_ha_dependency",
		DeferredCapabilities: append([]string(nil), deferredCapabilities...), ForbiddenInfrastructureDetected: []string{},
		UsedDeferredCapabilitiesAsEvidence: false,
	}, nil
}

func composeServices(body string) []string {
	insideServices := false
	result := []string{}
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "services:" && !strings.HasPrefix(line, " ") {
			insideServices = true
			continue
		}
		if !insideServices {
			continue
		}
		if line != "" && !strings.HasPrefix(line, " ") {
			break
		}
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") && strings.HasSuffix(strings.TrimSpace(line), ":") {
			result = append(result, strings.TrimSuffix(strings.TrimSpace(line), ":"))
		}
	}
	return result
}

func relativePath(repositoryRoot, path string) string {
	relative, err := filepath.Rel(repositoryRoot, path)
	if err != nil {
		return filepath.ToSlash(filepath.Base(path))
	}
	return filepath.ToSlash(relative)
}

func writeExclusiveJSON(path string, value report) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
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
	if _, err := file.Write(encoded); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	remove = false
	return nil
}
