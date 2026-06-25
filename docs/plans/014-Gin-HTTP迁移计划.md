---
layer: Plan
doc_no: 014
audience: Dev, QA
feature_area: Gin HTTP迁移
purpose: 定义从旧 HTTP 主线迁移到 Gin 主线的执行顺序、边界和验证门禁
canonical_path: docs/plans/014-Gin-HTTP迁移计划.md
status: completed
version: v1.1
owner: Codex
inputs:
  - docs/design/005-平台整体重构设计.md
  - docs/design/006-Go后端工程与启动架构设计.md
outputs:
  - Gin 主线 router 与 middleware
  - 新 handler 组织方式
  - 新接口契约维护路径
triggers:
  - 应用骨架收敛完成后
downstream:
  - docs/plans/016-worker与遗留清理计划.md
---

> **已完成（2026-06-25）：** Gin 主线、`internal/platform/http`、静态 OpenAPI 与旧 Huma/server 删除均已完成。

# 背景

当前仓库 HTTP 层存在新旧并存和设计漂移问题。需要通过单独计划，将 Gin 主线迁移与接口契约策略一起收敛。

# 目标

1. 建立 Gin 为唯一 HTTP 主线。
2. 统一 middleware、错误响应、分组与 handler 组织方式。
3. 明确新的机器可读接口契约产物维护路径。
4. 切断旧 Huma / legacy server 的主运行身份。

# 非目标

1. 本计划不直接完成全部数据库 ORM 迁移。
2. 本计划不替代 worker 收敛任务。

# Task 1: 建立 Gin router 与全局中间件

目标：

1. 建立 `internal/platform/http` 新主结构。
2. 注册 health、auth、monitor 等基础路由组。
3. 统一 request id、recover、auth、logging 等中间件。

验证门禁：

```bash
go test ./internal/platform/http ./tests/unit/platform/http/... ./...
```

# Task 2: 迁移主要 API 域

迁移优先级建议：

1. health
2. auth
3. monitor
4. content
5. topic
6. trend
7. notify
8. admin/ops

每个域的要求：

1. handler 不承载业务规则。
2. DTO 与 service 分层。
3. 错误响应格式统一。

验证门禁：

```bash
go test ./...
bash scripts/smoke-api.sh
```

# Task 3: 重建接口契约产物

目标：

1. 确定新契约生成工具。
2. 保留单一事实源。
3. 明确 `docs/openapi.json` 是否延续为正式输出路径。

验证门禁：

```bash
test -f docs/openapi.json
bash scripts/validate-openapi.sh
```

# Task 4: 旧路由主线降级

目标：

1. 旧 Huma 路由不再承担主运行路径。
2. 旧 `internal/server` 仅在迁移期保留，最终进入删除清单。
3. 单元测试与集成测试切换到新主线。

验证门禁：

```bash
rg -n "internal/server|huma" internal tests scripts cmd
```

说明：

1. 历史设计与历史计划中保留对旧主线的引用不视为本阶段失败。
2. 本阶段验证只针对运行代码、测试和脚本入口。

# 风险与边界

1. 如果只迁移路由不迁移契约产物，会再次形成代码和文档双事实源。
2. 如果旧路由继续留在主运行路径上，HTTP 主线不会真正收敛。

# 变更记录

## v1.1

1. 标记计划状态为已完成。

## v1.0

1. 新建 Gin HTTP 迁移计划。
2. 将路由迁移、契约重建和旧主线降级打包为同一阶段任务。
