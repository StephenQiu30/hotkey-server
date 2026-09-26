---
layer: Plan
scope: issue
doc_no: "040"
title: Codex分析调度与批处理执行计划
status: planned
version: v1.0
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/040-Codex分析调度与批处理执行计划.md
prd: docs/prd/002-信息获取主链路需求.md
design: docs/design/002-信息获取主链路设计.md
source_task: 从旧 TASK-002-S03-T01 的分析链路拆出；005只交标注有效性
architecture_prerequisite: "046 S03"
depends_on: ["002", "005"]
---

# Plan 040：Codex 分析调度与批处理

交付新入库内容持续进入分析、限流后保留积压且可恢复的链路。FR-002-005、NFR-001-104/106/107 与 AC-002-008；Plan 005 负责输出有效性，017 负责质量抽检，不能只修空值而遗漏任务生产与恢复。

| SPEC | 文件与行为 |
|---|---|
| SPEC-040-JOB-001 | 修改 `backend/app/analysis/services.py`、`worker/scheduler.py`：扫描缺当前内容版本/主题规则/提示词版本的有效标注，以这些身份的确定性 operation ID 建 Job/Outbox。无效标注允许有界再尝试，连续无效转可查询失败，不能每30秒无限重排；每日分析预算由既有账本约束。 |
| SPEC-040-DATA-001 | 复用 `pack_prompt_batches`、`analysis/schemas.py`、`analysis/prompts.py`；一批≤30条、序列化≤24000字符、正文≤1500字符、每帖评论≤50条。固定 content_version_ids 和实际评论样本/截断标记，旧 Job 不读取更新后的正文代替原输入。相关性、摘要/情感/观点一次结构化返回。 |
| SPEC-040-SEC-001 | `ai/adapters/codex_app_server.py`、`worker/app.py` 复用 app-server/experimentalApi、单 Job 子进程、空工作目录和最小环境；600秒硬截止、取消回收、限流显式延后。模型不可发起付费请求；外部正文不能授权读取文件或使用其他工具。记录 ai_call_id 与用量，不输出正文/秘密到日志。 |

- [ ] CHK-040-001 → JOB-001：`backend/tests/unit/test_scheduler.py`、新增 `backend/tests/integration/test_analysis_pipeline.py` 验证并发扫描只一单、已存在无效行可恢复、达到预算停止、重放不重复标注。
- [ ] CHK-040-002 → DATA-001：`backend/tests/unit/test_analysis_annotations.py` 验证30/31条、24000字符边界、多字节文字、50/51评论、任务受理后原文更新及单条无效隔离。
- [ ] CHK-040-003 → SEC-001：`backend/tests/unit/test_codex_app_server.py` 与真实子进程验证失败、取消、截止回收及最小环境；样本含提示注入文字，输出只接受数据字段。
- [ ] CHK-040-004 → AC-002-008：四关键词来源与六榜各一组真实命中，链路逐个记录入库→分析Job→ai_call→有效/失败结论；时效由 006/009 统一复算，不以单组样本关闭95%指标。

运行 B 门禁，证据归 M1 Acceptance 的 Plan 040。关闭分析扫描可回退，已入库内容与待分析数保留，不将失败写为“不相关”。

## 阶段门禁

- [ ] CHK-040-G0-001：核对046 S03、005受控有效性产物、预算账本与当前调度注册。
- [ ] CHK-040-G1-001：冻结JOB/DATA/SEC的批量上限、输入版本、进程隔离和重放边界。
- [ ] CHK-040-G2-001：保存31条/24000字符、旧正文、注入、取消及预算耗尽失败测试。
- [ ] CHK-040-G3-001：完成CHK-040-001—003；真实模型样本不得补写005的前置技术结论。
- [ ] CHK-040-G4-001：运行B、真实PostgreSQL/Kafka及受控子进程截止回归。
- [ ] CHK-040-G5-001：执行CHK-040-004，逐来源保存真实入库→Job→ai_call→有效/异常结论。
- [ ] CHK-040-G6-001：登记AC-002-008链路部分；60分钟比例由006/009计算。
