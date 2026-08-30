package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	reportVersion          = "hotkey-planned-runbook-dry-run-v1"
	repositorySecretCanary = "runbook-secret-canary-991"
)

var (
	revisionPattern   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	statusPattern     = regexp.MustCompile(`(?m)^status:[[:space:]]*([^[:space:]]+)[[:space:]]*$`)
	bashBlockPattern  = regexp.MustCompile("(?s)```(?:bash|sh)\\s*(.*?)```")
	makeTargetPattern = regexp.MustCompile(`(?m)^([a-z0-9][a-z0-9-]*):`)
)

type runbookContract struct {
	ID             string
	Path           string
	CommandMarkers []string
}

type report struct {
	Version                  string          `json:"version"`
	Status                   string          `json:"status"`
	Approval                 string          `json:"approval"`
	Environment              string          `json:"environment"`
	Hardware                 string          `json:"hardware"`
	GitRevision              string          `json:"git_revision"`
	Isolated                 bool            `json:"isolated"`
	ProductionEgressDisabled bool            `json:"production_egress_disabled"`
	DryRunScope              string          `json:"dry_run_scope"`
	ActivationEligible       bool            `json:"activation_eligible"`
	PendingG6Activation      []string        `json:"pending_g6_activation"`
	Runbooks                 []runbookResult `json:"runbooks"`
	Differences              []string        `json:"differences"`
}

type runbookResult struct {
	ID                      string   `json:"id"`
	Document                string   `json:"document"`
	Status                  string   `json:"status"`
	ActivationEligible      bool     `json:"activation_eligible"`
	ParametersPresent       bool     `json:"parameters_present"`
	IsolationPresent        bool     `json:"isolation_present"`
	CopyableCommandsPresent bool     `json:"copyable_commands_present"`
	ExpectedOutputPresent   bool     `json:"expected_output_present"`
	StopPointsPresent       bool     `json:"stop_points_present"`
	RollbackPresent         bool     `json:"rollback_present"`
	AuditPresent            bool     `json:"audit_present"`
	EntrypointsVerified     bool     `json:"entrypoints_verified"`
	Differences             []string `json:"differences"`
}

var runbookContracts = []runbookContract{
	{ID: "001", Path: "docs/operations/001-部署升级与回滚.md", CommandMarkers: []string{"docker compose -f docker-compose.yml config --quiet", "docker compose --env-file .env.prod -f docker-compose-prod.yml config --quiet"}},
	{ID: "002", Path: "docs/operations/002-备份恢复与重建.md", CommandMarkers: []string{"make repeated-restore-rehearsal-acceptance"}},
	{ID: "003", Path: "docs/operations/003-来源授权预算与故障处置.md", CommandMarkers: []string{"make source-live-smoke"}},
	{ID: "004", Path: "docs/operations/004-可观测性SLO与事件响应.md", CommandMarkers: []string{"make observability-alert-acceptance"}},
	{ID: "005", Path: "docs/operations/005-保留删除与撤权处置.md", CommandMarkers: []string{"make retention-rehearsal-acceptance"}},
	{ID: "006", Path: "docs/operations/006-密钥轮换与泄漏响应.md", CommandMarkers: []string{"make secret-rotation-acceptance"}},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "planned Runbook dry-run failed")
		os.Exit(1)
	}
}

func run() error {
	output := strings.TrimSpace(os.Getenv("HOTKEY_RUNBOOK_DRY_RUN_OUTPUT"))
	environment := strings.TrimSpace(os.Getenv("HOTKEY_RUNBOOK_DRY_RUN_ENVIRONMENT"))
	hardware := strings.TrimSpace(os.Getenv("HOTKEY_RUNBOOK_DRY_RUN_HARDWARE"))
	revision := strings.TrimSpace(os.Getenv("HOTKEY_RUNBOOK_DRY_RUN_GIT_REVISION"))
	repository := strings.TrimSpace(os.Getenv("HOTKEY_RUNBOOK_DRY_RUN_REPOSITORY_ROOT"))
	productionEgressDisabled := strings.TrimSpace(os.Getenv("HOTKEY_RUNBOOK_DRY_RUN_PRODUCTION_EGRESS_DISABLED")) == "true"
	if repository == "" {
		var err error
		repository, err = inferRepositoryRoot()
		if err != nil {
			return err
		}
	}
	if output == "" || !filepath.IsAbs(output) || !filepath.IsAbs(repository) || environment == "" || hardware == "" || !revisionPattern.MatchString(revision) || !productionEgressDisabled {
		return errors.New("complete isolated dry-run configuration is required")
	}
	if strings.ContainsAny(environment+hardware, "\r\n") || len(environment) > 128 || len(hardware) > 128 {
		return errors.New("dry-run labels are invalid")
	}

	result := report{
		Version: reportVersion, Status: "verified", Approval: "required",
		Environment: environment, Hardware: hardware, GitRevision: revision,
		Isolated: true, ProductionEgressDisabled: true,
		DryRunScope:         "contract_and_repository_entrypoint_resolution_without_operational_side_effects",
		ActivationEligible:  false,
		PendingG6Activation: []string{"001", "002", "003", "004", "005", "006"},
		Runbooks:            []runbookResult{}, Differences: []string{},
	}
	for _, contract := range runbookContracts {
		assessment := inspectRunbook(repository, contract)
		result.Runbooks = append(result.Runbooks, assessment)
		for _, difference := range assessment.Differences {
			result.Differences = append(result.Differences, contract.ID+":"+difference)
		}
	}
	if len(result.Differences) > 0 {
		result.Status = "failed"
	}
	if err := writeExclusiveReport(output, result); err != nil {
		return err
	}
	if result.Status != "verified" {
		return errors.New("one or more planned Runbook contracts are incomplete")
	}
	return nil
}

