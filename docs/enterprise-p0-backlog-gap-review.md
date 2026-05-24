# 企业级 P0 Backlog 缺口复核（2026-05-24）

基于 `docs/engineering/验收标准.md`、`openspec/changes/archive/2026-05-14-enterprise-p0-hotspot-platform/tasks.md`、及本次代码变更进行的复核结果。

## 已补齐（已回归验证）

| 功能项 | PRD | Plan | 验收依据 |
|---|---|---|---|
| A2 LLM Provider 异常策略与可观测 | [docs/product/prd/06-LLM供应商切换与可观测性补齐PRD.md](./product/prd/06-LLM供应商切换与可观测性补齐PRD.md) | [docs/plans/13-LLM供应商切换与容错计划.md](./plans/13-LLM供应商切换与容错计划.md) | `settings` 增加 `AI_PROVIDER_ERROR_STRATEGY` 与 `AI_FALLBACK_PROVIDER`；`ai_analysis` 支持 fallback/skip/error 分支；日志事件 `ai_provider_selection`、`ai_provider_fallback`、`ai_provider_skip`；`tests/test_mvp_services.py` 覆盖关键路径。 |
| A4 聚类版本化回溯 | [docs/product/prd/07-热点聚类去重与版本化回溯PRD.md](./product/prd/07-热点聚类去重与版本化回溯PRD.md) | [docs/plans/14-热点聚类与回溯查询计划.md](./plans/14-热点聚类与回溯查询计划.md) | `run_hotspot_check` 写入 `cluster_id` / `cluster_version` / `clustered_at`；新增 `/api/hotspots/cluster/{cluster_id}`、`/api/hotspots/{id}/cluster-history`；单测覆盖版本递增与查询。 |
| A8 RBAC 最小权限 | [docs/product/prd/08-安全增强-RBAC与权限治理PRD.md](./product/prd/08-安全增强-RBAC与权限治理PRD.md) | [docs/plans/15-安全增强与RBAC计划.md](./plans/15-安全增强与RBAC计划.md) | `users.role` 持久化、`require_permission()` 依赖、关键路由接入；`tests/test_mvp_services.py` 覆盖 viewer/admin 权限边界。 |
| 16 任务编排与执行序列 | [docs/product/prd/00-企业级AI热点监控平台PRD.md](./product/prd/00-企业级AI热点监控平台PRD.md) | [docs/plans/16-企业级P0任务编排.md](./plans/16-企业级P0任务编排.md) | A2/A4/A8 复核结果归并为批次任务 B/C 执行清单，避免与功能缺口重复定义。 |

## 已闭环的企业级 P0 缺口（2026-05-24 复核）

| 缺口 | 状态 | 验收依据 |
|---|---|---|
| 真实环境端到端（B1） | 已闭环 | `docs/acceptance/B1-回放证据包.md`：PostgreSQL/API 回放、热点入库、AI fallback、报告和通知落库完成；外部真实凭据缺失已记录为不可达证据。 |
| 企业级运维级验收（B2） | 已闭环 | `docs/acceptance/B2-健康可达性问题清单.md`：compose、API、Web、Nginx、PostgreSQL、Redis 启动与可达性验证通过，API restart 恢复演练通过。 |
| 监控与告警闭环（B3） | 已闭环 | `docs/acceptance/B3-告警演练结论.md`：`/api/ops/metrics` 指标基线、限流 429 告警演练与策略完成。 |
| 权限边界运维闭环（C1） | 已闭环 | `docs/acceptance/C1-运维手册补充.md`：admin/viewer 权限边界、403/token 排障与角色恢复步骤完成。 |
| 告警阈值与变更流程（C2） | 已闭环 | `docs/acceptance/C2-阈值与变更SOP.md`：阈值 v1、变更审批、回退流程、发布 runbook 完成。 |
| 开发执行口径补齐 | 已闭环 | D0 PostgreSQL 单库策略已通过 PR #40 追踪；B/C 阶段验收证据已进入 `docs/acceptance/`。 |

## 审核结论

- 企业级 P0 的代码级能力（A2/A4/A8/D0）与运营级验收（B1/B2/B3/C1/C2）均已有当前证据。
- 外部 X/Twitter、Bing、OpenAI、SMTP 真实成功调用依赖生产凭据；本轮在无凭据环境下记录为不可达证据，并验证 fallback/skipped 不阻断主闭环。
- Issue 关闭前应引用对应 `docs/acceptance/` 证据和本轮 PR。
