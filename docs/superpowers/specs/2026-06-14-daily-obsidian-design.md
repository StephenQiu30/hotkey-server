# 热点日报 → Obsidian 知识库 设计规格

> 头脑风暴确认稿。Canonical 设计见 [`docs/design/004-热点日报Obsidian知识库设计.md`](../../design/004-热点日报Obsidian知识库设计.md)。

## 需求确认记录

| 维度 | 决策 |
|------|------|
| 主流程 | 服务端定时生成 Markdown，写入 Obsidian 同步目录 |
| 内容范围 | 每个 `keyword_monitor` 独立产出 |
| 摘要方式 | 服务端 LLM 生成主题摘要 |
| Vault 结构 | 扁平主题笔记 + YAML frontmatter，Dataview 聚合 |
| 时间窗口 | 北京时间自然日 00:00–23:59 |
| 部署 | iCloud / 坚果云 / Obsidian Sync 同步文件夹 |
| MVP 边界 | 仅 hotkey-server |

## 验收标准

1. 每日 08:00 CST（可配置）自动为每个 active monitor 生成昨日热点 Markdown。
2. 每篇笔记含完整 frontmatter，Obsidian Dataview 可按 `date`、`monitor` 查询。
3. LLM 摘要写入笔记正文，并回写 `topics.summary` 与 `topic_daily_exports.summary_text`。
4. 重复执行同日期同 topic 不重复创建文件，而是覆盖更新。
5. LLM 或写盘失败时 `topic_daily_exports.status=failed`，其他 topic 不受影响。
6. `make test` 全绿，含 digest 时间边界与渲染测试。

## 方案选择

采用 **DB 记录 + 文件发布**（方案 B）：

- `topic_daily_exports` 存摘要与发布状态
- Job 先落库再写 Vault
- 兼顾幂等、重试与未来 Web/API 复用

## Linear 追踪

| 层级 | Issue |
|------|-------|
| Epic | [STE-301](https://linear.app/stephenqiu/issue/STE-301/epic-热点日报-obsidian-知识库-mvp) |
| OpenSpec + 文档 | [STE-302](https://linear.app/stephenqiu/issue/STE-302/日报-openspec-设计文档门禁) |
| 数据模型 + 配置 | [STE-303](https://linear.app/stephenqiu/issue/STE-303/日报-topic-daily-exports-与配置扩展) |
| digest 模块 | [STE-304](https://linear.app/stephenqiu/issue/STE-304/日报-digest-自然日窗口与主题筛选) |
| obsidian 模块 | [STE-305](https://linear.app/stephenqiu/issue/STE-305/日报-obsidian-markdown-渲染与写盘) |
| LLM 模块 | [STE-306](https://linear.app/stephenqiu/issue/STE-306/日报-llm-摘要模块) |
| 发布 Job | [STE-307](https://linear.app/stephenqiu/issue/STE-307/日报-publish-daily-topics-job-调度) |
| 测试与验收 | [STE-308](https://linear.app/stephenqiu/issue/STE-308/日报-端到端测试与验收) |

## 实施计划

详见 [`docs/plans/011-热点日报Obsidian知识库计划.md`](../../plans/011-热点日报Obsidian知识库计划.md)。
