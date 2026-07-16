# Task execution governance implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the accepted task lifecycle, evidence validation, executable smoke gate, archived-document migration, and PLAN-006 readiness gate for HotKey Server.

**Architecture:** Add a repository-only Go validator under `tests/documentation` so the same checks run locally and in GitHub Actions without a second runtime. Keep document parsing, repository rules, and Git evidence resolution behind focused interfaces. Add a live HTTP smoke script as an explicit opt-in target, then update existing governance documents and review only PLAN-006 for readiness.

**Tech Stack:** Go 1.26.3, `go.yaml.in/yaml/v3`, POSIX shell, GNU Make, GitHub Actions, Markdown frontmatter, Git.

## Global constraints

- Scope is `hotkey-server`; do not modify `hotkey-web` or `hotkey-miniapp`.
- `db/schema.sql` remains the only database schema source of truth.
- `docs/openapi/swagger.json` remains the generated public API source of truth.
- Only one formal Plan may have `execution_status: in_progress`.
- Only Acceptance with `result: accepted` permits `archived + done`.
- Archived PRD and Plan files stay at their `canonical_path`.
- Plan changes require independent review before `review_status: approved`.
- Use test-first commits and Chinese commit subjects required by `AGENTS.md`.
- Do not activate PLAN-007–017 during this implementation plan.

---

### Task 1: Parse formal documents and report structural issues

**Files:**

