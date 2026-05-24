---
layer: PRD
doc_no: "26"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: ai-orchestration
purpose: "明确 AI 调用职责边界：LangChain 为主路径，LangGraph 仅增强场景且默认关闭。"
canonical_path: docs/product/prd/26-AI模型接入抽象（LangChain-LangGraph）PRD.md
status: approved
version: "1.1.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/06-LLM供应商切换与可观测性补齐PRD.md
  - docs/product/prd/24-热点事件判定与热度引擎PRD.md
  - docs/engineering/技术方案.md
outputs:
  - AIOrchestrator 主路径抽象定义
  - LangChain/LangGraph 路由策略与开关
  - trace 与 provider fallback 观测字段
triggers:
  - 新增/变更 AI 能力或供应商时
  - 引入增强编排或补充验证流程时
downstream:
  - docs/plans/26-LangChain-LangGraph接入与模型治理实现计划.md
  - docs/plans/28-里程碑与任务领取总控计划.md
---

# AI 模型接入抽象（LangChain/LangGraph）PRD（v1）

## 1. 目标（SMART）

1. 建立统一 `AIOrchestrator` 接口，固定默认主路径为 LangChain。
2. 明确 `analyze / expand_queries / fact_check_basic` 三类主能力。
3. 为高价值/异常事件设置增强触发和 LangGraph 增强编排，默认开关关闭。
4. 所有 AI 分支都必须可回退到主路径，不阻断热点主链路。

## 2. 非目标

- 不把所有 AI 功能硬切到 LangGraph。
- 不在 P1/P2 引入长期会话记忆与复杂工作流。
- 不改变现有 `/api/daily-reports` 与热点核心语义。

## 3. 职责边界（强制）

### 3.1 LangChain 主路径（必须启用）

- `analyze`: 真实性与相关性基础分析。
- `expand_queries`: 查询扩展。
- `fact_check_basic`: 基础可疑内容提示。

### 3.2 LangGraph 增强路径（条件启用）

- 触发条件：高热度+真伪不足+来源冲突（至少一项满足）。
- 行为：复核、重打分、跨工具证据整合。
- 默认关闭：`AI_USE_LANGGRAPH=false`。

## 4. 配置与路由

- `AI_USE_LANGGRAPH`（默认 `false`）
- `AI_LANGGRAPH_TIMEOUT_SECONDS`
- `AI_ENHANCE_HOTNESS_THRESHOLD`
- `AI_ENHANCE_RISK_THRESHOLD`

路由失败与超时必须回退至 LangChain，并写明 `fallback_reason`。

## 5. 可观测

- 日志输出 `ai_orchestrator_decision`。
- 每次调用记录 `provider_trace`（provider 名称、耗时、错误、fallback 次序）。
- 关键路径必须关联 `trace_id`。

## 6. 验收（Given/When/Then）

1. Given `AI_USE_LANGGRAPH=false`，When 满足高价值条件，Then 仍走 LangChain。
2. Given 开关开启且触发条件满足，When 处理事件，Then 进入 `enhanced_path` 并写 `enhance_decision`。
3. Given LangGraph 超时，When 已触发增强，Then 自动回退到 LangChain。
4. Given provider 失败，When 进行 analyze，Then 记录 trace 并走可用 provider。

## 7. 风险

- 触发策略不当导致成本上升：默认关闭 + 阈值严格。
- 供应商失败面扩展：保留 LangChain 主路径和 provider 降级顺序。

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-24 | StephenQiu30 | 1.1.0 | PRD 收口：明确职责边界、默认关闭增强与回退要求 |
