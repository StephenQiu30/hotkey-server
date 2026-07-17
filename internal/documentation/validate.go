package docgovernance

import (
	"fmt"
	"regexp"
	"sort"
)

var (
	planReferencePattern = regexp.MustCompile(`^PLAN-([0-9]{3})$`)
	validExecutionStates = map[string]bool{
		"backlog": true, "ready": true, "in_progress": true, "blocked": true, "done": true, "superseded": true,
	}
	validReviewStates = map[string]bool{
		"pending": true, "in_review": true, "approved": true, "changes_requested": true,
	}
	validAcceptanceResults = map[string]bool{
		"pending": true, "accepted": true, "rejected": true, "accepted_with_risk": true,
	}
)

// Validate applies lifecycle-only governance rules to the parsed document
// set. File parsing and Markdown-link issues remain the responsibility of
// LoadRepository so callers can report both groups together.
func (repository *Repository) Validate() []Issue {
	if repository == nil {
		return []Issue{{Path: "docs", Rule: "repository.nil", Message: "repository is required"}}
	}
	prds, plans, acceptances := documentGroups(repository.Documents)
	issues := []Issue{}
	inProgress := []Document{}
	for _, document := range repository.Documents {
		switch document.Layer {
		case "PRD":
			if document.DocNo != "000" {
				if _, exists := plans[document.DocNo]; !exists {
					issues = append(issues, Issue{Path: document.Path, Rule: "mapping.plan_missing", Message: "matching Plan is required"})
				}
			}
			issues = append(issues, validateExecutionState(document)...)
		case "Plan":
			if document.DocNo != "000" {
				if _, exists := prds[document.DocNo]; !exists {
					issues = append(issues, Issue{Path: document.Path, Rule: "mapping.prd_missing", Message: "matching PRD is required"})
				}
			}
			issues = append(issues, validateExecutionState(document)...)
			issues = append(issues, validateReviewState(document)...)
			issues = append(issues, validateReadyPlan(document, prds)...)
			if document.ExecutionStatus == "in_progress" {
				inProgress = append(inProgress, document)
			}
		case "Acceptance":
			if document.Result == "" || !validAcceptanceResults[document.Result] {
				issues = append(issues, Issue{Path: document.Path, Rule: "frontmatter.result", Message: "Acceptance result is invalid"})
			}
		}
	}

	if len(inProgress) > 1 {
		for _, document := range inProgress {
			issues = append(issues, Issue{Path: document.Path, Rule: "lifecycle.in_progress_unique", Message: "only one Plan may be in_progress"})
		}
	}
	issues = append(issues, validatePlanDependencies(plans)...)
	issues = append(issues, validateArchiveEvidence(prds, plans, acceptances)...)
	sortIssues(issues)
	return issues
}

func documentGroups(documents []Document) (map[string]Document, map[string]Document, map[string]Document) {
	prds := map[string]Document{}
	plans := map[string]Document{}
	acceptances := map[string]Document{}
	for _, document := range documents {
		if document.DocNo == "000" {
			continue
		}
		switch document.Layer {
		case "PRD":
			prds[document.DocNo] = document
		case "Plan":
			plans[document.DocNo] = document
		case "Acceptance":
			acceptances[document.DocNo] = document
		}
	}
	return prds, plans, acceptances
}

func validateExecutionState(document Document) []Issue {
	if document.ExecutionStatus == "" || !validExecutionStates[document.ExecutionStatus] {
		return []Issue{{Path: document.Path, Rule: "frontmatter.execution_status", Message: "execution_status is invalid"}}
	}
	return nil
}

func validateReviewState(document Document) []Issue {
	if document.ReviewStatus == "" || !validReviewStates[document.ReviewStatus] {
		return []Issue{{Path: document.Path, Rule: "frontmatter.review_status", Message: "review_status is invalid"}}
	}
	return nil
}

func validateReadyPlan(plan Document, prds map[string]Document) []Issue {
	if plan.ExecutionStatus != "ready" {
		return nil
	}
	issues := []Issue{}
	if plan.Status != "accepted" {
		issues = append(issues, Issue{Path: plan.Path, Rule: "lifecycle.ready_plan", Message: "ready Plan must be accepted"})
	}
	if plan.ReviewStatus != "approved" {
		issues = append(issues, Issue{Path: plan.Path, Rule: "lifecycle.ready_review", Message: "ready Plan requires approved review"})
	}
	prd, exists := prds[plan.DocNo]
	if !exists || prd.Status != "accepted" {
		issues = append(issues, Issue{Path: plan.Path, Rule: "lifecycle.ready_prd", Message: "ready Plan requires an accepted PRD"})
	}
	return issues
}

