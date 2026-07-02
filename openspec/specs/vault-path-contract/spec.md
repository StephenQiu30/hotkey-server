# vault-path-contract Specification

## Purpose

定义 Obsidian Vault 中知识对象的存放目录结构和路径生成契约，使用显式 switch-case 路径矩阵而非通用 `kind+s` 拼接，确保路径可预测、可枚举、可审核。

## Requirements

### Requirement: 存放区域种类

系统 SHALL 定义以下五个存放区域：

| 知识类型 | 存放路径 | 文件命名格式 |
|----------|----------|-------------|
| Event | `{root}/HotKey/events/{monitorSlug}/` | `{date}-{stableID}-{titleSlug}.md` |
| Topic | `{root}/HotKey/topics/{monitorSlug}/` | `{date}-topic-{id}-{titleSlug}.md` |
| DailyDigest | `{root}/HotKey/digests/daily/{monitorSlug}/` | `{date}-{stableID}.md` |
| Theme | `{root}/HotKey/themes/` | `{stableID}-{titleSlug}.md` |
| Export | `{root}/HotKey/exports/{exportKind}/{monitorSlug}/` | `{date}-{stableID}.md` |

其中 Export 支持以下子类型：`daily/`、`weekly/`、`monthly/`、`thematic/`、`material/`。

#### Scenario: Event 路径
- **WHEN** 调用 `BuildKnowledgePath("/vault", PathInput{Kind: "event", MonitorSlug: "ai-regulation", Date: "2026-07-01", StableID: "evt-101", TitleSlug: "ai-guize"})`
- **THEN** 返回 `/vault/HotKey/events/ai-regulation/2026-07-01-evt-101-ai-guize.md`

#### Scenario: Topic 路径
- **WHEN** 调用 `BuildKnowledgePath("/vault", PathInput{Kind: "topic", MonitorSlug: "ai-regulation", Date: "2026-07-01", StableID: "42", TitleSlug: "ai-zhengce"})`
- **THEN** 返回 `/vault/HotKey/topics/ai-regulation/2026-07-01-topic-42-ai-zhengce.md`

#### Scenario: DailyDigest 路径
- **WHEN** 调用 `BuildKnowledgePath("/vault", PathInput{Kind: "daily-digest", MonitorSlug: "ai-regulation", Date: "2026-07-01", StableID: "ddigest-101"})`
- **THEN** 返回 `/vault/HotKey/digests/daily/ai-regulation/2026-07-01-ddigest-101.md`

#### Scenario: Theme 路径
- **WHEN** 调用 `BuildKnowledgePath("/vault", PathInput{Kind: "theme", StableID: "thm-42", TitleSlug: "ai-security"})`
- **THEN** 返回 `/vault/HotKey/themes/thm-42-ai-security.md`

#### Scenario: 每周导出路径
- **WHEN** 调用 `BuildKnowledgePath("/vault", PathInput{Kind: "weekly-export", MonitorSlug: "ai-regulation", Date: "2026-W27", StableID: "wexp-101"})`
- **THEN** 返回 `/vault/HotKey/exports/weekly/ai-regulation/2026-W27-wexp-101.md`

#### Scenario: 专题导出路径
- **WHEN** 调用 `BuildKnowledgePath("/vault", PathInput{Kind: "thematic-export", Date: "2026-07-01", StableID: "texp-101", TitleSlug: "ai-overview"})`
- **THEN** 返回 `/vault/HotKey/exports/thematic/2026-07-01-texp-101-ai-overview.md`

#### Scenario: 素材清单导出路径
- **WHEN** 调用 `BuildKnowledgePath("/vault", PathInput{Kind: "material-export", Date: "2026-07-01", StableID: "mexp-101"})`
- **THEN** 返回 `/vault/HotKey/exports/material/2026-07-01-mexp-101.md`

#### Scenario: 未知 kind 降级到 misc
- **WHEN** 调用 `BuildKnowledgePath("/vault", PathInput{Kind: "unknown", Date: "2026-07-01", StableID: "unk-1"})`
- **THEN** 返回 `/vault/HotKey/misc/2026-07-01-unk-1.md`

### Requirement: PathInput 结构体

系统 SHALL 定义 `PathInput` 结构体包含路径生成所需的全部参数：

```go
type PathInput struct {
    Kind        string // 知识类型: event / topic / daily-digest / theme / weekly-export / monthly-export / thematic-export / material-export
    MonitorSlug string // 监控 slug（可选，部分类型使用）
    Date        string // 日期或周期标识（可选，部分类型使用）
    StableID    string // 稳定 ID
    TitleSlug   string // 标题 slug（可选，部分类型使用）
}
```

#### Scenario: 必填参数校验
- **WHEN** `PathInput` 中 `Kind` 为空
- **THEN** 路径生成 SHALL 返回降级路径 `/vault/HotKey/misc/unfiled.md`

#### Scenario: 参数组合校验
- **WHEN** `PathInput{Kind: "theme", StableID: "", TitleSlug: ""}`
- **THEN** 路径生成 SHALL 返回降级路径 `/vault/HotKey/misc/unfiled.md`

### Requirement: Dataview 查询兼容性

所有存放区域 SHALL 可通过 Obsidian Dataview 查询：

| 查询 | 匹配路径 |
|------|---------|
| `FROM "HotKey/events"` | 所有 Event 笔记 |
| `FROM "HotKey/topics"` | 所有 Topic 笔记 |
| `FROM "HotKey/digests/daily"` | 所有日报笔记 |
| `FROM "HotKey/themes"` | 所有专题笔记 |
| `FROM "HotKey/exports"` | 所有导出报告 |

#### Scenario: Dataview FROM 路径
- **WHEN** Dataview 查询 `FROM "HotKey/events"`
- **THEN** SHALL 返回所有前缀为 `{root}/HotKey/events/` 的笔记

### Requirement: 向后兼容

已有 `BuildPath` 函数 SHALL 保持可用，新代码应优先使用 `BuildKnowledgePath`。
已有 Topic 笔记的路径格式 `{root}/HotKey/topics/{slug}/{date}-topic-{id}-{slug}.md` SHALL 保持不变。

#### Scenario: 已有笔记路径不变化
- **WHEN** 调用 `BuildPath("/vault", "ai", "2026-06-14", "42", "ai-ce")`
- **THEN** 返回 `/vault/HotKey/topics/ai/2026-06-14-topic-42-ai-ce.md`（与原有行为一致）
