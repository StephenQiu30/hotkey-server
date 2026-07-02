## Context

023 已冻结 Event/Topic 双层对象模型。当前 `internal/obsidian` 仅支持 Topic 单类型渲染和 `BuildPath`（仅 `/HotKey/topics/`）。缺乏：

- 显式路径矩阵覆盖 events/、topics/、digests/daily/、themes/、exports/
- Event / DailyDigest / Theme 的独立渲染器
- 统一导出编排器（日报、周报、月报、专题报告、素材清单）
- 端到端 Vault 生成回归测试

`internal/export/` 包尚不存在，所有导出逻辑分布在 `internal/obsidian/`、`internal/digest/`、`internal/jobs/` 三个包中。

## Goals / Non-Goals

**Goals:**

1. 升级 `BuildPath` 为 `BuildKnowledgePath`，使用显式 switch-case 路径矩阵覆盖 5 个存放区域。
2. 为 Event / Topic / DailyDigest / Theme 提供独立渲染函数，各使用 `type: hotkey-*` frontmatter 标签。
3. 在 `internal/export` 实现导出 bundle 编排和报告渲染器，支持每日/每周/每月/专题/素材清单。
4. 在 `internal/jobs` 增加 `publish_exports` job 编排出口。
5. 增加端到端 Vault 快照回归集成测试。

**Non-Goals:**

- 不实现结构化回写（回写能力在 025 计划中）。
- 不修改 `topic_daily_exports` 等数据库 schema 边界。
- 不新增 Web/Miniapp 展示层。
- 不使用通用 `kind+s` 目录拼接（避免偷懒）。
- 不从零散 Topic 列表临时拼装日报（必须经过 digest 选择阶段）。
- 不让导出实现反过来定义对象模型。

## Decisions

### D1: 路径生成使用显式 switch-case 而非 map

**Choice**: `BuildKnowledgePath` 中使用 `switch in.Kind` 硬编码每个知识类型的路径模板。

**Rationale**: 虽然 map 更短，但 switch-case 在编译期就能发现未覆盖的类型分支（编译器警告 exhaustive switch），且每个分支可以独立调整格式而不影响其他类型。符合 "不用通用 kind+s 偷懒" 的非目标。

**Alternatives considered**: map[string]PathTemplate — 更 DRY 但运行时才能发现缺少类型，且类型间路径格式差异大（events 有 monitor slug、themes 无 monitor slug）。

### D2: 渲染器使用独立函数而非接口/通用 renderer

**Choice**: `RenderEventNote()`、`RenderTopicNote()`（已有）、`RenderDigestNote()`、`RenderThemeNote()` 各为一个公开函数。

**Rationale**: 各知识对象 frontmatter 字段完全不同：Event 有 event_id/event_key/date/topic_ids；Digest 有 digest_date/monitor/topic_count/event_count；Theme 有 theme_id/title/related_topics。通用接口会退化成参数膨胀的上帝函数或 reflection 杂烩。

**Alternatives considered**: `Renderer[T any]` 泛型接口 — Go 1.18+ 支持泛型，但此项目未启用泛型模式，且泛型仍然需要每类型实现 Render 方法，等价于独立函数。

### D3: ExportBundle 作为编排中间对象而非直接渲染

**Choice**: `BuildExportBundle()` 先收集齐全数据（Kind、DateRange、Monitors、Topics、Events、Themes），然后 `RenderPeriodicReport()` / `RenderMaterialList()` / `RenderThematicReport()` 消费。

**Rationale**: 分离"编排收集"和"渲染输出"两个阶段，让测试可以分别针对 bundle 构建和渲染输出进行。同时避免导出逻辑直接侵入 obsidian 渲染层。

**Alternatives considered**: 直接渲染传递各知识对象给渲染函数 — 耦合更强，测试需要构建完整对象链。

### D4: publish_exports job 作为 publish_daily_topics 的补充而不是替代

**Choice**: 新增 `publish_exports.go` job 独立处理夜间/周期批量导出，与已有的 `publish_daily_topics`（按 topic 粒度发布日报）共存。

**Rationale**: 每日 topic 渲染是增量逐条写入，而报告/素材清单是批量聚合渲染。两个方向的调度（按 topic 事件驱动 vs 按时间定时触发）和重试策略不同，合并会复杂化 job 逻辑。

### D5: 快照回归测试用 TempDir 而非固定路径

**Choice**: `GenerateKnowledgeSnapshot(root, scenario) error` 写入 `t.TempDir()` 然后在快照中验证目录和文件存在性。

**Rationale**: 不依赖 CI 环境存在的 OBSIDIAN_VAULT_PATH，测试可并行、可重复、不污染本地 vault。

## Risks / Trade-offs

- **Risk**: Event / DailyDigest / Theme 模型定义与已存在的数据库模型不一致。
  **Mitigation**: 这些输入结构体设计在 `internal/obsidian` 包内（如 `TopicNoteInput` 模式），不与 GORM 模型绑定。数据库查询层负责 map 到输入结构体。

- **Risk**: export bundle 与 publish_daily_topics 产生重复输出。
  **Mitigation**: export bundle 的 Daily 报告定位为跨 monitor 的综述报告，而非逐 topic 笔记的重新发布。路径分别位于 `exports/daily/` vs `topics/`。

- **Risk**: 路径变更破坏已有笔记的 Dataview 查询。
  **Mitigation**: 路径兼容性参见 specs 中的目录契约。本次不更改 `topics/` 结构（保持不变），新增区域不受影响。
