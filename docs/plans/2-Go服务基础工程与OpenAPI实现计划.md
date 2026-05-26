---
layer: Plan
doc_no: "2"
audience:
  - Tech-Lead
  - Dev
  - QA
  - Ops
feature_area: "area:infra area:api"
purpose: "将 `Go服务基础工程与OpenAPI` PRD 拆成可执行任务、验证命令和回滚边界。"
canonical_path: "docs/plans/2-Go服务基础工程与OpenAPI实现计划.md"
status: approved
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/2-Go服务基础工程与OpenAPIPRD.md
  - docs/engineering/1-Go后端重建与开源仓库治理设计.md
outputs:
  - Go服务基础工程与OpenAPI实现任务
  - Go服务基础工程与OpenAPI验证证据
triggers:
  - "docs/product/prd/2-Go服务基础工程与OpenAPIPRD.md 变更"
  - "对应 GitHub 或 Linear issue 状态变更"
downstream:
  - docs/acceptance/README.md
---

# 2-Go服务基础工程与OpenAPI 实现计划

## 1. 目标

建立 Go 模块化单体基础工程、PostgreSQL、pgvector、Redis 配置和 OpenAPI 导出。

## 2. 文件清单

- PRD：`docs/product/prd/2-Go服务基础工程与OpenAPIPRD.md`
- Plan：`docs/plans/2-Go服务基础工程与OpenAPI实现计划.md`
- 设计输入：`docs/engineering/1-Go后端重建与开源仓库治理设计.md`
- 验收输出：后续按任务结果写入 `docs/acceptance/`

## 3. 任务拆解

1. 阅读 PRD 和工程设计，确认本任务属于 **P0 开源核心闭环**。
2. 写失败测试或文档结构检查，先锁定验收边界。
3. 实现最小可用改动，不夹带其他编号任务。
4. 导出或更新 OpenAPI，并记录端侧影响。
5. 执行验证命令，保存关键输出。
6. 更新 GitHub issue 和 Linear issue 状态。

## 4. TDD 与验证

- 文档任务：运行 CI 中的文档编号与旧运行时缺失检查，或补充新的仓库治理检查。
- Go 实现任务：运行 `go test ./...`。
- OpenAPI 任务：运行项目定义的 OpenAPI 导出命令，并检查生成文件。
- 数据库任务：运行 schema 初始化和最小读写测试。

## 5. 执行顺序

- 先完成 PRD/Plan 文件。
- 再实现后端 schema、服务和 API。
- 再更新 OpenAPI 与端侧契约说明。
- 最后补验收证据和 issue 状态。

## 6. 回滚策略

- 文档变更可通过 Git 单独 revert。
- schema 变更必须提供向后兼容或重建空库说明。
- 外部来源、AI、Redis、pgvector 相关能力必须可禁用或降级。

## 7. 验收标准

- 服务可启动，健康检查可用，OpenAPI 可导出，基础测试通过。
- 本任务不引用旧 FastAPI 编号文档作为事实源。
- GitHub 与 Linear issue 均指定负责人。
- 工作区不保留一次性中间产物。

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-26 | StephenQiu30 | 1.0.0 | 初版，按 Go 后端全面重构新编号体系创建 |
