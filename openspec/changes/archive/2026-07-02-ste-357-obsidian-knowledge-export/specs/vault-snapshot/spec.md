# vault-snapshot Specification

## Purpose

定义端到端 Vault 快照回归测试的行为规范，验证完整目录结构生成与文件写入。

## Requirements

### Requirement: 快照生成

系统 SHALL 提供 `GenerateKnowledgeSnapshot(root string, scenario ScenarioInput) error` 函数，在给定 root 下生成完整的 Vault 目录结构，包含所有知识类型。

```go
type ScenarioInput struct {
    Events  []EventNoteInput
    Topics  []TopicNoteInput
    Digests []DigestNoteInput
    Themes  []ThemeNoteInput
}

type VaultSnapshot struct {
    Root string
    Paths []string
}
```

#### Scenario: 快照生成所有目录
- **WHEN** `GenerateKnowledgeSnapshot(t.TempDir(), SampleScenario())`
- **THEN** 在 `root` 下 SHALL 创建以下目录之一（非空）：
  - `HotKey/events/`
  - `HotKey/topics/`
  - `HotKey/digests/daily/`
  - `HotKey/themes/`
  - `HotKey/exports/`

#### Scenario: 快照目录可枚举
- **WHEN** 调用 `GenerateKnowledgeSnapshot` 后列出目录结构
- **THEN** 每个知识类型的目录 SHALL 存在且包含预期数量的 `.md` 文件

### Requirement: 快照内容验证

每个生成的 `.md` 文件 SHALL 包含有效的 YAML frontmatter。

#### Scenario: 快照文件 frontmatter 完整
- **WHEN** 读取生成的 `.md` 文件
- **THEN** SHALL 以 `---\n` 开头，包含有效的 frontmatter 字段

### Requirement: 目录结构与 Dataview 查询一致

生成的目录结构 SHALL 与 `docs/obsidian/dataview-examples.md` 中的 Dataview FROM 查询路径一致。

#### Scenario: 目录与 Dataview 路径匹配
- **WHEN** 列出快照中的目录
- **THEN** 目录结构 SHALL 覆盖 Dataview 示例中的所有 FROM 路径
