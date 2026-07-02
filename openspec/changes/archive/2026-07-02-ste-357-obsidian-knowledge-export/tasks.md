## 1. Vault 路径矩阵（pathing.go）

- [ ] 1.1 写 `BuildKnowledgePath` 失败测试（`pathing_test.go`）
- [ ] 1.2 实现 `PathInput` 结构体和 `BuildKnowledgePath` 路径矩阵
- [ ] 1.3 实现向后兼容：已有 `BuildPath` 不变
- [ ] 1.4 运行测试确认通过：`go test ./internal/obsidian -run TestBuildKnowledgePath -v`

## 2. 知识对象渲染器

- [ ] 2.1 写 Event 渲染失败测试（`render_event_test.go`）
- [ ] 2.2 实现 `EventNoteInput` 和 `RenderEventNote`
- [ ] 2.3 写 Digest 渲染失败测试（`render_digest_test.go`）
- [ ] 2.4 实现 `DigestNoteInput` 和 `RenderDigestNote`
- [ ] 2.5 写 Theme 渲染失败测试（`render_theme_test.go`）
- [ ] 2.6 实现 `ThemeNoteInput` 和 `RenderThemeNote`
- [ ] 2.7 运行全渲染测试确认通过：`go test ./internal/obsidian -run 'TestRender(Event|Digest|Topic|Theme)Note' -v`

## 3. 导出编排器（internal/export）

- [ ] 3.1 写 `BuildExportBundle` 失败测试（`bundle_test.go`）
- [ ] 3.2 实现 `ExportBundle`、`ExportKind`、`DateRange`、`BuildExportBundle`
- [ ] 3.3 写周期报告渲染失败测试（`report_renderer_test.go`）
- [ ] 3.4 实现 `RenderPeriodicReport`、`RenderThematicReport`、`RenderMaterialList`
- [ ] 3.5 运行全导出测试确认通过：`go test ./internal/export -v`

## 4. 导出 Job（internal/jobs）

- [ ] 4.1 实现 `publish_exports.go` job 骨架
- [ ] 4.2 实现 `publish_exports_test.go`

## 5. 端到端 Vault 快照回归

- [ ] 5.1 写集成失败测试（`tests/integration/knowledge_vault_test.go`）
- [ ] 5.2 实现 `GenerateKnowledgeSnapshot` 和 `ScenarioInput`
- [ ] 5.3 创建测试 fixtures（`tests/fixtures/knowledge_vault/README.md`）
- [ ] 5.4 运行集成测试确认通过：`go test ./tests/integration -run TestKnowledgeVaultSnapshot -v`

## 6. 文档更新

- [ ] 6.1 更新 `docs/obsidian/dataview-examples.md` 添加 exports/ FROM 路径
- [ ] 6.2 更新 `openspec/specs/daily-digest/spec.md` 目录与 frontmatter 定义
