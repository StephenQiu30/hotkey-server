---
layer: PRD
doc_no: "06"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
  - Ops
feature_area: backend-security
purpose: "定义企业级 P0 后端 LLM 供应商切换、异常降级/跳过策略及观测埋点的最小闭环。"
canonical_path: "docs/product/prd/06-LLM供应商切换与可观测性补齐PRD.md"
status: draft
version: "0.1.0"
owner: "StephenQiu30"
inputs:
  - "docs/engineering/验收标准.md（A2）"
  - "server/app/services/ai_analysis.py"
  - "server/app/services/ai/providers"
outputs:
  - "LLM Provider 可切换与异常容错的实现与验收标准"
  - "后续运维可查的 provider 调用链日志"
triggers:
  - "开启企业级 P0 任务 E4 后"
  - "AI key 切换或模型异常可观测性不足"
downstream:
  - "docs/plans/13-LLM供应商切换与容错计划.md"
  - "docs/engineering/验收标准.md"
---

# 企业级 LLM 供应商切换与可观测性 PRD（A2）

## 1. 背景

当前后端支持 `openai / deepseek / gemini / fallback` provider 注册，但异常时仅依赖“返回降级值”而无显式策略区分，也缺少事件级可追溯日志。企业级 P0 要求在模型异常时可明确执行降级或跳过，并提供可观测证据，避免“静默失败”。

## 2. 目标（SMART）

### 2.1 目标

- 在不改动现有热点链路的前提下，让 AI Provider 的行为可配置为：
  1. 按 `AI_PROVIDER` 选择主模型；
  2. 主模型异常时按策略执行 `fallback` 或 `skip`；
  3. 每一次失败、降级、跳过都可在日志中检索。
- 通过单测验证：切换 provider 不影响接口兼容，主模型失败会触发可验证的 fallback/skip 行为。

### 2.2 量化验收

- 单元测试覆盖主路径与异常路径。
- 主模型异常下，`fallback` 模式下分析返回 `provider="fallback"`，`used_fallback=True`。
- 主模型异常下，`skip` 模式下分析返回 `used_fallback=False` 且 `provider="skipped"`，并按现有规则安全降级。
- 日志至少包含 `ai_provider_selection`、`ai_provider_fallback`、`ai_provider_skip` 三类结构化事件。
- OpenAPI/route 不新增接口，仅通过配置行为变化交付。

## 3. 非目标

- 不引入企业级路由网关、模型成本计费控制台或复杂服务编排。
- 不在本阶段实现多租户/配额策略。
- 不替换现有 `AI` Prompt 结构，不重构 provider 实现细节。

## 4. 功能定义

- 配置新增：
  - `AI_PROVIDER_ERROR_STRATEGY`: `fallback | skip | error`（默认 `fallback`）
  - `AI_FALLBACK_PROVIDER`: 默认 `fallback`（保留扩展位）
- 行为定义：
  - `fallback`：主 provider 异常时切到 fallback provider 返回本地解析结果，`used_fallback=True`；
  - `skip`：主 provider 异常时返回跳过分析结果，不更新外部调用；
  - `error`：沿用当前异常抛出路径。
- 观测要求：
  - 任何 provider 失败必须输出事件日志；
  - fallback/skip 决策在日志与 `AiAnalysis.raw_response` 中可追溯。

## 5. 风险与审慎点

- 本地回退逻辑依赖 `fallback` provider 的稳定性；必须保证其永不抛异常。
- Skip 策略会让热点进入 `filtered`，需在运营文档中明确该场景可见性。
- 查询扩展与分析共享同一策略，若策略失效会影响两条链路。

## 6. 交付与验收

- 在 `server/app/core/settings.py` 增加上述策略配置。
- 在 `server/app/services/ai_analysis.py` 增加统一策略分支与日志埋点。
- 为策略、fallback/skip 与日志行为补充 `tests/test_mvp_services.py`。
- 更新 `docs/验收标准.md` 将 A2 勾选为“本轮执行中/完成待确认”。

## 7. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-24 | StephenQiu30 | 0.1.0 | 新建企业级 A2 补齐 PRD |
