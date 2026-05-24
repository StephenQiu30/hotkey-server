---
layer: acceptance
doc_no: "C2"
audience:
  - PM
  - Dev
  - QA
  - Ops
purpose: "定义告警阈值 v1、变更回退流程和发布 runbook。"
owner: "StephenQiu30"
inputs:
  - "docs/product/prd/13-告警阈值标准化PRD.md"
  - "docs/plans/21-告警阈值与变更流程计划.md"
  - "docs/acceptance/B3-告警演练结论.md"
outputs:
  - "C2 阈值与变更 SOP"
triggers:
  - "关闭 Issue #19/#33/#34/#35 前"
downstream:
  - "docs/enterprise-p0-backlog-gap-review.md"
---

# C2 阈值与变更 SOP

验收时间：2026-05-24 15:48 Asia/Shanghai  
关联 Issue：#19、#33、#34、#35

## alerts-threshold-v1

| 编号 | 指标 | 阈值 | 等级 | 观测依据 |
|---|---|---|---|---|
| A-001 | `status_by_class.5xx` | 5 分钟新增 > 0 | P1 | API 应保持无 5xx |
| A-002 | `rate_limit_exceeded_total` | 5 分钟新增 > 10 | P2 | B3 演练 125 次请求触发 12 次 429 |
| A-003 | check run `failure_count` | 单次任务 > 0 | P2 | 外部凭据缺失、通知失败需人工确认 |
| A-004 | notification `status=failed` | 10 分钟新增 > 0 | P2 | SMTP 失败影响通知闭环 |
| A-005 | AI fallback in production | 连续 3 次任务出现 fallback | P2 | 生产模型配置下 fallback 代表模型链路退化 |

## 变更流程

1. 发起人提交阈值变更说明，包含指标、旧值、新值、原因、影响范围。
2. Reviewer 对照最近一次 `docs/acceptance/B3-告警演练结论.md` 或生产指标确认合理性。
3. 在测试/预发环境执行一次触发或非触发验证。
4. 合并 PR 后标注生效时间。
5. 24 小时内观察误报与漏报；异常则按回退流程恢复上一版。

## 回退流程

1. 记录误报/漏报触发时间和影响范围。
2. 恢复上一版阈值文档或配置。
3. 重新执行一次 `/api/ops/metrics` 查询确认指标读取正常。
4. 在对应 Issue/PR 里补充回退原因和复盘结论。

## 发布 runbook

| 告警等级 | 响应时效 | 处理动作 |
|---|---|---|
| P1 | 15 分钟内确认 | 查看 API 日志、数据库健康、最近发布；必要时回滚 |
| P2 | 30 分钟内确认 | 查看 check run、通知、外部凭据、限流来源 |
| P3 | 下个工作日 | 记录趋势，必要时调整阈值 |

## 阈值调整演练

本次按 B3 限流演练结果评估 A-002：

- 演练输入：125 次 `/api/health` 请求。
- 观测结果：`rate_limit_exceeded_total=12`。
- 决策：保留 `5 分钟新增 > 10` 为 P2 阈值。
- 回退：无需回退；若生产误报，则临时提高到 `> 30` 并保留 24 小时观察窗口。

## 验收结论

- #33 告警阈值建议版本：完成。
- #34 告警变更与回退流程：完成。
- #35 告警阈值发布 runbook：完成。
