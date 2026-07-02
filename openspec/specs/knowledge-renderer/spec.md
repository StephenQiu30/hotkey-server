# knowledge-renderer Specification

## Purpose

定义 Event / Topic / DailyDigest / Theme 四个知识对象的 Markdown 渲染契约，包括 frontmatter 字段、body 结构和类型标签。

## Requirements

### Requirement: 各对象 frontmatter 类型标签

每个知识对象的 Markdown 笔记 SHALL 包含 `type: hotkey-*` 标签以区分对象种类：

| 对象类型 | Frontmatter `type` 值 | 渲染函数 |
|----------|----------------------|----------|
| Event | `hotkey-event` | `RenderEventNote(EventNoteInput)` |
| Topic | `hotkey-topic` | `RenderTopicNote(TopicNoteInput)` |
| DailyDigest | `hotkey-digest` | `RenderDigestNote(DigestNoteInput)` |
| Theme | `hotkey-theme` | `RenderThemeNote(ThemeNoteInput)` |

#### Scenario: Event 类型标签
- **WHEN** 调用 `RenderEventNote(EventNoteInput{...})`
- **THEN** 输出 SHALL 包含 frontmatter 字段 `type: hotkey-event`

#### Scenario: Digest 类型标签
- **WHEN** 调用 `RenderDigestNote(DigestNoteInput{...})`
- **THEN** 输出 SHALL 包含 frontmatter 字段 `type: hotkey-digest`

#### Scenario: Theme 类型标签
- **WHEN** 调用 `RenderThemeNote(ThemeNoteInput{...})`
- **THEN** 输出 SHALL 包含 frontmatter 字段 `type: hotkey-theme`

### Requirement: EventNoteInput 与渲染

系统 SHALL 定义 `EventNoteInput` 结构体并实现 `RenderEventNote`：

```go
type EventNoteInput struct {
    EventID   int64
    EventKey  string
    Title     string
    Date      string
    Summary   string
    TopicIDs  []int64
}
```

渲染输出 SHALL 包含 frontmatter：`event_id`、`event_key`、`date`、`topic_ids`、`tags`。

#### Scenario: Event 笔记包含事件标识
- **WHEN** `RenderEventNote(EventNoteInput{EventID: 101, EventKey: "evt:ai", Title: "AI 事件", Date: "2026-07-01", Summary: "摘要", TopicIDs: []int64{42}})`
- **THEN** 输出 SHALL 包含 `event_id: 101`、`event_key: "evt:ai"`、`date: 2026-07-01`、`topic_ids: [42]`

#### Scenario: Event 笔记 body 包含标题和摘要
- **WHEN** `RenderEventNote(...{Title: "AI 事件", Summary: "重要事件摘要"})`
- **THEN** 输出 SHALL 包含 `# AI 事件` 和 `重要事件摘要`

### Requirement: DigestNoteInput 与渲染

系统 SHALL 定义 `DigestNoteInput` 结构体并实现 `RenderDigestNote`：

```go
type DigestNoteInput struct {
    DigestID    int64
    Date        string
    Monitor     string
    MonitorID   int64
    TopicCount  int
    EventCount  int
    Topics      []DigestTopicItem
    Events      []DigestEventItem
    Summary     string
}

type DigestTopicItem struct {
    TopicID   int64
    Title     string
    Summary   string
    HeatScore float64
}

type DigestEventItem struct {
    EventID  int64
    Title    string
    Summary  string
}
```

#### Scenario: Digest 笔记包含汇总信息
- **WHEN** `RenderDigestNote(...{Date: "2026-07-01", Monitor: "AI监管", TopicCount: 5, EventCount: 3})`
- **THEN** 输出 SHALL 包含 `date: 2026-07-01`、`monitor: AI监管`、`topic_count: 5`、`event_count: 3`

#### Scenario: Digest 笔记 body 包含主题列表
- **WHEN** Digest 包含 `Topics: []DigestTopicItem{{Title: "政策动态", Summary: "摘要内容"}}`
- **THEN** 输出 body 中 SHALL 包含 `## 热点主题` 标题和 `政策动态 - 摘要内容`

### Requirement: ThemeNoteInput 与渲染

系统 SHALL 定义 `ThemeNoteInput` 结构体并实现 `RenderThemeNote`：

```go
type ThemeNoteInput struct {
    ThemeID       int64
    Title         string
    Summary       string
    RelatedTopics []string
    EventCount    int
}
```

#### Scenario: Theme 笔记包含专题标识
- **WHEN** `RenderThemeNote(ThemeNoteInput{ThemeID: 42, Title: "AI安全监管", Summary: "..."})`
- **THEN** 输出 SHALL 包含 `type: hotkey-theme`、`theme_id: 42`、`title: AI安全监管`

### Requirement: Frontmatter 定界符

所有笔记 SHALL 使用 YAML frontmatter，以 `---\n` 开头和 `\n---\n` 结束。

#### Scenario: Frontmatter 定界符存在
- **WHEN** 任何对象渲染后
- **THEN** 输出 SHALL 以 `---\n` 开头，且包含 `\n---\n`

### Requirement: Tags 字段

Event 笔记 SHALL 包含 tags：`hotkey`、`event`。
Digest 笔记 SHALL 包含 tags：`hotkey`、`digest`、`daily`。
Theme 笔记 SHALL 包含 tags：`hotkey`、`theme`。

#### Scenario: Event tags
- **WHEN** `RenderEventNote()` 输出一个 AI 相关 Event
- **THEN** frontmatter 中 SHALL 包含 `tags:` 以及 `- hotkey`、`- event`

#### Scenario: Theme tags
- **WHEN** `RenderThemeNote()` 输出一个 Theme
- **THEN** frontmatter 中 SHALL 包含 `tags:` 以及 `- hotkey`、`- theme`
