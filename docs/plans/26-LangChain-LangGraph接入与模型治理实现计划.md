---
layer: Plan
doc_no: "26"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: ai-orchestration
purpose: "完成 AI 接入分层治理：LangChain 主路径稳定，LangGraph 增强可控。"
canonical_path: docs/plans/26-LangChain-LangGraph接入与模型治理实现计划.md
status: approved
version: "1.1.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/26-AI模型接入抽象（LangChain-LangGraph）PRD.md
outputs:
  - AIOrchestrator 接口与注入
  - LangGraph 增强开关与触发规则
  - trace/provider_fallback 全链路可观测
triggers:
  - "AI 供应商或能力切换"
  - "增强编排上线"
downstream:
  - docs/plans/28-里程碑与任务领取总控计划.md
---

# LangChain / LangGraph 接入与模型治理实现计划（v1）

## 1. 目标

在 P2 篇章中交付 AI 接入的治理化实现：`LangChain` 为默认主路径；`LangGraph` 为条件增强；缺省关闭且可回退。

## 2. Issue 映射

- #50 P2: LangChain 默认模型接入层实现（analyze/expand/fact_check）
- #51 P2: 增强场景路由开关与触发器（AI_USE_LANGGRAPH=false）
- #52 P2: LangGraph 增强流程（仅高价值事件）
- #53 P2: fallback 到 LangChain、provider 切换、trace 与审计日志

## 3. 任务拆解（含文件）

### T1 Orchestrator 接口
- 新建：`server/app/services/ai/orchestrator.py`
- 修改：`server/app/services/ai_analysis.py`

### T2 LangChain 主路径
- 修改：`server/app/services/ai/providers/*`
- 新建：`server/app/services/ai/chain_factory.py`

### T3 LangGraph 增强与路由
- 新建：`server/app/services/ai/workflow.py`
- 修改：`server/app/core/settings.py`

### T4 可观测与回退
- 修改：`server/app/services/check_runner.py`
- 修改：`server/app/services/ai/chain_factory.py`

## 4. TDD 测试清单

- `test_orchestrator_uses_langchain_by_default`
- `test_langgraph_disabled_by_default`
- `test_langgraph_trigger_routes_to_graph`
- `test_langgraph_timeout_falls_back_to_chain`
- `test_open_endpoint_works_with_langchain`

## 5. 交付门禁

- 默认 `AI_USE_LANGGRAPH=false` 不改变既有主链路行为。
- 开启增强时 `ai_orchestrator_decision` 与 `enhanced_path` 可观测。
- 全量回退路径通过集成测试。

## 6. 风险与回滚

- 超时阈值过低导致增强不足：默认保守参数，逐步放开。
- 回退风险：保留 `provider_trace` 与 `fallback_reason`；任一异常回退到 LangChain。
