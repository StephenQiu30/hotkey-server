# 13-LLM供应商切换与容错计划

## 目标

在不改动热点主流程的前提下，补齐企业级 A2 的 provider 切换、异常策略与可观测日志。

## 范围与不变项

- 范围：
  - 新增 AI provider 异常策略配置；
  - 扩展分析失败/降级/跳过策略；
  - 增加结构化日志与 `tests/test_mvp_services.py` 覆盖。
- 不变项：
  - 现有 `/api/reports`、`/api/search` 等接口形态不变；
  - 现有 fallback 规则（本地解析）作为兜底策略。

## 任务拆解

- **P0-A2-1：配置与策略模型**
  - 在 `server/app/core/settings.py` 新增：
    - `ai_provider_error_strategy`
    - `ai_fallback_provider`
  - 写单测：
    - 配置加载不影响默认行为（fallback）。

- **P0-A2-2：分析链路策略分支**
  - 在 `server/app/services/ai_analysis.py` 增加：
    - `analyze_hotspot` 主路径异常捕获分支；
    - `expand_keyword_queries` 失败策略；
    - `provider_event` 日志；
    - `skip` 模式下可回溯的 `AnalysisResult`。
  - 写单测：
    - `fallback` 模式切换到 fallback 后的结果；
    - `skip` 模式不再调用 fallback 且返回可解释事件。

- **P0-A2-3：日志与可观测交付**
  - 在日志中输出 `ai_provider_selection` / `ai_provider_fallback` / `ai_provider_skip`；
  - 测试 `openai` 失败时调用顺序和日志触发。

## 依赖与顺序

- 先完成配置和链路分支，再补测。
- 先提交 `test:`（新增失败/降级测试），再提交 `impl:`。

## 验收

- 测试通过；
- 相关 openapi 不变，功能行为可由单测验证；
- `docs/engineering/验收标准.md` 的 A2 状态更新为“已完成/待复核”。
