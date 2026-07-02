# export-orchestrator Specification

## Purpose

定义统一导出编排器的行为规范，支持从知识对象（Event / Topic / Digest / Theme）编排每日、每周、每月周期报告以及专题报告和素材清单。

## Requirements

### Requirement: ExportBundle 结构

系统 SHALL 定义 `ExportBundle` 结构体作为编排中间对象：

```go
type ExportBundle struct {
    Kind       ExportKind
    DateRange  DateRange
    MonitorID  int64
    TopicIDs   []int64
    EventIDs   []int64
    ThemeIDs   []int64
    GeneratedAt time.Time
    Content    string
}

type ExportKind string

const (
    ExportDaily    ExportKind = "daily"
    ExportWeekly   ExportKind = "weekly"
    ExportMonthly  ExportKind = "monthly"
    ExportThematic ExportKind = "thematic"
    ExportMaterial ExportKind = "material"
)

type DateRange struct {
    Start string // YYYY-MM-DD 或 ISO 周
    End   string
}
```

#### Scenario: 构建周报 bundle
- **WHEN** 调用 `BuildExportBundle(BuildExportBundleInput{Kind: "weekly", MonitorID: 1, DateRange: DateRange{Start: "2026-06-24", End: "2026-06-30"}})`
- **THEN** 返回的 bundle 中 `Kind` 为 `"weekly"`，`DateRange.Start` 为 `"2026-06-24"`，`DateRange.End` 为 `"2026-06-30"`

### Requirement: 周期报告渲染

系统 SHALL 实现 `RenderPeriodicReport` 函数渲染周期报告：

```go
func RenderPeriodicReport(bundle ExportBundle, input PeriodicReportInput) string
```

`PeriodicReportInput` SHALL 包含报告所需的汇总数据：

```go
type PeriodicReportInput struct {
    Title       string
    PeriodLabel string // e.g. "2026年第27周"
    TopicCount  int
    EventCount  int
    Topics      []ReportTopicItem
    Summary     string
}

type ReportTopicItem struct {
    Rank       int
    Title      string
    HeatScore  float64
    Trend      string
}
```

#### Scenario: 周报 frontmatter
- **WHEN** 调用 `RenderPeriodicReport(bundle, PeriodicReportInput{Title: "AI监管周报", PeriodLabel: "2026年第27周"})`
- **THEN** 输出 SHALL 包含 frontmatter `type: hotkey-export`、`export_kind: weekly`、`title: AI监管周报`

#### Scenario: 周报包含周期汇总
- **WHEN** 周报包含 `TopicCount: 10`、`EventCount: 3`、`Summary: "本周热点..."`
- **THEN** 输出 SHALL 包含 `## 本周概览`、`热点主题数: 10`、`重要事件数: 3`、body 中 SHALL 包含 `本周热点...`

### Requirement: 专题报告渲染

系统 SHALL 实现 `RenderThematicReport` 函数渲染专题报告：

```go
func RenderThematicReport(bundle ExportBundle, input ThematicReportInput) string
```

#### Scenario: 专题报告 frontmatter
- **WHEN** 调用 `RenderThematicReport(bundle, ThematicReportInput{Title: "AI安全态势专题"})`
- **THEN** 输出 SHALL 包含 `type: hotkey-export`、`export_kind: thematic`、`title: AI安全态势专题`

### Requirement: 素材清单渲染

系统 SHALL 实现 `RenderMaterialList` 函数渲染素材清单：

```go
func RenderMaterialList(input MaterialListInput) string
```

#### Scenario: 素材清单包含来源
- **WHEN** `MaterialListInput{ThemeTitle: "AI监管", Items: []MaterialItem{{Fact: "新规发布", SourceURL: "https://x.com/1"}}}`
- **THEN** 输出 SHALL 包含 frontmatter `type: hotkey-export`、`export_kind: material`，body 中 SHALL 包含 `新规发布` 和来源 URL

### Requirement: 导出路径

所有导出类型使用 `BuildKnowledgePath` 生成写入路径：

| 导出类型 | Kind 参数 | 示例路径 |
|----------|-----------|---------|
| 每日 | `daily-export` | `{root}/HotKey/exports/daily/{slug}/{date}-{id}.md` |
| 每周 | `weekly-export` | `{root}/HotKey/exports/weekly/{slug}/{date}-{id}.md` |
| 每月 | `monthly-export` | `{root}/HotKey/exports/monthly/{slug}/{date}-{id}.md` |
| 专题 | `thematic-export` | `{root}/HotKey/exports/thematic/{date}-{id}-{title}.md` |
| 素材 | `material-export` | `{root}/HotKey/exports/material/{date}-{id}.md` |

#### Scenario: 导出使用 BuildKnowledgePath
- **WHEN** 渲染导出并确定写入路径
- **THEN** 调用 `BuildKnowledgePath` 且 Kind 参数与导出类型一致
