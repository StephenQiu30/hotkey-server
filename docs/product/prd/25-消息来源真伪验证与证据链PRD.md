---
layer: PRD
doc_no: "25"
audience:
  - PM
  - Dev
  - QA
  - Ops
feature_area: source-trustworthiness
purpose: "建立来源真伪评估与证据链标准，支持可解释降权与留痕。"
canonical_path: docs/product/prd/25-消息来源真伪验证与证据链PRD.md
status: approved
version: "1.1.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/03-分阶段功能需求（P0/P1）.md
  - docs/engineering/验收标准.md
  - docs/product/prd/24-热点事件判定与热度引擎PRD.md
outputs:
  - truth_score 与 source_risk_level 标准口径
  - source_evidence_bundle 落库和归因字段
  - 低可信降权策略可解释落地
triggers:
  - "来源争议/误报复盘发生"
  - "新增来源接入或域名规则变更"
  - "新增证据维度时"
downstream:
  - docs/plans/25-消息来源真伪验证与证据链实现计划.md
  - docs/plans/28-里程碑与任务领取总控计划.md
---

# 消息来源真伪验证与证据链 PRD（v1）

## 1. 目标（SMART）

1. 为每条热点形成结构化来源证据，生成 `truth_score`（0-100）和 `source_risk_level`。
2. 未命中可信规则时给出 `source_risk_tags` 与可追溯 `source_evidence_bundle`。
3. 低可信事件采用“可解释降权”而非硬阻断，默认保留展示能力。
4. 证据与降权结果必须可在列表/详情中观察并可回溯。

## 2. 非目标

- 不引入付费反诈/反垃圾第三方服务。
- 不实现语义全文查重平台。
- 不改变消息源授权与登录模型。

## 3. 证据信号（v1）

- `source_reachable`：抓取是否成功。
- `url_stability`：URL 归一化与跳转链路是否健康。
- `domain_risk`：高风险域名、短链、重复重定向等规则分值。
- `publish_depth`：发布时间字段完整性与时效性。
- `cross_source_count`：当前批次内相同主题的跨源命中次数。

## 4. 真伪评分（v1）

`truth_score = round(0.35*source_reachable + 0.20*url_stability + 0.20*domain_risk + 0.15*publish_depth + 0.10*cross_source_count, 2)`

- 结果分档：
  - `trust_level=high`：`truth_score >= 80`
  - `trust_level=medium`：`60 <= truth_score < 80`
  - `trust_level=low`：`truth_score < 60`

## 5. 降权策略

- `source_risk_level=high`：进入 `filtered` 优先级更高。
- `trust_level=low`：应用可配置 `LOW_TRUST_PENALTY` 降低 `hotness_score`。
- 采集失败场景写 `source_evidence_bundle = {"status":"degraded"}`，`trust_level=medium`。

## 6. 数据与输出

新增字段建议：
- `truth_score`
- `truth_reason`
- `source_risk_level`
- `source_risk_tags`
- `source_evidence_bundle`
- `source_evidence_version`

## 7. 验收（Given/When/Then）

1. Given URL 含异常参数，When 采集证据，Then `url_stability=false` 且对应风险标签写入。
2. Given 多源同质事件，When 计数归并，Then `cross_source_count` 写入证据 bundle。
3. Given 证据采集超时，When 写入 `AiAnalysis`，Then 仍返回默认评分并进入降级链路。
4. Given `source_risk_level=low`，When 计算结果，Then 触发热度降权并可在日志中定位原因。

## 8. 可观测

- 每条事件需记录：`source_evidence_bundle`、`source_risk_tags`、`truth_score`、`source_risk_level`。
- 日志应包含 `fallback_reason` 与证据降级原因。

## 9. 风险

- 证据采集受外部网络影响：失败必须不阻断。
- 规则误伤风险域名：初期用高权重降级 + 审计开关逐步调参。

## 10. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-24 | StephenQiu30 | 1.1.0 | 真伪评估与证据链 PRD 执行化补全 |
