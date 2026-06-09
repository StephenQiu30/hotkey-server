---
layer: Plan
doc_no: "14"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:platform-runtime"
purpose: "建立统一的 runtime 装配入口，让服务模式切换和依赖注入行为可测试、可复用、可追踪。"
canonical_path: "docs/plans/14-runtime服务装配实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/design/002-热点平台能力补齐与上线接通设计.md"
  - "docs/plans/13-配置与环境启动实现计划.md"
  - "docs/product/prd/01-项目治理与Symphony编排PRD.md"
outputs:
  - "runtime 服务装配实现任务"
triggers:
  - "服务模式或依赖注入边界变化"
downstream:
  - "docs/plans/15-Redis任务运行时接线实现计划.md"
---

# 14-runtime服务装配实现计划

## 1. 目标

建立统一的 runtime 装配入口，让 `api`、`worker`、`all` 等服务模式切换和依赖注入行为可测试、可复用、可追踪。

## 2. 文件清单

- 修改：`cmd/server/main.go`（或 `cmd/hotkey-api`）
- 修改：`internal/app/server.go`
- 修改：`internal/worker/worker.go`
- 修改：`internal/scheduler/scheduler.go`
- 创建或扩展：`internal/app/bootstrap_test.go`

## 3. 任务拆解

1. 梳理当前依赖装配路径，并为失败模式写测试。
2. 统一 runtime 入口和依赖构造逻辑。
3. 接入配置层输出的依赖参数。
4. 补齐 `api`、`worker`、`all` 模式初始化测试。
5. 运行验证并回写结果。

## 4. TDD 与验证

- runtime 装配边界清晰且可测试。
- 服务模式切换不需要重复编写初始化逻辑。
- 依赖缺失或配置错误时初始化应明确失败。

## 5. 执行顺序

1. `test:` runtime 初始化与模式切换失败测试。
2. `impl:` 统一 bootstrap 与依赖注入。
3. `refactor:` 清理重复的初始化代码。

## 6. 回滚策略

回滚 bootstrap 统一入口，恢复各模式独立初始化路径；不影响 config 层变更。

## 7. 验收命令

```bash
go test ./internal/app/...
gofmt -w cmd internal
go test ./...
python3 -m unittest discover -s tests
```

## 8. Symphony / Linear 要求

任务状态、标签和流转规则完全以本仓库 `WORKFLOW.md` 和本地 Symphony 实现为准。Plan 不定义额外状态、不发明额外标签、不覆盖 Symphony 的状态机。

Linear issue 只承载任务内容：PRD 路径、Plan 路径、任务范围、禁止范围、TDD 验收命令和回写要求。Symphony 负责监听 active states、创建 workspace、运行 Codex，并按 `WORKFLOW.md` prompt 驱动执行。

## 9. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-06-09 | StephenQiu30 | 1.0.0 | 按文档规范重写，编号调整为 14 |
