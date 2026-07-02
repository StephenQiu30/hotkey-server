---
layer: Plan
doc_no: 024
audience: Dev, QA
feature_area: Obsidian 知识沉淀与统一导出
purpose: 定义 Vault 目录、知识对象 Markdown 契约、统一导出编排与端到端回归任务
canonical_path: docs/plans/024-Obsidian知识沉淀与导出计划.md
status: draft
version: v1.0
owner: Codex
inputs:
  - docs/prd/001-obsidian热点知识中台需求.md
  - docs/plans/022-Obsidian热点知识中台总体实施计划.md
  - docs/plans/023-事件主题知识模型与同步基线计划.md
  - docs/obsidian/dataview-examples.md
outputs:
  - Vault 目录结构
  - Daily/Weekly/Monthly/Theme/Material 导出能力
  - Obsidian 渲染与导出回归基线
triggers:
  - 已冻结 Event/Topic 双层对象
  - 需要把知识对象稳定写入 Vault 并支持统一导出
downstream:
  - docs/plans/025-双向回写与治理计划.md
---

# Obsidian Knowledge Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立稳定的 Obsidian Vault 目录、知识对象 Markdown 契约和统一导出能力，使日报、专题和素材清单都能从同一套知识对象生成。

**Architecture:** 由 `internal/obsidian` 负责 Markdown 与路径契约，由新的 `internal/export` 负责导出编排，由 worker job 负责生成与刷新导出结果。所有导出都依赖 `Event/Topic/Theme/ExportBundle`，不再直接从零散 topic 列表临时拼接。

**Tech Stack:** Go, Markdown, YAML frontmatter, Obsidian Dataview, worker jobs

---

### Task 1: 冻结 Vault 目录结构与路径生成器

**Files:**
- Modify: `internal/obsidian/writer.go`
- Create: `internal/obsidian/pathing.go`
- Create: `internal/obsidian/pathing_test.go`
- Modify: `docs/obsidian/dataview-examples.md`

- [ ] **Step 1: 写路径生成器失败测试**

```go
func TestBuildKnowledgePaths(t *testing.T) {
    got := BuildKnowledgePath("/vault", PathInput{
        Kind:       "event",
        MonitorSlug: "ai-regulation",
        Date:       "2026-07-01",
        StableID:   "evt-101",
        TitleSlug:  "ai-监管规则发布",
    })
    want := "/vault/HotKey/events/ai-regulation/2026-07-01-evt-101-ai-监管规则发布.md"
    if got != want {
        t.Fatalf("got %q want %q", got, want)
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/obsidian -run TestBuildKnowledgePaths -v`
Expected: FAIL with `undefined: BuildKnowledgePath`

- [ ] **Step 3: 实现路径生成器**

```go
func BuildKnowledgePath(root string, in PathInput) string {
    base := filepath.Join(root, "HotKey")
    switch in.Kind {
    case "event":
        return filepath.Join(base, "events", in.MonitorSlug, fmt.Sprintf("%s-%s-%s.md", in.Date, in.StableID, in.TitleSlug))
    case "topic":
        return filepath.Join(base, "topics", in.MonitorSlug, fmt.Sprintf("%s-%s-%s.md", in.Date, in.StableID, in.TitleSlug))
    case "daily-digest":
        return filepath.Join(base, "digests", "daily", in.MonitorSlug, fmt.Sprintf("%s-%s.md", in.Date, in.StableID))
    case "theme":
        return filepath.Join(base, "themes", fmt.Sprintf("%s-%s.md", in.StableID, in.TitleSlug))
    case "weekly-export":
        return filepath.Join(base, "exports", "weekly", in.MonitorSlug, fmt.Sprintf("%s-%s.md", in.Date, in.StableID))
    default:
        return filepath.Join(base, "misc", fmt.Sprintf("%s-%s.md", in.Date, in.StableID))
    }
}
```

- [ ] **Step 4: 更新 Dataview 文档示例**

```markdown
FROM "HotKey/events"
FROM "HotKey/topics"
FROM "HotKey/digests/daily"
FROM "HotKey/themes"
FROM "HotKey/exports"
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/obsidian -run TestBuildKnowledgePaths -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/obsidian/pathing.go internal/obsidian/pathing_test.go internal/obsidian/writer.go docs/obsidian/dataview-examples.md
git commit -m "feat: 冻结知识中台 vault 目录结构"
```

### Task 2: 为 Event / Topic / DailyDigest / Theme 增加独立渲染器

**Files:**
- Create: `internal/obsidian/render_event.go`
- Create: `internal/obsidian/render_event_test.go`
- Create: `internal/obsidian/render_topic.go`
- Create: `internal/obsidian/render_topic_test.go`
- Create: `internal/obsidian/render_digest.go`
- Create: `internal/obsidian/render_digest_test.go`
- Create: `internal/obsidian/render_theme.go`
- Create: `internal/obsidian/render_theme_test.go`

- [ ] **Step 1: 写 Event 渲染失败测试**

