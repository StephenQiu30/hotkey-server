---
layer: Plan
scope: issue
doc_no: "030"
title: 报告Markdown与PDF导出执行计划
status: planned
version: v1.1
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/030-报告Markdown与PDF导出执行计划.md
prd: docs/prd/007-扩展能力需求.md
design: docs/design/007-扩展能力设计.md
architecture_prerequisite: "046 S03"
source_task: TASK-007-S04-T01 的报告部分；原始数据迁045
depends_on: ["018", "019"]
---

# Plan 030：报告 Markdown 与 PDF 导出

## 范围与需求

交付已final报告的Markdown/PDF下载；FR-007-001、NFR-001-101/107、AC-007-004的报告部分。CSV/JSON已拆045，四格式AC在两卡汇合。本卡不影响报告生成或信息获取的完成判定。

## SPEC 与实现路径

| ID | 具体契约 |
|---|---|
| SPEC-030-API-001 | 新增 `backend/app/reports/exports.py`；扩展reports models/schemas、schema、Worker注册。`api/routers/reports.py` 提供POST `/api/reports/{report_id}/exports`（createReportExport），输入operation_id、format(md/pdf)、固定report_version→202；新增 `api/routers/report_exports.py`（prefix=/report-exports）并在 `api/router.py` 聚合注册，GET `/api/report-exports/{id}`（getReportExport）返回pending/running/succeeded/failed/hash；GET `/{id}/download`（downloadReportExport）owner授权流式下载，未就绪409、不可访问404、非法格式422。 |
| SPEC-030-DATA-001 | `report_exports` 唯一(owner,report_id,version,format,renderer_version)，保存产物对象引用/hash/字节数/状态。Markdown取存档原文；PDF保留版本、时间窗、数字、来源、缺口、模板版标记和可点引用。重放复用相同产物，失败重试保留attempt。 |
| SPEC-030-SEC-001 | PDF用现有Playwright/browser边界渲染受限本地HTML，禁止导航/远程资源/脚本/file URL；渲染接口归适配边界、运行装配归Worker。中文字体和分页固定renderer_version，环境缺字体返回明确失败。无需新服务；不能把正文当浏览器指令。 |
| SPEC-030-JOB-001 | `report.export` 初始硬截止120秒、文档上限5MB为工程保护值；超限失败不返回截断成功文件。临时文件校验格式/字节/hash后存既有MinIO桶，取消/失败清本次临时物，不删存档。 |
| SPEC-030-UI-001 | 修改 `frontend/src/app/reports/[reportId]/components/report-detail.tsx` 提供两格式操作、任务/失败状态，调用生成客户端；下载再次授权，非JSON流不被全局响应封装改写。 |

## Checklist 与验收

文件责任补充：修改 `backend/app/core/config.py`，为 `report.export` 登记120秒进程硬截止；修改 `worker/app.py` 的处理器注册和 `worker/execution.py` 的进程截止路径。先在 `backend/tests/unit/test_worker_execution.py` 验证未知kind受理/执行会失败，再加入新kind；扩展 `backend/tests/integration/test_collection_jobs.py` 验证超时、取消、异常退出、重放及Kafka连续offset。生成 `frontend/src/api/` 与路由同批核对，不能只验证PDF渲染函数。

- [ ] CHK-030-001 → API/DATA：新增 `backend/tests/integration/test_report_exports.py` 验证owner、固定旧版本、并发重放、5MB边界、MinIO失败、下载Content-Type/Disposition。
- [ ] CHK-030-002 → SEC/JOB：新增 `backend/tests/unit/test_report_exports.py`，HTML/远程图片/file链接不造成网络或文件读取，渲染失败无成功对象；真实browser检查中文/分页/链接。
- [ ] CHK-030-003 → UI/AC-007-004：浏览器导出真实日报与周报两格式，逐项对照存档数字/版本/引用与降级标记，文件可打开可读；PDF未通过只能报告Markdown局部结果。

运行B/C/F和真实browser/MinIO验证，DDL保留库先051；证据写M6 Acceptance Plan030。回退停导出入口，保留原报告和失败状态，不把空文件/扩展名/HTTP200当导出成功。

## 阶段门禁

- [ ] CHK-030-G0-001：核对046 S03、018/019报告版本、现有kind白名单与共享Worker文件顺序。
- [ ] CHK-030-G1-001：冻结API/DATA/SEC/JOB/UI五项SPEC及core/config.py、Worker和生成客户端文件责任。
- [ ] CHK-030-G2-001：保存未知kind失败、120秒超时、取消、越界资源、下载越权的失败测试摘要。
- [ ] CHK-030-G3-001：实现两格式导出、注册和进程回收，验证源报告未被覆盖。
- [ ] CHK-030-G4-001：运行B/C/F、真实PostgreSQL/MinIO/browser与端到端进程截止回归。
- [ ] CHK-030-G5-001：按CHK-030-003打开真实MD/PDF并逐字段比对存档。
- [ ] CHK-030-G6-001：记录AC-007-004的报告格式部分；CSV/JSON由045汇合。