- Create: `tests/documentation/validator.go`
- Create: `tests/documentation/validator_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**

- Consumes: repository root containing `docs/design`, `docs/prd`, `docs/plans`, `docs/acceptance`, and `docs/operations`
- Produces: `LoadRepository(root string, resolver CommitResolver) (*Repository, error)`
- Produces: `(*Repository).Validate() []Issue`
- Produces: `Issue{Path string, Rule string, Message string}` with stable sorting by path, rule, and message
- Produces: `CommitResolver.Resolve(root, revision string) error` for evidence validation without hard-coding process execution in parser tests

- [ ] **Step 1: Add the YAML dependency as a direct test dependency**

Move `go.yaml.in/yaml/v3 v3.0.4` from the indirect block to the direct `require` block. Do not add another YAML library.

- [ ] **Step 2: Write failing parser and structural tests**

Add table-driven tests that create temporary repositories and assert exact issues:

```go
func TestValidateStructuralMetadata(t *testing.T) {
	tests := []struct {
		name string
		files map[string]string
		want []Issue
	}{
		{
			name: "missing canonical path",
			files: map[string]string{
				"docs/design/001-design.md": designFrontmatter(""),
			},
			want: []Issue{{
				Path: "docs/design/001-design.md",
				Rule: "frontmatter.required",
				Message: "missing canonical_path",
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writeRepository(t, test.files)
			repository, err := LoadRepository(root, acceptingResolver{})
			if err != nil { t.Fatal(err) }
			if diff := diffIssues(repository.Validate(), test.want); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
```

Cover missing frontmatter, invalid YAML, missing required fields, invalid enum values, duplicate `doc_no`, duplicate `canonical_path`, canonical path mismatch, broken relative Markdown links, and `%20` URL decoding.

- [ ] **Step 3: Run the focused test and confirm RED**

Run: `go test ./tests/documentation -run 'TestValidateStructuralMetadata|TestValidateMarkdownLinks' -count=1`

Expected: FAIL because `LoadRepository`, `Issue`, and helpers do not exist.

- [ ] **Step 4: Implement the document model and parser**

Use these concrete types in `validator.go`:

```go
type Document struct {
	Path            string
	Layer           string
	DocNo           string
	CanonicalPath   string
	Status          string
	ExecutionStatus string
	ReviewStatus    string
	Result          string
	EvidenceCommits []string
	Inputs          []string
	Downstream      []string
	DependsOn       []string
	MarkdownLinks   []string
}

type Issue struct {
	Path    string
	Rule    string
	Message string
}

type CommitResolver interface {
	Resolve(root, revision string) error
}
```

Split the first `---` frontmatter block, decode with `yaml.v3` using known fields, normalize repository-relative paths with `filepath.Clean`, and decode Markdown URL paths with `url.PathUnescape`. Reject absolute paths and links that escape the repository root.

- [ ] **Step 5: Run parser and structural tests and confirm GREEN**

Run: `go test ./tests/documentation -run 'TestValidateStructuralMetadata|TestValidateMarkdownLinks' -count=1`

Expected: PASS with no network, Git, or database access.

- [ ] **Step 6: Format and commit Task 1**

Run:

```bash
gofmt -w tests/documentation/validator.go tests/documentation/validator_test.go
go mod tidy
go test ./tests/documentation -count=1
git diff --check
git add go.mod go.sum tests/documentation/validator.go tests/documentation/validator_test.go
git commit -m "test: 建立正式文档结构校验"
```

Expected: commit contains only the YAML dependency promotion and documentation validator tests/implementation.

---

### Task 2: Validate lifecycle, dependencies, indexes, commands, and Git evidence

**Files:**

- Modify: `tests/documentation/validator.go`
- Modify: `tests/documentation/validator_test.go`
- Create: `tests/documentation/repository_test.go`

**Interfaces:**

- Consumes: parsed `Document` values from Task 1
- Produces: rules `mapping.prd_plan`, `dependency.exists`, `dependency.cycle`, `lifecycle.state`, `acceptance.required`, `acceptance.commit`, `index.state`, and `command.exists`
- Produces: `GitCommitResolver` implemented with `git cat-file -e <sha>^{commit}`
- Produces: `TestRepositoryDocumentationGovernance`, the single repository-level quality gate

- [ ] **Step 1: Write failing lifecycle matrix tests**

Cover these exact invalid combinations:

```go
var invalidLifecycleCases = []struct {
	name string
	prd  Document
	plan Document
	acceptance *Document
}{
	{name: "ready without accepted PRD", prd: prd("review", "ready"), plan: plan("accepted", "ready", "approved")},
	{name: "ready without approved Plan", prd: prd("accepted", "ready"), plan: plan("accepted", "ready", "pending")},
	{name: "done without acceptance", prd: prd("archived", "done"), plan: plan("archived", "done", "approved")},
	{name: "done with rejected acceptance", prd: prd("archived", "done"), plan: plan("archived", "done", "approved"), acceptance: acceptance("rejected")},
}
```

Also test a valid `archived + done + approved + accepted Acceptance` combination.

- [ ] **Step 2: Write failing dependency, index, command, and evidence tests**

Add fixtures for:

- missing `PLAN-999`
- a `PLAN-006 -> PLAN-007 -> PLAN-006` cycle
- a ready Plan whose dependency is not done
- PRD without same-number Plan
- done Plan without same-number Acceptance
- Acceptance without one full 40-character `evidence_commits` SHA
- unresolved full SHA through a rejecting fake resolver
- README row status that disagrees with frontmatter
- `make unknown-target` in an accepted/ready Plan
- `sh scripts/missing.sh` in an accepted/ready Plan

- [ ] **Step 3: Run lifecycle tests and confirm RED**

Run: `go test ./tests/documentation -run 'TestValidateLifecycle|TestValidateDependencies|TestValidateEvidence|TestValidateIndex|TestValidateCommands' -count=1`

Expected: FAIL because repository-level rules are not implemented.

- [ ] **Step 4: Implement lifecycle and dependency validation**

Build maps keyed by layer and `doc_no`. Validate PRD/Plan one-to-one mapping, resolve `PLAN-NNN`, and use depth-first traversal with `visiting` and `visited` sets to report cycles. Enforce ready and done invariants from Design-015.

- [ ] **Step 5: Implement index, command, and evidence validation**

Parse Markdown table rows only from each layer README. Compare the document and execution states printed in the row with frontmatter. Inspect commands only for Plan documents in `accepted`, `ready`, `in_progress`, `done`, or `archived` states. Resolve `make` targets from Makefile labels and `sh` paths from the repository.

Require `evidence_commits` to contain at least one lowercase 40-character hexadecimal SHA when Acceptance has `result: accepted`. Resolve every SHA with `CommitResolver`.

- [ ] **Step 6: Add the repository-level test**

```go
func TestRepositoryDocumentationGovernance(t *testing.T) {
	root := repositoryRoot(t)
	repository, err := LoadRepository(root, GitCommitResolver{})
	if err != nil { t.Fatal(err) }
	for _, issue := range repository.Validate() {
		t.Errorf("%s [%s] %s", issue.Path, issue.Rule, issue.Message)
	}
}
```

The first run against the current repository is expected to fail and becomes the migration RED for Task 4.

- [ ] **Step 7: Run focused validator tests and confirm GREEN**

Run: `go test ./tests/documentation -run 'TestValidate' -count=1`

Expected: fixture tests PASS; `TestRepositoryDocumentationGovernance` may still FAIL only on current repository migration issues.

- [ ] **Step 8: Commit Task 2**

Run:

```bash
gofmt -w tests/documentation/validator.go tests/documentation/validator_test.go tests/documentation/repository_test.go
go test ./tests/documentation -run 'TestValidate' -count=1
git diff --check
git add tests/documentation
git commit -m "impl: 校验计划状态依赖与验收证据"
```

Expected: fixture-level validator behavior is green; repository migration remains an explicit failing integration test.

---

### Task 3: Add repository gates and a live HTTP smoke target

**Files:**

- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Create: `scripts/smoke.sh`
- Create: `tests/scripts/smoke_test.sh`
- Modify: `docs/operations/001-本地与GitHub CI质量门禁.md`

**Interfaces:**

- Consumes: `go test ./tests/documentation -count=1`
- Produces: `make docs-validate`
- Consumes: `HOTKEY_SMOKE_BASE_URL`, for example `http://127.0.0.1:8080`
- Produces: `make smoke`, which validates live `/healthz`, `/readyz`, and `/api/v1/capabilities`
- Produces: GitHub Actions full history so evidence SHAs can resolve

- [ ] **Step 1: Write the failing smoke script contract test**

Create a POSIX shell test that starts a temporary local HTTP fixture, returns Result JSON for the three required paths, runs `scripts/smoke.sh`, then changes one response to `code: 90001` and asserts non-zero exit.

Run: `sh tests/scripts/smoke_test.sh`

Expected: FAIL because `scripts/smoke.sh` does not exist.

- [ ] **Step 2: Add failing Make target assertions**

Extend the documentation validator fixture tests to require `docs-validate` and `smoke` targets referenced by accepted plans.

Run: `go test ./tests/documentation -run TestValidateCommands -count=1`

Expected: FAIL against the current Makefile because `smoke` and `docs-validate` do not exist.

- [ ] **Step 3: Implement the live smoke script**

Implement `scripts/smoke.sh` with POSIX `sh`, `curl`, a temporary directory, and `trap` cleanup. Require `HOTKEY_SMOKE_BASE_URL`, request each path with a timeout, require HTTP 200, and require the JSON body to contain numeric `"code":0`, a string `"message"`, and a `"data"` field. Do not start the application or embed credentials in the script.

- [ ] **Step 4: Wire Make and CI**

Add:

```make
.PHONY: docs-validate smoke

docs-validate:
	$(GO) test ./tests/documentation -count=1

smoke:
	sh tests/scripts/smoke_test.sh
	test -n "$$HOTKEY_SMOKE_BASE_URL"
	sh scripts/smoke.sh
```

Add `docs-validate` to `ci` before build. Set `fetch-depth: 0` on `actions/checkout` so historical evidence commits resolve.

- [ ] **Step 5: Update the CI operations guide**

Document that `make ci` includes governance validation. Document that `make smoke` requires an already running disposable Server and `HOTKEY_SMOKE_BASE_URL`; it is not part of CI because CI does not start a long-lived application.

- [ ] **Step 6: Run and commit Task 3**

Run:

```bash
sh tests/scripts/smoke_test.sh
go test ./tests/documentation -run TestValidateCommands -count=1
git diff --check
git add Makefile .github/workflows/ci.yml scripts/smoke.sh tests/scripts/smoke_test.sh docs/operations/001-本地与GitHub\ CI质量门禁.md
git commit -m "ci: 接入文档治理与运行时冒烟门禁"
```

Expected: script contract test and command validator pass. Live `make smoke` remains opt-in until a disposable Server is running.

---

### Task 4: Migrate archived evidence and repair governance conflicts

**Files:**

- Modify: `docs/acceptance/001-模块化单体启动与工程门禁验收.md`
- Modify: `docs/acceptance/002-单一Schema与数据库平台验收.md`
- Modify: `docs/acceptance/003-HTTP契约安全与可观测基础验收.md`
- Modify: `docs/acceptance/004-身份认证会话与权限验收.md`
- Modify: `docs/acceptance/005-监控主题规则与来源配置验收.md`
- Modify: `docs/README.md`
- Modify: `docs/acceptance/README.md`
- Modify: `docs/plans/README.md`
- Modify: `docs/plans/013-Cron与River主链路编排计划.md`
- Modify: `docs/plans/017-运行治理容量与端到端验收计划.md`
- Modify: `docs/operations/README.md`
- Test: `tests/documentation/repository_test.go`

**Interfaces:**

- Consumes: repository integration RED from Task 2
- Produces: full `evidence_commits` arrays for Acceptance 001–005
- Produces: unique Operations path `docs/operations/002-本地运行与恢复操作.md` reserved by PLAN-017
- Produces: current-file-aware PLAN-017 execution actions
- Produces: repository documentation governance GREEN

- [ ] **Step 1: Record the repository migration RED**

Run: `go test ./tests/documentation -run TestRepositoryDocumentationGovernance -count=1`

Expected: FAIL because Acceptance 001–005 do not yet provide the required full `evidence_commits` list. Record any additional validator issues verbatim before changing documents.

- [ ] **Step 2: Add full evidence commits to Acceptance 001–005**

Use these exact values:

```yaml
evidence_commits:
  - 26820fcf0d209492dc3c500d205881ecf06e8603
```

```yaml
evidence_commits:
  - fba5be0833bf6fdfc6c04579589aaba8653937f0
```

```yaml
evidence_commits:
  - 1f3709e81b60b50735249ee452964a86e975f8d9
```

```yaml
evidence_commits:
  - ae601bb55bd3c57aee92d7445b868ddc281e8d1d
```

```yaml
evidence_commits:
  - da1d66729513feae1aef801dae1abac10f61d05a
```

Keep existing historical range fields because their body text still references them.

- [ ] **Step 3: Document the state and evidence rules in governance indexes**

Update `docs/README.md`, `docs/plans/README.md`, and `docs/acceptance/README.md` to link Design-015 and define `evidence_commits`. State that archived files remain at canonical paths and only `result: accepted` unlocks done.

- [ ] **Step 4: Repair PLAN-013 and PLAN-017 command/path conflicts**

Keep `make smoke` in their validation tables because Task 3 makes it executable. Change PLAN-017 Operations output to `docs/operations/002-本地运行与恢复操作.md`. Change existing Operations audit files from broad “Create” globs to exact Modify rows, and list only new retention/reconciliation/query files as Create rows.

- [ ] **Step 5: Clarify Schema work wording**

In PLAN-013 and PLAN-017, replace generic Schema creation claims with explicit validation or incremental constraint/index work against the existing complete target Schema. Do not remove target tables already present in `db/schema.sql`.

- [ ] **Step 6: Run repository governance and confirm GREEN**

Run:

```bash
go test ./tests/documentation -count=1
make docs-validate
git diff --check
```

Expected: PASS. The validator reports no lifecycle, reference, command, index, or evidence issues.

- [ ] **Step 7: Commit Task 4**

Run:

```bash
git add docs tests/documentation/repository_test.go
git commit -m "docs: 迁移归档证据并修复计划治理冲突"
```

Expected: the commit contains only governance migrations and validator integration adjustments.

---

### Task 5: Complete the governance baseline and prepare PLAN-006 review

**Files:**

- Modify: `docs/superpowers/plans/2026-07-16-task-execution-governance.md`
- Modify: `.github/pull_request_template.md`
- Test: all files from Tasks 1–4

**Interfaces:**

- Consumes: completed Tasks 1–4
- Produces: a review checklist that includes Design/PRD/Plan states, RED/GREEN evidence, API runtime evidence, full SHA evidence, and original-path archive confirmation
- Produces: clean governance baseline ready for independent review

- [ ] **Step 1: Extend the Pull Request reviewer checklist**

Add checkboxes for:

- Design and PRD accepted before Plan ready
- dependency Plans done
- schema/records/OpenAPI synchronization reviewed
- Acceptance uses full `evidence_commits`
- runtime HTTP evidence included when public APIs change
- archived PRD/Plan remain at canonical paths

- [ ] **Step 2: Run the complete local governance verification**

Run:

```bash
go test ./tests/documentation -count=1
sh tests/scripts/smoke_test.sh
make docs-validate
git diff --check
```

Expected: all commands PASS.

- [ ] **Step 3: Run the repository quality gate**

Run with disposable services:

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_governance_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
make ci
make clean
```

Expected: OpenAPI has no drift; vet, database runtime, all Go tests, build, architecture, repository, Schema, and documentation governance pass. `make clean` removes the `hotkey` binary.

- [ ] **Step 4: Inspect the final diff and commit Task 5**

Run:

```bash
git diff --check
git status --short
git add .github/pull_request_template.md docs/superpowers/plans/2026-07-16-task-execution-governance.md
git commit -m "docs: 完成任务治理执行基线"
```

Expected: working tree is clean after the commit.

- [ ] **Step 5: Start the separate PLAN-006 readiness cycle**

Read Design-005, Design-012, Design-014, PRD-006, PLAN-006, current Source/Monitor code, target Schema, and OpenAPI. Write a separate detailed implementation plan for PLAN-006 only after an independent Reviewer approves its Design, PRD, dependencies, files, interfaces, red/green commands, and acceptance matrix.

Do not change PLAN-006 to ready in this governance commit. The state transition belongs to its independently reviewed readiness commit.

## Plan self-review

- Design-015 coverage: Tasks 1–5 cover structural validation, lifecycle, dependencies, evidence, commands, smoke, CI, canonical-path archive rules, and just-in-time PLAN-006 review.
- Placeholder scan: every code-changing step names files, commands, expected signals, and concrete behavior.
- Interface consistency: Tasks 1–2 define and reuse `Repository`, `Document`, `Issue`, `CommitResolver`, and `GitCommitResolver` consistently.
- Scope check: governance implementation is independently testable; PLAN-006 business implementation remains a separate plan.
