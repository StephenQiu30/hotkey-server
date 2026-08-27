package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	candidateRPO = 15 * time.Minute
	candidateRTO = 2 * time.Hour
)

type manifest struct {
	Version                   string        `json:"version"`
	GitRevision               string        `json:"git_revision"`
	Environment               string        `json:"environment"`
	Isolated                  bool          `json:"isolated"`
	ProductionEgressDisabled  bool          `json:"production_egress_disabled"`
	IncidentCutoffAt          time.Time     `json:"incident_cutoff_at"`
	RecoveryPointAt           time.Time     `json:"recovery_point_at"`
	DrillStartedAt            time.Time     `json:"drill_started_at"`
	ServicesReadableAt        time.Time     `json:"services_readable_at"`
	ReconciliationCompletedAt time.Time     `json:"reconciliation_completed_at"`
	Assets                    []assetResult `json:"assets"`
	Differences               []string      `json:"differences"`
}

type assetResult struct {
	Name           string `json:"name"`
	ExpectedCount  int64  `json:"expected_count"`
	ActualCount    int64  `json:"actual_count"`
	ExpectedSHA256 string `json:"expected_sha256"`
	ActualSHA256   string `json:"actual_sha256"`
}

type verification struct {
	Version         string        `json:"version"`
	Status          string        `json:"status"`
	GitRevision     string        `json:"git_revision"`
	Environment     string        `json:"environment"`
	RPOSeconds      int64         `json:"rpo_seconds"`
	RTOSeconds      int64         `json:"rto_seconds"`
	CandidateRPOMet bool          `json:"candidate_rpo_met"`
	CandidateRTOMet bool          `json:"candidate_rto_met"`
	Assets          []assetResult `json:"assets"`
	Differences     []string      `json:"differences"`
	VerifiedAt      time.Time     `json:"verified_at"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	input := strings.TrimSpace(os.Getenv("HOTKEY_RECOVERY_MANIFEST"))
	output := strings.TrimSpace(os.Getenv("HOTKEY_RECOVERY_OUTPUT"))
	if input == "" || output == "" {
		return errors.New("HOTKEY_RECOVERY_MANIFEST and HOTKEY_RECOVERY_OUTPUT are required")
	}
	payload, err := os.ReadFile(input)
	if err != nil {
		return errors.New("read recovery manifest")
	}
	var evidence manifest
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return errors.New("decode strict recovery manifest")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("recovery manifest must contain exactly one JSON object")
	}
	result, err := verify(evidence)
	if err != nil {
		return err
	}
	if err := writeExclusiveJSON(output, result); err != nil {
		return err
	}
	fmt.Printf("recovery evidence written to %s (RPO=%ds, RTO=%ds)\n", output, result.RPOSeconds, result.RTOSeconds)
	return nil
}

func verify(evidence manifest) (verification, error) {
	if evidence.Version != "hotkey-recovery-manifest-v1" || strings.TrimSpace(evidence.Environment) == "" {
		return verification{}, errors.New("recovery manifest version or environment is invalid")
	}
	if len(evidence.GitRevision) != 40 || strings.Trim(evidence.GitRevision, "0123456789abcdef") != "" {
		return verification{}, errors.New("recovery git revision must be a 40-character lowercase commit SHA")
	}
	if !evidence.Isolated || !evidence.ProductionEgressDisabled {
		return verification{}, errors.New("recovery drill must be isolated with production egress disabled")
	}
	if evidence.RecoveryPointAt.IsZero() || evidence.IncidentCutoffAt.Before(evidence.RecoveryPointAt) || evidence.DrillStartedAt.Before(evidence.IncidentCutoffAt) ||
		evidence.DrillStartedAt.IsZero() || evidence.ServicesReadableAt.Before(evidence.DrillStartedAt) ||
		evidence.ReconciliationCompletedAt.Before(evidence.ServicesReadableAt) {
		return verification{}, errors.New("recovery timeline is incomplete or out of order")
	}
	required := map[string]bool{
		"postgres_facts": false, "minio_evidence": false, "vault_all_files": false,
		"vault_manual_regions": false, "river_jobs_attempts": false,
	}
	seen := make(map[string]struct{}, len(evidence.Assets))
	for _, asset := range evidence.Assets {
		if _, exists := required[asset.Name]; !exists {
			return verification{}, fmt.Errorf("unexpected recovery asset %q", asset.Name)
		}
		if _, duplicate := seen[asset.Name]; duplicate {
			return verification{}, fmt.Errorf("duplicate recovery asset %q", asset.Name)
		}
		seen[asset.Name] = struct{}{}
		required[asset.Name] = true
		if asset.ExpectedCount < 0 || asset.ActualCount != asset.ExpectedCount || !validSHA256(asset.ExpectedSHA256) || asset.ActualSHA256 != asset.ExpectedSHA256 {
			return verification{}, fmt.Errorf("recovery asset %q does not reconcile", asset.Name)
		}
		if asset.Name == "vault_manual_regions" && asset.ExpectedCount == 0 {
			return verification{}, errors.New("recovery drill must include at least one protected manual Vault region")
		}
	}
	for name, present := range required {
		if !present {
			return verification{}, fmt.Errorf("recovery manifest is missing %q", name)
		}
	}
	if len(evidence.Differences) != 0 {
		return verification{}, errors.New("recovery manifest contains unexplained differences")
	}
	assets := append([]assetResult(nil), evidence.Assets...)
	sort.Slice(assets, func(left, right int) bool { return assets[left].Name < assets[right].Name })
	rpo := evidence.IncidentCutoffAt.Sub(evidence.RecoveryPointAt)
	rto := evidence.ServicesReadableAt.Sub(evidence.DrillStartedAt)
	return verification{
		Version: "hotkey-recovery-verification-v1", Status: "reconciled",
		GitRevision: evidence.GitRevision, Environment: evidence.Environment,
		RPOSeconds: int64(rpo / time.Second), RTOSeconds: int64(rto / time.Second),
		CandidateRPOMet: rpo <= candidateRPO, CandidateRTOMet: rto <= candidateRTO,
		Assets: assets, Differences: []string{}, VerifiedAt: time.Now().UTC(),
	}, nil
}

func validSHA256(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}

func writeExclusiveJSON(path string, value any) error {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return errors.New("recovery output path is invalid")
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o750); err != nil {
		return errors.New("create recovery evidence directory")
	}
	file, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("recovery evidence file already exists or cannot be created")
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(value)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		return errors.New("write recovery evidence")
	}
	return nil
}
