---
layer: Plan
scope: issue
doc_no: "045"
title: 原始数据CSV与JSON导出执行计划
status: planned
version: v1.0
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/045-原始数据CSV与JSON导出执行计划.md
prd: docs/prd/007-扩展能力需求.md
design: docs/design/007-扩展能力设计.md
source_task: 从旧 TASK-007-S04-T01 的原始数据导出拆出；030只交报告
architecture_prerequisite: "046 S03"
depends_on: ["041"]
---

# Plan 045：原始数据 CSV 与 JSON 导出

从030分出数据文件交付，FR-007-001、AC-007-004的CSV/JSON部分；不依赖报告已生成。NFR-001-101/102/107。

文件责任补充：修改 `backend/app/core/config.py` 为 `content.export` 登记120秒进程硬截止，修改 `worker/app.py` 注册新kind并由 `worker/execution.py` 监督截止。先在 `backend/tests/unit/test_worker_execution.py` 保存未知kind失败，再注册；扩展 `backend/tests/integration/test_collection_jobs.py` 覆盖超时、取消、未知退出、重放与连续offset。路由变化从运行OpenAPI重生 `frontend/src/api/`，文件与源同批审查。

| SPEC | 文件与交付 |
|---|---|
| SPEC-045-API-001 | 新增 `backend/app/content/exports.py`，扩展 `content/schemas.py`，新增 `api/routers/content_exports.py`（prefix=/content-exports），在 `api/router.py` 聚合注册：POST `/api/content-exports`（createContentExport）输入operation_id、topic/source、[start,end)、format(csv/json)，返回202 JobAcceptedView；GET `/{export_id}`（getContentExport）返回状态/版本范围/行数；GET `/{export_id}/download`（downloadContentExport）owner授权流式下载，未就绪409、不可访问404。 |
| SPEC-045-DATA-001 | `content_export_requests` 保存owner、筛选、冻结内容版本ID清单、schema_version、format、状态、对象引用/哈希；写 `content/models.py` 和唯一schema。首次受理冻结版本，分页读取同一清单，运行中新增内容不混入。列固定content_id/source_key/external_id/url/title/body/author_id/author_name/published_at/first_observed_at/observed_at/指标及缺失标记；不导出凭据、内部错误栈或第三方未授权正文。 |
| SPEC-045-SEC-001 | CSV使用UTF-8、引号转义，对以=、+、-、@及控制字符开头的文本作公式防护并在格式说明注明；JSON保留null。时间ISO8601 UTC，CSV以空字段+缺失标记区分未知。合法空集输出带表头CSV或[]，UI明确0条，不谎称存在数据。 |
| SPEC-045-JOB-001 | 新增content.export处理器，复用jobs/worker，按冻结清单分块写受控临时文件，完成校验后存入既有MinIO桶并保留哈希；初始单请求上限10000条、硬截止120秒作为工程保护值，超限422提示缩小范围。取消/失败清理本次临时物，不改原内容。 |

- [ ] CHK-045-001 → API/DATA：新增 `backend/tests/integration/test_content_exports.py`，验证owner、快照中途新增/修改、分页无重复遗漏、字段/行数和10000/10001边界。
- [ ] CHK-045-002 → SEC/JOB：新增 `backend/tests/unit/test_content_exports.py`，覆盖CSV公式/引号/换行、JSON null、取消、MinIO失败、重放与下载授权。
- [ ] CHK-045-003 → AC-007-004：在 `frontend/src/app/content/components/content-list.tsx` 加导出操作及任务状态；浏览器导出真实数据两格式，核对10条源记录、哈希、失败提示和未授权下载拒绝。

运行 B、C、F 与真实PostgreSQL/MinIO集成；结果写M6 Acceptance Plan045。030只负责报告文件，四格式AC必须汇合两卡；回退停导出入口，不清空源数据。

## 阶段门禁

- [ ] CHK-045-G0-001：核对046 S03、041阅读产物、导出授权和现有kind白名单。
- [ ] CHK-045-G1-001：冻结API/DATA/SEC/JOB的字段、冻结清单、10000条及文件责任。
- [ ] CHK-045-G2-001：保存未知kind、公式注入、10001条、取消与越权下载失败测试。
- [ ] CHK-045-G3-001：完成CHK-045-001/002并注册可回收的新kind。
- [ ] CHK-045-G4-001：运行B/C/F、真实PostgreSQL/MinIO和端到端120秒截止/offset回归。
- [ ] CHK-045-G5-001：按CHK-045-003导出真实CSV/JSON并核对10条源记录。
- [ ] CHK-045-G6-001：登记AC-007-004的原始格式部分；报告格式由030汇合。