func inspectRunbook(repository string, contract runbookContract) runbookResult {
	result := runbookResult{
		ID: contract.ID, Document: contract.Path, ActivationEligible: false,
		Differences: []string{},
	}
	payload, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(contract.Path)))
	if err != nil {
		result.Differences = append(result.Differences, "document_unreadable")
		return result
	}
	body := string(payload)
	if match := statusPattern.FindStringSubmatch(body); len(match) == 2 {
		result.Status = strings.TrimSpace(match[1])
	}
	result.ParametersPresent = strings.Contains(body, "参数") || strings.Contains(body, "变量")
	result.IsolationPresent = strings.Contains(body, "隔离")
	blocks := bashBlockPattern.FindAllStringSubmatch(body, -1)
	result.CopyableCommandsPresent = len(blocks) > 0
	result.ExpectedOutputPresent = strings.Contains(body, "预期输出") || strings.Contains(body, "成功判据") || strings.Contains(body, "成功与停止") || strings.Contains(body, "成功、停止")
	result.StopPointsPresent = strings.Contains(body, "停止")
	result.RollbackPresent = strings.Contains(body, "回滚") || strings.Contains(body, "回退") || strings.Contains(body, "恢复")
	result.AuditPresent = strings.Contains(body, "审计") || strings.Contains(body, "证据")
	result.EntrypointsVerified = entrypointsVerified(repository, blocks, contract.CommandMarkers)

	checks := []struct {
		passed bool
		code   string
	}{
		{result.Status == "planned", "status_must_remain_planned"},
		{result.ParametersPresent, "parameters_missing"},
		{result.IsolationPresent, "isolation_boundary_missing"},
		{result.CopyableCommandsPresent, "copyable_commands_missing"},
		{result.ExpectedOutputPresent, "expected_output_or_success_criteria_missing"},
		{result.StopPointsPresent, "stop_points_missing"},
		{result.RollbackPresent, "rollback_or_recovery_missing"},
		{result.AuditPresent, "audit_evidence_missing"},
		{result.EntrypointsVerified, "entrypoint_unverified"},
	}
	for _, check := range checks {
		if !check.passed {
			result.Differences = append(result.Differences, check.code)
		}
	}
	return result
}

func entrypointsVerified(repository string, blocks [][]string, markers []string) bool {
	commands := ""
	for _, block := range blocks {
		if len(block) == 2 {
			commands += block[1] + "\n"
		}
	}
	targets := map[string]bool{}
	needsMakefile := false
	for _, marker := range markers {
		needsMakefile = needsMakefile || strings.HasPrefix(marker, "make ")
	}
	if needsMakefile {
		makefile, err := os.ReadFile(filepath.Join(repository, "backend", "Makefile"))
		if err != nil {
			return false
		}
		for _, match := range makeTargetPattern.FindAllStringSubmatch(string(makefile), -1) {
			targets[match[1]] = true
		}
	}
	for _, marker := range markers {
		if !strings.Contains(commands, marker) {
			return false
		}
		fields := strings.Fields(marker)
		if len(fields) >= 2 && fields[0] == "make" && !targets[fields[1]] {
			return false
		}
		if strings.HasPrefix(marker, "docker compose") {
			compose := "docker-compose.yml"
			if strings.Contains(marker, "docker-compose-prod.yml") {
				compose = "docker-compose-prod.yml"
			}
			if info, err := os.Stat(filepath.Join(repository, compose)); err != nil || !info.Mode().IsRegular() {
				return false
			}
		}
	}
	return true
}

func inferRepositoryRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for candidate := current; ; candidate = filepath.Dir(candidate) {
		if info, statErr := os.Stat(filepath.Join(candidate, "docs", "operations")); statErr == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", errors.New("repository root not found")
		}
	}
}

func writeExclusiveReport(path string, value report) error {
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
