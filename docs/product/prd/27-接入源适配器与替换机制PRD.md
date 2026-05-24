---
layer: PRD
doc_no: "27"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
  - Ops
feature_area: source-ingestion
purpose: "建立可替换、可降级的来源接入机制，保障热点抓取高可用与可观测。"
canonical_path: docs/product/prd/27-接入源适配器与替换机制PRD.md
status: approved
version: "1.1.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/00-企业级AI热点监控平台PRD.md
  - docs/plans/11-后端热点检测与报告计划.md
  - docs/engineering/技术方案.md
outputs:
  - SourceAdapter 与 SourceSelector 设计
  - 来源健康检查与回退策略
  - 来源状态日志可观测字段
triggers:
  - 新来源接入
  - 来源稳定性异常频发
downstream:
  - docs/plans/27-接入源适配器改造与可替换实现计划.md
  - docs/plans/28-里程碑与任务领取总控计划.md
---

# 接入源适配器与替换机制 PRD（v1）

## 1. 目标（SMART）

1. 定义统一来源输入接口与选择器，支持来源级健康驱动的路由。
2. 单源失败时不阻断主批次，可自动切换到下一可用来源。
3. 来源失败、延迟、成功率信息落库并可审计。
4. 支持配置化灰度与快速回退。

## 2. 非目标

- 不建设统一外部爬虫平台。
- 不引入外部付费来源计费系统。
- 不改写用户与认证模型。

## 3. 适配器目标

- 标准入口：`fetch_hot_topics`。
- 标准输出：`Candidate`。
- 统一错误模型：失败不抛异常中断，写日志并继续后续来源。

## 4. 路由与健康策略

- 主排序：`weight` + `health_score`。
- 失败超过阈值（默认 3 次）将来源标记为 `degraded` 或暂时隔离。
- 连续超时优先回退 `source_fallback`。

## 5. 配置项

- `SOURCE_HEALTH_WINDOW_SECONDS=300`
- `SOURCE_FAILURE_THRESHOLD=3`
- `SOURCE_TIMEOUT_SECONDS=8`
- `SOURCE_MAX_CONCURRENCY=4`

## 6. 验收（Given/When/Then）

1. Given 单来源连续失败 3 次，When 下一轮抓取，Then 自动切换到下一来源。
2. Given 主来源恢复，When 切回策略评估，Then 可重新纳入候选路由。
3. Given 失败来源触发告警阈值，When 检查日志，Then 能定位 `source_selected`、`source_fallback`。
4. Given 新增适配器注册，When 启动，Then 既有来源不受影响。

## 7. 风险

- 失败策略过度敏感导致抖动：使用时间窗与最小冷却。
- 回退频率高造成吞吐下降：结合并发与健康窗口联动。

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-24 | StephenQiu30 | 1.1.0 | 接入源 PRD 收口为可替换与健康路由策略 |
