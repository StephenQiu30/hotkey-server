---
layer: PRD
scope: backend
doc_no: "008"
title: FastAPI后端工程治理
status: implemented
version: v1.0
owner: HotKey Team
canonical_path: docs/prd/008-FastAPI后端工程治理.md
design: docs/design/008-FastAPI后端工程治理设计.md
plan: docs/plans/008-FastAPI后端工程治理计划.md
---

# 008 FastAPI后端工程治理需求

## 1. 问题与目标

开发者需要在增加来源、内容、事件、分析和知识模块时，快速判断文件归属、依赖方向、事务边界、HTTP契约和验证范围。目标不是增加架构层数，而是让错误结构在合入前自动失败，让运行问题可以通过request_id、稳定错误和分层健康信号定位。

## 2. 范围

P0包括后端规范事实源、当前代码审计、跨域ORM门禁、未来模块登记门禁、按操作收敛错误响应契约、最小HTTP完成日志、规范文档入口和现有验证。P1包括公开API不透明cursor、metrics/trace接收端、公网限流/TLS/备份演练。微服务拆分、异步ORM切换和通用框架不在本轮范围。

## 3. 需求

| ID | 需求 |
|---|---|
| BR-008-001 | 后端变更必须有一个项目内可引用、可执行、可审查的FastAPI工程规范 |
| FR-008-001 | 规范必须覆盖目录、依赖、生命周期、并发、事务、API、迁移、任务、安全、可观测和测试 |
| FR-008-002 | 架构测试扫描全部源码，未知顶层模块、跨域ORM导入和非法路由依赖必须失败 |
| FR-008-003 | 每个HTTP响应带request_id，服务端完成日志可按request_id定位，且不记录query/body/凭据 |
| FR-008-004 | Design、PRD、Plan、Acceptance和AGENTS互相链接，新增实现可从仓库入口找到规范 |
| FR-008-005 | OpenAPI错误响应按操作声明，健康端点和业务端点不共享一组虚报错误 |
| NFR-008-001 | 数据库路由不得在事件循环运行阻塞I/O；Session不得跨线程、请求或任务共享 |
| NFR-008-002 | 一个业务用例的状态、审计和投递意图按设计在同一事务提交 |
| NFR-008-003 | OpenAPI为唯一HTTP契约，输入输出均由Pydantic约束，前端不引用ORM结构 |
| NFR-008-004 | 迁移只追加，唯一head与readiness一致，真实PostgreSQL可重放并与metadata对账 |
| NFR-008-005 | Celery任务在至少一次交付下幂等，数据库事实不依赖Celery result backend |
| NFR-008-006 | 验证结果必须区分静态/单元、真实数据库/消息、容器、浏览器和外部来源 |

## 4. 验收标准

- `AC-008-001`：Given全量`backend/src/**/*.py`，When运行architecture测试，Then当前源码通过，构造未知模块或跨域ORM导入样例时门禁失败。
- `AC-008-002`：Given带敏感query的HTTP请求，When请求完成，Then日志含request_id、方法、路由模板、状态和耗时，不含query值。
- `AC-008-003`：Given当前应用，When运行Ruff、mypy、unit/architecture和OpenAPI检查，Then全部通过，没有生成契约漂移，且样例操作只声明实际可返回的错误。
- `AC-008-004`：Given可丢弃PostgreSQL/RabbitMQ，When运行集成测试，Then迁移、事务、并发、Outbox与消息契约通过；环境不存在时必须记录skip而不是passed。
- `AC-008-005`：Given任一新后端切片，When开发者读取AGENTS/README，Then可以定位008规范、文件职责和必须运行的门禁。

## 5. 追踪

| 需求 | 决策 | 验收 |
|---|---|---|
| BR/FR-008-001、004 | DEC-008-001 项目规范优先于外部目录模板 | AC-008-005 |
| FR-008-002 | DEC-008-002 模块所有权+AST门禁 | AC-008-001、003 |
| FR-008-003 | DEC-008-003 路由模板最小日志 | AC-008-002 |
| FR-008-005 | DEC-008-007 按操作错误契约 | AC-008-003 |
| NFR-008-001、002 | DEC-008-004 同步路由、独立Session、service提交 | AC-008-003、004 |
| NFR-008-003、004、005 | DEC-008-005 OpenAPI/Alembic/Job账本事实源 | AC-008-003、004 |
| NFR-008-006 | DEC-008-006 分级证据 | AC-008-003、004 |
