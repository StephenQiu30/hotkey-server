package architecture_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func Test003PartialAnd004CompletedAcceptancesRecordHonestGates(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	tests := []struct {
		path           string
		docNo          string
		expectedStatus string
		statusFragment string
		checkFragment  string
		indexLink      string
	}{
		{
			path:           "docs/acceptance/003-智能研判事件热度与人工治理验收.md",
			docNo:          "003",
			expectedStatus: "failed",
			statusFragment: "| `AC-003-003` | `partial` |",
			checkFragment:  "`CHK-003-G5-001` 保持未勾选",
			indexLink:      "[部分验收](003-智能研判事件热度与人工治理验收.md)",
		},
		{
			path:           "docs/acceptance/004-通知报告知识投影与检索验收.md",
			docNo:          "004",
			expectedStatus: "passed",
			statusFragment: "| `AC-004-008` | `passed` |",
			checkFragment:  "当前结论：`passed`",
			indexLink:      "[验收完成](004-通知报告知识投影与检索验收.md)",
		},
	}
	index := readRepositoryFile(t, repository, "docs/acceptance/README.md")
	for _, test := range tests {
		t.Run(test.docNo, func(t *testing.T) {
			acceptance := readRepositoryFile(t, repository, test.path)
			if status := frontmatterStatus(t, filepath.Join(repository, test.path)); status != test.expectedStatus {
				t.Errorf("%s Acceptance status = %q, want %s", test.docNo, status, test.expectedStatus)
			}
			if !regexp.MustCompile(`(?m)^verified_revision: "[0-9a-f]{40}"$`).MatchString(acceptance) {
				t.Errorf("%s Acceptance must pin a complete verified revision", test.docNo)
			}
			for number := 1; number <= 8; number++ {
				acceptanceID := "AC-" + test.docNo + "-00" + string(rune('0'+number))
				if !strings.Contains(acceptance, "| `"+acceptanceID+"` |") {
					t.Errorf("%s Acceptance is missing %s status", test.docNo, acceptanceID)
				}
			}
			if !strings.Contains(acceptance, test.statusFragment) || !strings.Contains(acceptance, test.checkFragment) {
				t.Errorf("%s Acceptance does not preserve its current gate result", test.docNo)
			}
			if !strings.Contains(index, test.indexLink) || !strings.Contains(index, test.docNo+" |") {
				t.Errorf("Acceptance index does not expose the %s result", test.docNo)
			}
		})
	}
}
