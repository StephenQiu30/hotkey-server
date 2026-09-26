---
layer: Plan
scope: issue
doc_no: "036"
title: SearXNG新闻搜索执行计划
status: planned
version: v1.0
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/036-SearXNG新闻搜索执行计划.md
prd: docs/prd/002-信息获取主链路需求.md
design: docs/design/002-信息获取主链路设计.md
source_task: 新增缺口；四关键词来源需SearXNG独立真实采集卡
architecture_prerequisite: "046 S03"
depends_on: ["031", "033"]
---

# Plan 036：SearXNG 新闻搜索

交付 `news_search` 从本机 SearXNG 的 `duckduckgo news` 入口到持久内容和缺口的链路。承接 FR-002-003、AC-002-002/004、NFR-001-101/102/105/106/111；SearXNG 进程健康不等于引擎有结果。

| SPEC | 实施路径与契约 |
|---|---|
| SPEC-036-API-001 | 修改 `backend/app/sources/adapters/web_search.py`、`connections/presets.py`；只请求配置的 `127.0.0.1:8888` JSON 搜索入口，固定 `duckduckgo news`，记录请求词、页号、实际引擎和 unresponsive/timeout 信息。响应有 error/失败引擎时不能因 results=[] 记成功空。 |
| SPEC-036-DATA-001 | `content/discovery.py` 用来源原生 ID，缺失则规范 URL 构造稳定身份并记录依据；只去除明确追踪参数，保留有语义 query。发布日期与首次发现分离；摘要不是完整正文，内容版本保留字段可用性。 |
| SPEC-036-JOB-001 | `content/discovery_execution.py` 只在支持并有预算时翻页，页重复/无可信尾段/有限结果均保留缺口；同页重放不重复持久计量。适配器仅访问本机服务，结果链接不作为自动抓正文入口。 |

- [ ] CHK-036-001 → API-001：扩展 `backend/tests/unit/test_keyword_search_adapters.py`，覆盖引擎失败与合法空、非 JSON、慢响应、重复页、没有 publishedDate。
- [ ] CHK-036-002 → DATA-001/JOB-001：在 `backend/tests/integration/test_keyword_discovery.py` 验证 URL 身份、窗过滤、部分页入库、预算停止、重放和请求页数对账。
- [ ] CHK-036-003 → AC-002-002/004：真实本机引擎产生可打开原帖，记录实际引擎与 JSON 结果、Job/内容版本/窗口；第二次扫描不重复身份。外部引擎不可用时来源保持失败并显示原因。

运行 B 门禁；前端复用 041、覆盖复用 034。真实结果写 M1 Acceptance 的 Plan 036，不跨库拼接旧计数，不以 HTTP 200 关闭 AC；72 小时由 009。回退仅停本来源，无需更改其他来源配置。

## 阶段门禁

- [ ] CHK-036-G0-001：核对046 S03、031/033技术产物和本机引擎配置。
- [ ] CHK-036-G1-001：冻结API/DATA/JOB的实际引擎、错误、URL身份与分页边界。
- [ ] CHK-036-G2-001：保存HTTP200但引擎失败、非JSON、重复页、预算耗尽的失败测试。
- [ ] CHK-036-G3-001：完成CHK-036-001/002，部分结果和错误不伪装完整空集。
- [ ] CHK-036-G4-001：运行B及真实PostgreSQL/Kafka计量重放。
- [ ] CHK-036-G5-001：执行CHK-036-003真实本机引擎与原帖核对。
- [ ] CHK-036-G6-001：登记AC-002-002/004的本来源结果；持续窗由009。
