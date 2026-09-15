---
layer: Acceptance
scope: backend
doc_no: "008"
title: FastAPI后端工程治理验收
status: passed
version: v1.0
owner: HotKey Team
canonical_path: docs/acceptance/008-FastAPI后端工程治理验收.md
design: docs/design/008-FastAPI后端工程治理设计.md
prd: docs/prd/008-FastAPI后端工程治理.md
plan: docs/plans/008-FastAPI后端工程治理计划.md
---

# 008 FastAPI后端工程治理验收

## 1. 验收上下文

- 日期：2026-09-16，Asia/Shanghai。
- 基线提交：`b98fd874b827bf68fbce489e57e5e2ce47a52bff`，包含已通过远程CI的事件合并/拆分。
- 验收只统计本轮工程治理与OpenAPI错误契约收敛，不重复声明事件业务验收。
- 集成环境：一次性无持久卷PostgreSQL 16与RabbitMQ 4.1-management，独立端口和`hotkey_test`数据库；完成后容器已删除。

## 2. 结果

| AC | 结果 | 证据 |
|---|---|---|
| AC-008-001 全源码架构门禁 | passed | 新门禁先准确报告`contents/services.py → monitors.models`与`events/services.py → contents.models`；解耦后architecture共32项通过；未来模块负向样例、跨域ORM样例、HTTP契约、唯一FastAPI构造和lifespan规则均通过 |
| AC-008-002 脱敏请求日志 | passed | 测试请求包含`?token=must-not-be-logged`；唯一完成日志含method、`/health/live`路由模板、200、request_id和duration，不含query值 |
| AC-008-003 静态、单元与契约 | passed | Ruff format/check通过；mypy strict为99个源码文件无问题；全量pytest计入AC-008-004；OpenAPI snapshot无漂移，健康/会话/列表样例错误集合符合路由声明；pip-audit无已知漏洞 |
| AC-008-004 真实数据库与消息 | passed | 隔离PostgreSQL/RabbitMQ均ready，`126 passed`；没有integration skip，覆盖当前0012唯一head、metadata差异、事务/并发、Outbox/RabbitMQ和事件用例 |
| AC-008-005 规范入口 | passed | 根AGENTS、backend README及docs/design/prd/plans/acceptance索引均链接008；文件职责和门禁已写入Plan |

## 3. 执行命令摘要

```sh
uv run --directory backend pytest tests/architecture/test_dependencies.py tests/unit/test_request_logging.py -q
uv run --directory backend pytest tests/architecture -q
HOTKEY_TEST_DATABASE_URL=... HOTKEY_TEST_BROKER_URL=... uv run --project backend pytest backend/tests -q
uv run --project backend ruff format --check backend
uv run --project backend ruff check backend
uv run --directory backend mypy
uv run --directory backend/src python -m tools.export_openapi --check
uv run --project backend pip-audit
npm run check:contract --prefix frontend
npm run check:boundaries --prefix frontend
npm run test:boundaries --prefix frontend
npm run format:check --prefix frontend
npm run build --prefix frontend
```

最终观察：后端`126 passed`，architecture单独`32 passed`，Ruff/MyPy/OpenAPI/pip-audit通过；前端契约、边界、格式、TypeScript和Vite生产构建通过。

## 4. 已知限制与后续项

- FastAPI/Starlette TestClient产生两条上游弃用警告：当前`httpx`路径未来需评估迁移到`httpx2`，本轮未把警告误报为失败，也未未经兼容验证升级测试栈。
- 现有UUID及`timestamp|UUID`cursor不是不透明token，公开API前需要版本方案；新接口不得继续复制。
- 本轮未运行根Compose、浏览器E2E、真实MinIO、真实平台来源、TLS入口、备份恢复、性能压测、metrics/trace或持续运行；这些不属于FastAPI代码规范通过的证据。
- `passed`只表示008定义的工程治理切片通过，不表示007产品、真实采集或生产上线完成。

## 5. 回滚验证

本轮没有数据库迁移、API路径或请求/成功DTO变更；生成契约只收敛错误响应声明。跨域解耦仍由真实集成用例覆盖；日志只增加服务端观测，不改变响应。若回滚治理代码，必须同时回滚对应架构/契约/日志测试和008状态，不能保留`passed`结论。
