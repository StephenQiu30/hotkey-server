---
layer: Plan
scope: backend
doc_no: "008"
title: FastAPI后端工程治理计划
status: completed
version: v1.0
owner: HotKey Team
canonical_path: docs/plans/008-FastAPI后端工程治理计划.md
design: docs/design/008-FastAPI后端工程治理设计.md
prd: docs/prd/008-FastAPI后端工程治理.md
---

# 008 FastAPI后端工程治理计划

## 1. 基线与差距

基线已有FastAPI工厂/lifespan、APIRouter、同步SQLAlchemy服务、Alembic、Celery、严格Pydantic、稳定错误、OpenAPI生成及架构/集成测试。审计确认的本轮缺口是：未来模块被提前白名单、service跨域导入ORM未被阻止、关闭access log后没有统一HTTP完成日志、006早期段落残留AsyncSession旧描述、工程入口未链接统一规范。

本轮同步收敛OpenAPI全局错误响应，不改变路径、请求/成功响应DTO或运行时错误格式。现有cursor透明、生产metrics/trace、公网入口和备份恢复仍需要版本或部署决策，登记为P1。

## 2. 文件与职责

| 文件 | 操作 | 职责 |
|---|---|---|
| `docs/{design,prd,plans,acceptance}/008-*` | 新增 | 规范、需求、执行与证据事实源 |
| `AGENTS.md`、docs/backend索引 | 修改 | 把008设为后端实现必读入口 |
| `backend/tests/architecture/test_dependencies.py` | 修改 | 删除预白名单，禁止service跨域ORM导入 |
| `backend/src/monitors/services.py` | 修改 | 暴露匹配内容ID的只读查询契约 |
| `backend/src/contents/services.py` | 修改 | 通过拥有者服务查询monitor匹配；提供内容引用DTO |
| `backend/src/events/services.py` | 修改 | 使用内容DTO，不直接导入Content ORM；不改变事件API/迁移 |
| `backend/src/api/middleware.py` | 修改 | 输出脱敏HTTP完成/失败日志 |
| `backend/tests/unit/test_request_logging.py` | 新增 | 验证日志字段和query不泄漏 |
| `backend/src/api/responses.py`、`api/routers/*.py`、`main.py` | 新增/修改 | 按操作声明错误响应，移除应用级全量错误集合 |
| `backend/tests/architecture/test_http_contract.py` | 新增 | 检查operation metadata、身份安全与样例错误集合 |
| `docs/openapi/openapi.json`、`frontend/src/api/` | 生成 | FastAPI导出与UmiOpenAPI生成结果 |
| `docs/design/006-*` | 修改 | 修正已被后文取代的AsyncSession描述 |

## 3. 执行记录

- [x] `CHK-008-G0-001` → `AC-008-001`：读取AGENTS、006/007架构、源码、测试、Compose和CI，保留当前事件档案在途改动。
- [x] `CHK-008-G1-001` → `AC-008-005`：从官方资料建立规范，并明确没有唯一“大厂目录模板”。
- [x] `CHK-008-G3-001` → `AC-008-001`：先增加跨域ORM与未来模块负向测试，观察到两处真实失败。
- [x] `CHK-008-G4-001` → `AC-008-001`：以查询契约和只读DTO消除耦合，门禁转绿。
- [x] `CHK-008-G3-002` → `AC-008-002`：先增加HTTP完成日志测试并观察失败。
- [x] `CHK-008-G4-002` → `AC-008-002`：实现路由模板日志并验证不记录query。
- [x] `CHK-008-G3-003` → `AC-008-003`：先用契约测试证明健康和列表端点被全局声明了不可达错误。
- [x] `CHK-008-G4-003` → `AC-008-003`：移除全局错误集合，在路由上通过受控辅助函数明确声明可达错误。
- [x] `CHK-008-G5-001` → `AC-008-003`：执行格式、lint、mypy、测试和OpenAPI检查，结果写入Acceptance。

## 4. 规格

`SPEC-008-API-001`：每个操作有operation_id、tag、成功状态和响应Schema；错误保持`{code, request_id}`并按操作声明可达状态。本轮不改变现有路径、请求/成功DTO或运行时状态码。

`SPEC-008-DATA-001`：service只能导入自身ORM模型。跨域读取由拥有模块提供函数或DTO；跨域原子写显式传入同一Session，参与方不得commit。

`SPEC-008-OBS-001`：完成日志字段固定为event、request_id、method、route模板、status_code、duration_ms；异常日志增加type和堆栈。禁止原始URL/query/body/凭据。

`SPEC-008-OPS-001`：本轮不新增运行组件。现有单进程API、独立migrate、Celery prefork和根Compose保持不变。

## 5. 回滚与完成定义

代码改动不含数据库迁移或运行时API行为变化；OpenAPI错误声明收敛为可达集合。若DTO解耦导致事件内容展示或锁语义回归，集成测试必须失败；不能通过放宽架构检查规避。完成要求文档入口可达、负向门禁有效、静态/单元/架构/OpenAPI通过，并诚实记录真实服务与外部能力边界。
