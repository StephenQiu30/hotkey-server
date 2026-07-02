## Why

023 已冻结 Event/Topic 双层对象模型；当前 `internal/obsidian` 仅支持 Topic 单类型渲染与 `BuildPath` 路径生成，缺少 Event / DailyDigest / Theme / ExportBundle 的独立渲染契约和统一导出编排能力。需要通过第二阶段实现把知识对象稳定写入 Obsidian Vault，并统一组织导出路径与渲染逻辑，使日报、周报、月报、专题报告和素材清单都从同一套知识对象生成。

## What Changes

### New Capabilities

1. **Vault 路径契约** — 从当前 `BuildPath`（仅 topics）升级为显式路径矩阵 `BuildKnowledgePath`，覆盖 events/、topics/、digests/daily/、themes/、exports/ 五个存放区域。
2. **知识对象渲染器** — 为 Event / Topic / DailyDigest / Theme 提供独立 Markdown 渲染函数与 frontmatter 类型标签。
3. **导出编排器** — 在 `internal/export` 实现 `BuildExportBundle` 编排、`RenderReport` / `RenderMaterialList` 等报告渲染，支持周期报告（daily/weekly/monthly）、专题报告和素材清单。
4. **导出 Job** — 在 `internal/jobs` 新增 `publish_exports` job，将 ExportBundle 渲染为 Markdown 并原子写入 Vault。
5. **Vault 快照回归** — 端到端集成测试验证完整目录结构生成。

### Modified Capabilities

- `daily-digest` spec 中目录定义将从 `/HotKey/topics/{slug}/...` 扩展到 `/HotKey/topics/`（保持不变），新增 `/HotKey/events/`、`/HotKey/digests/daily/`、`/HotKey/themes/`、`/HotKey/exports/`。

## Capabilities

### New Capabilities

- `vault-path-contract`: Vault 目录结构、存放区域和路径生成契约，使用显式 switch-case 矩阵而非通用 `kind+s` 拼接。
- `knowledge-renderer`: 知识对象 Markdown 渲染器，为 Event / Topic / DailyDigest / Theme 各提供独立渲染函数与 frontmatter 类型标签。
- `export-orchestrator`: 从统一知识对象编排日报、周报、月报、专题报告和素材清单的导出引擎。
- `vault-snapshot`: 端到端 Vault 快照回归，验证完整目录结构生成与文件写入。

### Modified Capabilities

- `daily-digest`: 更新 Obsidian 目录和 Frontmatter requirement，反映新增的 events/、digests/daily/、themes/、exports/ 存放区域以及新增的 frontmatter 类型标签。

## Impact

- `internal/obsidian/`: 新增 `pathing.go`（路径矩阵）、`render_event.go`、`render_digest.go`、`render_theme.go`（独立渲染器）。
- `internal/export/`: 全新包 — `bundle.go`（导出 bundle 编排）、`report_renderer.go`（报告渲染）。
- `internal/jobs/`: 新增 `publish_exports.go`（导出 job）。
- `tests/integration/`: 新增 `knowledge_vault_test.go`（快照回归）。
- `docs/obsidian/dataview-examples.md`: 更新 Dataview FROM 路径示例。
- `openspec/specs/daily-digest/spec.md`: 扩展目录与 frontmatter 定义。