func validatePlanDependencies(plans map[string]Document) []Issue {
	issues := []Issue{}
	for _, plan := range plans {
		if plan.ExecutionStatus != "ready" {
			continue
		}
		for _, dependency := range plan.DependsOn {
			dependencyNo, valid := planReferenceNumber(dependency)
			dependencyPlan, exists := plans[dependencyNo]
			if !valid || !exists {
				issues = append(issues, Issue{Path: plan.Path, Rule: "plan.dependency_missing", Message: "dependency is missing: " + dependency})
				continue
			}
			if !isArchivedDone(dependencyPlan) {
				issues = append(issues, Issue{Path: plan.Path, Rule: "lifecycle.ready_dependency", Message: "dependency is not done: " + dependency})
			}
		}
	}

	for _, cycle := range dependencyCycles(plans) {
		for _, planNo := range cycle {
			issues = append(issues, Issue{Path: plans[planNo].Path, Rule: "plan.dependency_cycle", Message: "dependency cycle: " + joinCycle(cycle)})
		}
	}
	return issues
}

func validateArchiveEvidence(prds, plans, acceptances map[string]Document) []Issue {
	issues := []Issue{}
	for documentNo, plan := range plans {
		if !isArchivedDone(plan) {
			continue
		}
		prd, hasPRD := prds[documentNo]
		if !hasPRD || !isArchivedDone(prd) {
			issues = append(issues, Issue{Path: plan.Path, Rule: "lifecycle.archive_pair", Message: "matching PRD must be archived and done"})
		}
		acceptance, hasAcceptance := acceptances[documentNo]
		if !hasAcceptance || acceptance.Status != "accepted" || acceptance.Result != "accepted" {
			issues = append(issues, Issue{Path: plan.Path, Rule: "lifecycle.acceptance", Message: "accepted Acceptance is required before archiving"})
		}
	}
	for documentNo, prd := range prds {
		if !isArchivedDone(prd) {
			continue
		}
		plan, hasPlan := plans[documentNo]
		if !hasPlan || !isArchivedDone(plan) {
			issues = append(issues, Issue{Path: prd.Path, Rule: "lifecycle.archive_pair", Message: "matching Plan must be archived and done"})
		}
	}
	return issues
}

func isArchivedDone(document Document) bool {
	return document.Status == "archived" && document.ExecutionStatus == "done"
}

func planReferenceNumber(value string) (string, bool) {
	matches := planReferencePattern.FindStringSubmatch(value)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

func dependencyCycles(plans map[string]Document) [][]string {
	state := map[string]int{}
	stack := []string{}
	cycles := [][]string{}
	seen := map[string]bool{}
	planNumbers := make([]string, 0, len(plans))
	for planNo := range plans {
		planNumbers = append(planNumbers, planNo)
	}
	sort.Strings(planNumbers)
	var visit func(string)
	visit = func(planNo string) {
		state[planNo] = 1
		stack = append(stack, planNo)
		for _, dependency := range plans[planNo].DependsOn {
			dependencyNo, valid := planReferenceNumber(dependency)
			if !valid {
				continue
			}
			if _, exists := plans[dependencyNo]; !exists {
				continue
			}
			switch state[dependencyNo] {
			case 0:
				visit(dependencyNo)
			case 1:
				cycle := cycleFromStack(stack, dependencyNo)
				key := joinCycle(cycle)
				if !seen[key] {
					seen[key] = true
					cycles = append(cycles, cycle)
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[planNo] = 2
	}
	for _, planNo := range planNumbers {
		if state[planNo] == 0 {
			visit(planNo)
		}
	}
	return cycles
}

func cycleFromStack(stack []string, first string) []string {
	for index, value := range stack {
		if value == first {
			return append([]string(nil), stack[index:]...)
		}
	}
	return nil
}

func joinCycle(cycle []string) string {
	if len(cycle) == 0 {
		return ""
	}
	return fmt.Sprintf("PLAN-%s", cycle[0]) + " -> " + fmt.Sprintf("PLAN-%s", cycle[len(cycle)-1])
}
