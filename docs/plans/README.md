---
layer: Plan
scope: shared
doc_no: "000"
title: Plan 索引
status: planned
version: v1.0
owner: HotKey Team
canonical_path: docs/plans/README.md
---

# Plan 索引

本目录把 Design 与 PRD 转换为文件级、测试先行、可验证、可灰度和可回滚的执行计划。计划编制、现状审计、威胁盘点和评审准备可在 Design=`proposed`、PRD=`draft` 时进行；进入相应工程实现前必须通过 G1/G2，不能要求尚未执行的批准计划先批准自身。001–005 已通过 G1/G2/G3 并进入 `in_progress`。状态只使用 `planned`、`in_progress`、`blocked`、`completed`。

当前文档体系保持严格模式，不新增 `specs/` 或 `checklists/` 目录。每份 Plan 必须在正文内提供“实施规格（SPEC）”与“执行检查清单（CHECKLIST）”，并用稳定 ID 将 PRD 验收标准映射到任务、规格、检查项和最终 Acceptance 证据。

## 架构执行基线

所有计划都受以下边界约束：

- 后端保持 Go 模块化单体与单二进制，`cmd/hotkey` 的 `all`、`api`、`worker` 角色共享同一领域事实；
- 持久异步任务继续使用 PostgreSQL `river_job`、有界重试、租约恢复和幂等键，不引入 Kafka、Temporal 或内部事件总线；
- `backend/db/schema.sql` 是唯一 Schema 事实源，不创建 `migrations/`、分片 Schema 或自动迁移；
- `docs/openapi/swagger.json` 由 Swaggo 注解生成，前端只消费生成客户端；业务响应保持 `code`、`message`、`data`，成功业务码为 `0`；
- 前端继续使用 Next.js App Router、React、TypeScript、Tailwind CSS、现有 Radix 组合组件、Zustand、Axios、Recharts 与 GSAP；
- 全文检索使用 PostgreSQL FTS、`pg_trgm`、结构化筛选和高亮，不引入 Elasticsearch、向量检索、Embedding 或 RAG；
- 智能分析由根目录 `agent/` Python 服务执行，Go Worker 通过内部契约调用并保留最终业务校验；现有 Provider、Go Codex 与 Embedding 路径在替换验收和回滚演练完成前不得删除；
- 认证沿用现有身份、会话、JWT 与服务端 RBAC，不引入 Keycloak；
- PostgreSQL 是业务事实源，MinIO 保存原始证据，本地 Vault 保存人类可读知识投影，Redis 只承载短期状态与协同。

## 五个交付计划

| 编号 | Design | PRD | Plan | 24 周覆盖 | 状态 |
|---:|---|---|---|---|---|
| 001 | [HotKey 产品需求分析与总体架构](../design/001-HotKey产品需求分析与总体架构设计.md) | [PRD](../prd/001-HotKey产品需求分析与总体架构.md) | [Plan](001-HotKey产品需求分析与总体架构计划.md) | M0 与跨阶段约束 | in_progress |
| 002 | [监控、来源采集与证据链](../design/002-监控来源采集与证据链设计.md) | [PRD](../prd/002-监控来源采集与证据链.md) | [Plan](002-监控来源采集与证据链计划.md) | M1–M2 | in_progress |
| 003 | [智能研判、事件热度与人工治理](../design/003-智能研判事件热度与人工治理设计.md) | [PRD](../prd/003-智能研判事件热度与人工治理.md) | [Plan](003-智能研判事件热度与人工治理计划.md) | M3 | in_progress |
| 004 | [通知、报告、知识投影与检索](../design/004-通知报告知识投影与检索设计.md) | [PRD](../prd/004-通知报告知识投影与检索.md) | [Plan](004-通知报告知识投影与检索计划.md) | M4 | completed |
| 005 | [安全、运维、质量与交付](../design/005-安全运维质量与交付设计.md) | [PRD](../prd/005-安全运维质量与交付.md) | [Plan](005-安全运维质量与交付计划.md) | M5–发布观察 | in_progress |

## 24 周重排

| 周次 | 里程碑 | 主要范围 | 可验收出口 |
|---:|---|---|---|
| W1–W2 | M0 审计、范围与设计门禁 | 现状盘点、当前/目标差距、P0/P1 冻结、架构冲突清零、追踪矩阵、候选基线与开工检查 | G0–G3 通过；Design/PRD 已批准且 W3 首批 Plan 可进入执行 |
| W3–W6 | M1 薄垂直切片 | `Monitor → RSS → Evidence → Event → UI`，含 River 幂等、MinIO 原始证据和 PostgreSQL 事实链 | 一个真实 RSS 来源完成可重复端到端验收 |
| W7–W10 | M2 真实多来源 | RSS、Hacker News、X 官方来源；Rights、Credential、Quota、Checkpoint、部分成功和恢复 | 三来源真实授权冒烟；单来源失败不阻塞其余结果 |
| W11–W14 | M3 智能与热点 | Python Agent、Go 内部 Client、双端结构校验、Evidence 白名单、事件聚类、确定性 Heat、降级与人工治理 | Agent 可用和不可用两条路径均通过；错误输出不污染事实 |
| W15–W18 | M4 交付与知识 | WebSocket/REST 重放、日报与审批冻结、Vault 投影、PostgreSQL 全文检索、重建对账 | 通知、报告、知识和检索形成完整证据闭环 |
| W19–W21 | M5 治理与恢复 | 保留删除、审计、指标告警、容量、跨域安全收敛、备份恢复与回滚演练 | G5 前置运维材料和恢复证据就绪，可进入 W22 RC；不得表述为 G5 已通过 |
| W22 | RC | 全量 CI、契约、跨端 E2E、故障注入、性能与安全回归 | 无未处置 P0 缺陷，发布候选清单闭合 |
| W23 | UAT 与缓冲 | 真实用户故事、可访问性、来源授权复核、缺陷修正与复验 | UAT 签字；所有豁免有责任人和期限 |
| W24 | 发布与观察 | 生产预检、灰度、发布、回滚窗口、指标观察和复盘 | G6 通过；Acceptance 留存长期证据 |