```go
func TestRenderEventNote(t *testing.T) {
    got := RenderEventNote(EventNoteInput{
        EventID:   101,
        EventKey:  "evt:ai-regulation:2026-07-01",
        Title:     "AI 监管规则发布",
        Date:      "2026-07-01",
        Summary:   "监管机构发布新规。",
        TopicIDs:  []int64{42},
    })
    if !strings.Contains(got, "type: hotkey-event") {
        t.Fatal("missing event type")
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/obsidian -run TestRenderEventNote -v`
Expected: FAIL with `undefined: RenderEventNote`

- [ ] **Step 3: 实现 Event / Topic / Digest / Theme 独立渲染器**

```go
func RenderEventNote(in EventNoteInput) string {
    return fmt.Sprintf(`---
type: hotkey-event
event_id: %d
event_key: %q
date: %s
topic_ids: %v
---

# %s

%s
`, in.EventID, in.EventKey, in.Date, in.TopicIDs, in.Title, in.Summary)
}
```

- [ ] **Step 4: 为 Topic / Digest / Theme 写对应 frontmatter 断言**

```go
func TestRenderDigestNote(t *testing.T) {
    got := RenderDigestNote(DigestNoteInput{Date: "2026-07-01", Monitor: "AI监管"})
    if !strings.Contains(got, "type: hotkey-digest") {
        t.Fatal("missing digest type")
    }
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/obsidian -run 'TestRender(Event|Digest|Topic|Theme)Note' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/obsidian/render_event.go internal/obsidian/render_event_test.go internal/obsidian/render_topic.go internal/obsidian/render_topic_test.go internal/obsidian/render_digest.go internal/obsidian/render_digest_test.go internal/obsidian/render_theme.go internal/obsidian/render_theme_test.go
git commit -m "feat: 拆分事件主题日报专题渲染器"
```

### Task 3: 建立统一导出编排器，支持周期报告、专题报告和素材清单

**Files:**
- Create: `internal/export/bundle.go`
- Create: `internal/export/bundle_test.go`
- Create: `internal/export/report_renderer.go`
- Create: `internal/export/report_renderer_test.go`
- Create: `internal/jobs/publish_exports.go`
- Create: `internal/jobs/publish_exports_test.go`

- [ ] **Step 1: 写导出编排失败测试**

```go
func TestBuildExportBundle_WeeklyDigest(t *testing.T) {
    bundle := BuildExportBundle(BuildExportBundleInput{
        Kind:      "weekly",
        MonitorID: 1,
        DateRange: DateRange{Start: "2026-06-24", End: "2026-06-30"},
    })
    if bundle.Kind != "weekly" {
        t.Fatalf("got %q", bundle.Kind)
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/export -run TestBuildExportBundle_WeeklyDigest -v`
Expected: FAIL with `undefined: BuildExportBundle`

- [ ] **Step 3: 实现统一导出 bundle**

```go
type ExportBundle struct {
    Kind      string
    MonitorID int64
    DateRange DateRange
    TopicIDs  []int64
    EventIDs  []int64
    ThemeIDs  []int64
}
```

- [ ] **Step 4: 为三类导出各写一条渲染测试**

```go
func TestRenderMaterialList(t *testing.T) {
    got := RenderMaterialList(MaterialListInput{
        ThemeTitle: "AI监管",
        Items: []MaterialItem{{Fact: "新规发布", SourceURL: "https://x.com/1"}},
    })
    if !strings.Contains(got, "SourceURL") {
        t.Fatal("expected material source")
    }
}
```

- [ ] **Step 5: 新增 publish_exports job 并运行测试**

Run: `go test ./internal/export ./internal/jobs -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/export internal/jobs/publish_exports.go internal/jobs/publish_exports_test.go
git commit -m "feat: 新增统一导出编排能力"
```

### Task 4: 增加端到端 Vault 快照回归

**Files:**
- Create: `tests/integration/knowledge_vault_test.go`
- Create: `tests/fixtures/knowledge_vault/README.md`
- Modify: `scripts/validate-repository.sh`

- [ ] **Step 1: 写集成失败测试**

```go
func TestKnowledgeVaultSnapshot(t *testing.T) {
    root := t.TempDir()
    err := GenerateKnowledgeSnapshot(root, SampleScenario())
    if err != nil {
        t.Fatalf("generate snapshot: %v", err)
    }
    assertFileExists(t, filepath.Join(root, "HotKey", "events"))
    assertFileExists(t, filepath.Join(root, "HotKey", "topics"))
    assertFileExists(t, filepath.Join(root, "HotKey", "digests", "daily"))
    assertFileExists(t, filepath.Join(root, "HotKey", "exports"))
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestKnowledgeVaultSnapshot -v`
Expected: FAIL with `undefined: GenerateKnowledgeSnapshot`

- [ ] **Step 3: 在 `scripts/validate-repository.sh` 的架构校验段后追加知识中台快照回归**

```bash
sed -n '128,150p' scripts/validate-repository.sh
```

Expected: 显示 `Architecture boundary validation` 之后新增 `go test ./tests/integration -run TestKnowledgeVaultSnapshot -v`

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./tests/integration -run TestKnowledgeVaultSnapshot -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add tests/integration/knowledge_vault_test.go tests/fixtures/knowledge_vault/README.md scripts/validate-repository.sh
git commit -m "test: 增加知识 vault 端到端快照回归"
```
