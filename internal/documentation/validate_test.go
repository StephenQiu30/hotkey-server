package docgovernance_test

import (
	"testing"

	documentation "github.com/StephenQiu30/hotkey-server/internal/documentation"
)

func TestValidateLifecycleReportsMappingDependencyAndLifecycleViolations(t *testing.T) {
	repository := &documentation.Repository{Documents: []documentation.Document{
		{Path: "docs/prd/010.md", Layer: "PRD", DocNo: "010", Status: "archived", ExecutionStatus: "done"},
		{Path: "docs/plans/010.md", Layer: "Plan", DocNo: "010", Status: "archived", ExecutionStatus: "done", ReviewStatus: "approved"},
		{Path: "docs/acceptance/010.md", Layer: "Acceptance", DocNo: "010", Status: "review", Result: "pending"},
		{Path: "docs/prd/011.md", Layer: "PRD", DocNo: "011", Status: "accepted", ExecutionStatus: "ready"},
		{Path: "docs/plans/011.md", Layer: "Plan", DocNo: "011", Status: "accepted", ExecutionStatus: "in_progress", ReviewStatus: "approved", DependsOn: []string{"PLAN-010"}},
		{Path: "docs/prd/012.md", Layer: "PRD", DocNo: "012", Status: "accepted", ExecutionStatus: "ready"},
		{Path: "docs/plans/012.md", Layer: "Plan", DocNo: "012", Status: "accepted", ExecutionStatus: "ready", ReviewStatus: "approved", DependsOn: []string{"PLAN-011"}},
		{Path: "docs/prd/013.md", Layer: "PRD", DocNo: "013", Status: "accepted", ExecutionStatus: "ready"},
		{Path: "docs/plans/013.md", Layer: "Plan", DocNo: "013", Status: "accepted", ExecutionStatus: "in_progress", ReviewStatus: "approved", DependsOn: []string{"PLAN-014"}},
		{Path: "docs/prd/014.md", Layer: "PRD", DocNo: "014", Status: "accepted", ExecutionStatus: "ready"},
		{Path: "docs/plans/014.md", Layer: "Plan", DocNo: "014", Status: "accepted", ExecutionStatus: "ready", ReviewStatus: "approved", DependsOn: []string{"PLAN-013"}},
		{Path: "docs/prd/015.md", Layer: "PRD", DocNo: "015", Status: "accepted", ExecutionStatus: "ready"},
		{Path: "docs/prd/016.md", Layer: "PRD", DocNo: "016", Status: "review", ExecutionStatus: "ready"},
		{Path: "docs/plans/016.md", Layer: "Plan", DocNo: "016", Status: "accepted", ExecutionStatus: "ready", ReviewStatus: "pending"},
	}}

	issues := repository.Validate()
	for _, rule := range []string{"mapping.plan_missing", "lifecycle.acceptance", "lifecycle.in_progress_unique", "lifecycle.ready_dependency", "lifecycle.ready_prd", "lifecycle.ready_review", "plan.dependency_cycle"} {
		if !hasRule(issues, rule) {
			t.Errorf("Validate() issues = %#v, want rule %q", issues, rule)
		}
	}
}

func TestValidateAcceptsArchivedPlanWithAcceptedEvidence(t *testing.T) {
	repository := &documentation.Repository{Documents: []documentation.Document{
		{Path: "docs/prd/006.md", Layer: "PRD", DocNo: "006", Status: "archived", ExecutionStatus: "done"},
		{Path: "docs/plans/006.md", Layer: "Plan", DocNo: "006", Status: "archived", ExecutionStatus: "done", ReviewStatus: "approved"},
		{Path: "docs/acceptance/006.md", Layer: "Acceptance", DocNo: "006", Status: "accepted", Result: "accepted"},
	}}

	if issues := repository.Validate(); len(issues) != 0 {
		t.Fatalf("Validate() issues = %#v, want none", issues)
	}
}