以下能力不进入本轮 24 周承诺：评论正文与会话树、周报、新增或扩展邮件交付、更多来源、通用浏览器采集、Kafka/Temporal/Elasticsearch、其他业务微服务、Keycloak、Kubernetes 高可用和多地域部署。既有邮件能力只保持兼容，不扩大 P0；其他候选能力必须另立同编号三件套并重新评估人力与周期。

## 关键依赖与责任门禁

| 截止点 | 必须就绪 | 未就绪处理 |
|---|---|---|
| W2 / G2 | RSS/HN/X 的正式授权、配额、保留/删除义务和责任人；若 X 尚不可用，必须有明确 Go/No-Go | 回到 PRD 调整 P0 来源范围，不得用 Mock 冒充真实验收 |
| W10 / M3 前 | Codex 受信 Runner、认证注入、版本、预算、Sandbox/网络边界和回滚责任人 | 阻断 Codex 写路径，确定性链路继续；不得跳过 Live 门禁宣称可用 |
| W14 / M4 前 | Vault 根位置、备份介质、单写者/冲突责任人和人工区域恢复方式 | 只允许隔离目录投影，不开放正式知识发布 |
| W18 / M5 前 | Retention、恢复、发布、回滚、UAT、值班和事故响应责任人 | 不进入 G5 前置收敛，不以 W24 日期覆盖责任缺口 |
| W21 / RC 前 | Operations 001–006 的参数、隔离命令、成功输出、停止条件和回滚步骤 | 不进入 W22 RC；`planned` Runbook 不得冒充已演练 `active` 手册 |

## G0–G6 门禁

| 门禁 | 进入条件 | 退出条件 | 阻断规则 |
|---|---|---|---|
| G0 现状可信 | 仓库、Schema、OpenAPI、运行角色和基线命令可读取 | 当前实现、目标、差距和未知项均有证据；禁用架构不再被描述为当前能力 | 现状无法复现或仍混用交付包假设时阻断 |
| G1 设计接受 | Design 为 `proposed` | Design 为 `accepted`；关键取舍、安全、降级、迁移和回滚已评审 | 架构冲突、来源权利或事实源不清时阻断 |
| G2 范围批准 | PRD 为 `draft` | PRD 为 `approved`；P0 FR/NFR 到 AC 覆盖率 100%，P1 已移出 24 周承诺 | 无可测试 AC、容量环境未定义或外部授权无负责人时阻断 |
| G3 开工就绪 | Design accepted、PRD approved、Plan planned | 变更文件、测试先行步骤、SPEC、CHECKLIST、数据/契约顺序和回滚齐全，Plan 才可改为 `in_progress` | 任何 P0 AC 无 TASK/SPEC/CHK 映射时阻断 |
| G4 实现完成 | Plan in_progress | 目标测试、架构检查、Schema、OpenAPI、生成客户端和构建通过；实现与 SPEC 一致 | 放宽断言、手改生成物或静默改 Schema 时阻断 |
| G5 发布候选 | G4 通过 | E2E、性能、安全、故障恢复、备份恢复、Runbook 和回滚演练有证据 | 强制检查项未完成或只有目标描述没有结果时阻断 |
| G6 验收归档 | UAT 与发布观察完成 | Acceptance 为 `passed`；PRD=`implemented`、Plan=`completed`，长期证据可复查 | 未关闭 P0 缺陷、未验证回滚或证据不可重放时阻断 |

## 三件套状态推进

```text
Design: proposed ──G1──> accepted ───────────────> superseded
PRD:    draft    ──G2──> approved ───────G6─────> implemented
Plan:   planned  ──G3──> in_progress ────G6─────> completed
```

`blocked` 只表示执行期存在明确阻塞，不能替代范围决策。需求取消时 PRD 使用 `cancelled`，Plan 不得假装 `completed`。实现完成后由同编号 Acceptance 保存命令、结果、人工证据、已知限制和回滚验证；Plan 中勾选的检查项本身不是长期验收证据。

## 追踪与变更纪律

- PRD 的每个 `AC-NNN-xxx` 至少映射到一个 `TASK-NNN-Sxx-Txx`、一个 `SPEC-NNN-领域-xxx` 和一个 `CHK-NNN-Gx-xxx`；
- SPEC 领域只使用 `API`、`DATA`、`JOB`、`UI`、`SEC`、`OBS`、`OPS`；
- CHECKLIST 必须写明 AC 映射和预期证据；勾选后将实际证据转录到 Acceptance；
- 行为范围变化先更新 Design/PRD，再更新 Plan；实施细节变化只可在不扩大 PRD 的前提下更新 SPEC；
- Schema、OpenAPI、生成客户端、Compose 或运行入口变化时，源文件、生成物、测试和正式文档必须同批更新；
- 当前与目标必须分栏描述。候选性能指标只有在测试数据、硬件、命令和结果一并保存后才能升级为发布 SLO。
