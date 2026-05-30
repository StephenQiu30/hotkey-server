---
layer: Plan
doc_no: "12"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:admin"
purpose: "实现管理员 API、任务观测、配置状态和审计日志。"
canonical_path: "docs/plans/12-管理员API与运维观测实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/product/prd/12-管理员API与运维观测PRD.md"
outputs:
  - "管理员API与运维观测实现任务"
triggers:
  - "管理员能力或观测需求变化"
downstream:
  - "由 WORKFLOW.md 指定的 Symphony / Linear 流程接管"
---

# 12-管理员API与运维观测实现计划

## 1. 目标

提供管理员 API 管理来源、频道、任务、日报重跑、外部依赖状态和审计日志。

## 2. 文件清单

- 创建：`migrations/000012_audit_logs.up.sql`
- 创建：`internal/domain/job/`
- 创建：`internal/service/admin/`
- 创建：`internal/repository/postgres/adminrepo/`
- 创建：`internal/transport/http/handlers/admin/`
- 修改：`internal/transport/http/router.go`

## 3. 任务拆解

1. 创建 `audit_logs`。
2. 实现管理员写操作审计。
3. 实现任务状态查询、失败任务重试、日报重跑。
4. 实现配置状态 API：PostgreSQL、Redis、DashScope、SMTP。
5. 测试普通用户访问管理员 API 返回 403。

## 4. TDD 与验证

- admin token 创建来源写 audit log。
- user token 访问 admin API 返回 403。
- Redis 不可用时状态返回 degraded。

## 5. 执行顺序

1. `test:` admin middleware、audit、status API 失败测试。
2. `impl:` migration、service、handler、status checker。
3. `docs:` 更新管理员 API contract。

## 6. 回滚策略

回滚 audit_logs migration，移除 admin routes。

## 7. 验收命令

```bash
go test ./...
python3 -m unittest discover -s tests
```

## 8. Symphony / Linear 要求

任务状态、标签和流转规则完全以本仓库 `WORKFLOW.md` 和本地 Symphony 实现为准。Plan 不定义额外状态、不发明额外标签、不覆盖 Symphony 的状态机。

Linear issue 只承载任务内容：PRD 路径、Plan 路径、任务范围、禁止范围、TDD 验收命令和回写要求。Symphony 负责监听 active states、创建 workspace、运行 Codex，并按 `WORKFLOW.md` prompt 驱动执行。

## 9. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-31 | StephenQiu30 | 1.0.0 | 初版 |
