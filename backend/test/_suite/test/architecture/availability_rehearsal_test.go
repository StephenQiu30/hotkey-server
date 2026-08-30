package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestG5AvailabilityRehearsalUsesAnIsolatedSingleHostContract(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	measurement := readRepositoryFile(t, repository, "frontend/test/operations/availability-rehearsal.mjs")
	for _, fragment := range []string{
		`hotkey-single-host-availability-rehearsal-v1`,
		`conservative_all_probes_per_observation_minute`,
		`maintenance_window: "none"`,
		`candidateTargetPercent = 99.5`,
		`calendarWindowDays = 30`,
		`release_ready: false`,
		`candidate_methodology_not_production_sla`,
		`config.composeProject === "hotkey"`,
		`postgres_unavailable`,
		`redis_unavailable`,
		`minio_unavailable`,
		`worker_unavailable`,
		`writeFileSync(output, payload, { flag: "wx", mode: 0o600 })`,
	} {
		if !strings.Contains(measurement, fragment) {
			t.Errorf("availability measurement lost %q", fragment)
		}
	}

	runner := readRepositoryFile(t, repository, "frontend/test/operations/run-availability-rehearsal.mjs")
	for _, fragment := range []string{
		"const apiContainer = `${composeProject}-availability-api`",
		"const workerContainer = `${composeProject}-availability-worker`",
		`probesPerMinute: 6`,
		`await compose("stop", "hotkey-web", "hotkey-server")`,
		`"--role", "api"`,
		`"--role", "worker"`,
		`await restoreFreshStack()`,
	} {
		if !strings.Contains(runner, fragment) {
			t.Errorf("availability runner lost %q", fragment)
		}
	}

	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")
	for _, fragment := range []string{
		"Measure isolated single-host availability and dependency attribution",
		"node frontend/test/operations/run-availability-rehearsal.mjs",
		"HOTKEY_AVAILABILITY_COMPOSE_PROJECT: hotkey-ci",
		"HOTKEY_AVAILABILITY_OUTPUT: /tmp/hotkey-availability.json",
		"/tmp/hotkey-availability.json",
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("availability CI convention is missing %q", fragment)
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/005-安全运维质量与交付计划.md")
	row := markdownChecklistRow(t, plan, "CHK-005-G5-006")
	if !strings.HasPrefix(row, "- [x]") {
		t.Errorf("approved fixed-environment availability evidence did not close G5-006: %s", row)
	}
}
