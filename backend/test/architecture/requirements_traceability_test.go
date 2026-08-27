package architecture_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	requirementIDPattern = regexp.MustCompile("`(?:FR|NFR)-[0-9]{3}-[0-9]{3}`")
	acceptanceIDPattern  = regexp.MustCompile("`AC-[0-9]{3}-[0-9]{3}`")
	taskIDPattern        = regexp.MustCompile("`TASK-[0-9]{3}-S[0-9]{2}-T[0-9]{2}`")
	specIDPattern        = regexp.MustCompile("`SPEC-[0-9]{3}-[A-Z]+-[0-9]{3}`")
	checklistIDPattern   = regexp.MustCompile("`CHK-[0-9]{3}-G[0-9]-[0-9]{3}`")
)

func TestP0RequirementsHaveCompletePRDToPlanTraceability(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	for number := 1; number <= 5; number++ {
		docNo := fmt.Sprintf("%03d", number)
		t.Run(docNo, func(t *testing.T) {
			prdPath := oneNumberedDocument(t, filepath.Join(repository, "docs", "prd"), docNo)
			planPath := oneNumberedDocument(t, filepath.Join(repository, "docs", "plans"), docNo)
			prd := readRepositoryFile(t, repository, relativeDocumentPath(t, repository, prdPath))
			plan := readRepositoryFile(t, repository, relativeDocumentPath(t, repository, planPath))

			requirements := ownedMarkdownIDs(docNo, uniqueMarkdownIDs(requirementIDPattern, prd))
			acceptance := ownedMarkdownIDs(docNo, uniqueMarkdownIDs(acceptanceIDPattern, markdownSection(t, prd, "## 验收标准", "## 需求追踪")))
			trace := markdownSectionToEnd(prd, "## 需求追踪")
			if len(requirements) == 0 || len(acceptance) == 0 {
				t.Fatalf("PRD %s has no requirements or acceptance criteria", docNo)
			}
			assertRequirementTraceRows(t, docNo, requirements, acceptance, trace)

			tasks := markdownSection(t, plan, "## 测试先行任务", "## 实施规格")
			specs := markdownSection(t, plan, "## 实施规格", "## 执行检查清单")
			checks := markdownSection(t, plan, "## 执行检查清单", "## 验证命令")
			assertPlanArtifactRows(t, docNo, "TASK", taskIDPattern, tasks, acceptance)
			assertPlanArtifactRows(t, docNo, "SPEC", specIDPattern, specs, acceptance)
			assertPlanArtifactRows(t, docNo, "CHK", checklistIDPattern, checks, acceptance)
			for _, ac := range acceptance {
				for layer, section := range map[string]string{"TASK": tasks, "SPEC": specs, "CHK": checks} {
					if !strings.Contains(section, "`"+ac+"`") {
						t.Errorf("%s has no %s mapping for %s", docNo, layer, ac)
					}
				}
			}
		})
	}
}

func assertRequirementTraceRows(t *testing.T, docNo string, requirements, acceptance []string, trace string) {
	t.Helper()
	accepted := sliceSet(acceptance)
	for _, requirement := range requirements {
		row := markdownRowContaining(t, trace, "`"+requirement+"`")
		mapped := uniqueMarkdownIDs(acceptanceIDPattern, row)
		if len(mapped) == 0 {
			if strings.Contains(row, "P1") && strings.Contains(row, "专项验收") {
				continue
			}
			t.Errorf("%s P0 requirement %s has no AC in its trace row", docNo, requirement)
		}
		for _, ac := range mapped {
			if !accepted[ac] {
				t.Errorf("%s requirement %s maps to undefined %s", docNo, requirement, ac)
			}
		}
	}
	for _, ac := range acceptance {
		if !strings.Contains(trace, "`"+ac+"`") {
			t.Errorf("%s acceptance %s is not reachable from an FR/NFR trace row", docNo, ac)
		}
	}
}

func assertPlanArtifactRows(t *testing.T, docNo, kind string, pattern *regexp.Regexp, section string, acceptance []string) {
	t.Helper()
	seen := map[string]bool{}
	definedAcceptance := sliceSet(acceptance)
	for _, line := range strings.Split(section, "\n") {
		ids := uniqueMarkdownIDs(pattern, line)
		if len(ids) == 0 {
			continue
		}
		if len(ids) != 1 {
			t.Errorf("%s %s row must define exactly one ID: %s", docNo, kind, line)
			continue
		}
		id := ids[0]
		if seen[id] {
			t.Errorf("%s defines duplicate %s %s", docNo, kind, id)
		}
		seen[id] = true
		if !strings.Contains(id, "-"+docNo+"-") {
			t.Errorf("%s %s row defines foreign ID %s", docNo, kind, id)
		}
		mappedAcceptance := uniqueMarkdownIDs(acceptanceIDPattern, line)
		if len(ownedMarkdownIDs(docNo, mappedAcceptance)) == 0 {
			t.Errorf("%s %s %s has no AC mapping on the same row", docNo, kind, id)
		}
		for _, ac := range ownedMarkdownIDs(docNo, mappedAcceptance) {
			if !definedAcceptance[ac] {
				t.Errorf("%s %s %s maps to undefined %s", docNo, kind, id, ac)
			}
		}
	}
	if len(seen) == 0 {
		t.Errorf("%s plan has no %s artifacts", docNo, kind)
	}
}

func markdownSection(t *testing.T, document, start, end string) string {
	t.Helper()
	startIndex := strings.Index(document, start)
	if startIndex < 0 {
		t.Fatalf("document is missing section %q", start)
	}
	endIndex := strings.Index(document[startIndex+len(start):], end)
	if endIndex < 0 {
		t.Fatalf("document is missing section %q after %q", end, start)
	}
	return document[startIndex : startIndex+len(start)+endIndex]
}

func markdownSectionToEnd(document, start string) string {
	startIndex := strings.Index(document, start)
	if startIndex < 0 {
		return ""
	}
	return document[startIndex:]
}

func markdownRowContaining(t *testing.T, section, fragment string) string {
	t.Helper()
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "|") && strings.Contains(line, fragment) {
			return line
		}
	}
	t.Errorf("traceability table is missing row for %s", fragment)
	return ""
}

func uniqueMarkdownIDs(pattern *regexp.Regexp, value string) []string {
	seen := map[string]struct{}{}
	for _, raw := range pattern.FindAllString(value, -1) {
		seen[strings.Trim(raw, "`")] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func sliceSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func ownedMarkdownIDs(docNo string, values []string) []string {
	owned := make([]string, 0, len(values))
	for _, value := range values {
		if strings.Contains(value, "-"+docNo+"-") {
			owned = append(owned, value)
		}
	}
	return owned
}

func relativeDocumentPath(t *testing.T, repository, path string) string {
	t.Helper()
	relative, err := filepath.Rel(repository, path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(relative)
}
