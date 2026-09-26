---
layer: Plan
scope: issue
doc_no: "056"
title: HTTP契约客户端生成与CI门禁执行计划
status: planned
version: v1.0
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/056-HTTP契约客户端生成与CI门禁执行计划.md
prd: docs/prd/001-热点舆情监控平台需求.md
design: docs/design/001-热点舆情监控平台总体设计.md
source_task: 新增缺口；历史046 S03契约门槛缺现行维护与回归卡
architecture_prerequisite: "046 S03"
depends_on: []
---

# Plan 056：HTTP 契约、客户端生成与 CI 门禁

## 范围和技术验收

承接 NFR-001-104/107 的共享回归 `TECH-001-056`，不关闭产品 AC，也不替代历史046 S03进入实现前的真实门槛。当前FastAPI路由/Pydantic是唯一契约源，`/openapi.json` 派生 `frontend/src/api/`；本卡只维护实际故障和未来新增路由的漂移防线。不得手写DTO、编辑生成客户端或全局改写所有响应。

## 文件与 SPEC

| SPEC | 交付 |
|---|---|
| SPEC-056-API-001 | 修改 `backend/app/api/exception_handlers.py`、`middleware.py`、`docs.py`、`router.py`、`dependencies.py` 与 `core/errors.py`、`core/schemas.py`（仅故障相关文件）；每操作显式成功/错误类型、operation_id、request_id、422脱敏，非JSON/取消/网络错误保留协议语义。 |
| SPEC-056-CLIENT-001 | 修改 `frontend/src/request.ts`、`proxy.ts`、`frontend/package.json` 与必要的生成配置；由运行中的同提交FastAPI `/openapi.json` 执行 `pnpm openapi:generate`，生成 `frontend/src/api/`，随后 `pnpm openapi:check`、typecheck/build；禁止手工改生成物。 |
| SPEC-056-CI-001 | 修改 `.github/workflows/contract.yml`、`backend.yml`、`frontend.yml`、`runtime.yml` 中受影响门禁；启动同提交应用，生成并检测客户端差异、校验运行响应/OpenAPI 422/安全头、失败时CI失败。若工作流实际缺门禁，先以受控故障证明漏检。 |

测试文件责任：`backend/tests/integration/test_http_contract.py`、`test_http_contract_046.py` 及前端请求/路由相关现有测试；新增路由的业务断言仍归所属Issue。生成客户端与源路由同一切片审查，shared CI文件按索引串行。技术验收以本地命令和最终CI结果分别记录；开始运行的CI不能写通过。

## Checklist

- [ ] CHK-056-001 → API-001：保存运行响应与OpenAPI不符、422回显、网络/取消/非JSON误映射的失败用例。
- [ ] CHK-056-002 → CLIENT-001：从同提交服务生成，核对生成差异、operation_id、DTO与TypeScript消费者。
- [ ] CHK-056-003 → CI-001：使一个故意的契约漂移在CI受控失败，修复后检查最终全部必需job结论。
- [ ] CHK-056-G0-001：核对046 S03历史证据和当前路由、工作流、生成配置及文件责任。
- [ ] CHK-056-G1-001：冻结API/CLIENT/CI三项SPEC和业务卡拥有路由的接口交接。
- [ ] CHK-056-G2-001：保存CHK-056-001/003失败输出及不含敏感数据的复现条件。
- [ ] CHK-056-G3-001：修复契约/生成链，不全局放宽错误声明或静态忽略。
- [ ] CHK-056-G4-001：运行B/C/F、真实同提交应用OpenAPI与受影响CI；报告未运行的远端门禁。
- [ ] CHK-056-G5-001：受控技术回归适用；真实来源/平台旅程不适用，由业务Issue负责。
- [ ] CHK-056-G6-001：在共享Acceptance登记TECH-001-056、提交/CI身份和未覆盖业务路径。

失败时回退对应契约切片及生成客户端，保留失败CI和运行响应证据；不得把旧生成物手改为通过。
