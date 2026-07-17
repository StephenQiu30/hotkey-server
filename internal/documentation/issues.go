package docgovernance

import "sort"

// Issue is a deterministic, user-facing governance violation.
type Issue struct {
	Path    string
	Rule    string
	Message string
}

func sortIssues(issues []Issue) {
	sort.Slice(issues, func(left, right int) bool {
		if issues[left].Path != issues[right].Path {
			return issues[left].Path < issues[right].Path
		}
		if issues[left].Rule != issues[right].Rule {
			return issues[left].Rule < issues[right].Rule
		}
		return issues[left].Message < issues[right].Message
	})
}
