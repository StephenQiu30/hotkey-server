---
layer: Design
doc_no: "1"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
  - Ops
feature_area: "go-backend-rebuild"
purpose: "定义 HotKey 后端从现有 FastAPI 实现重建为 Go 开源后端的目标架构、分阶段范围和跨仓治理规则。"
canonical_path: "docs/engineering/1-Go后端重建与开源仓库治理设计.md"
status: approved
version: "1.1.0"
owner: "StephenQiu30"
inputs:
  - "AGENTS.md"
  - "docs/product/产品需求文档.md"
  - "docs/engineering/技术方案.md"
outputs:
  - "Go 后端目标架构"
  - "hotkey-* 跨仓命名与职责"
  - "分阶段实施边界"
  - "PRD 与 Plan 成对拆分规则"
  - "全局 AGENTS.md 治理要求"
triggers:
  - "后端重建启动"
  - "跨仓命名或职责调整"
  - "OpenAPI 事实源变更"
downstream:
  - "docs/plans/1-产品总览与阶段路线计划.md"
  - "AGENTS.md"
---

# 1-Go 后端重建与开源仓库治理设计

## 1. 背景

HotKey 项目将面向 AI 实时热点监测小程序重新设计后端服务。新后端采用 Go 技术栈，重建现有 `hotkey-server` 仓库，不继承当前 FastAPI 实现、目录结构、数据库结构、接口契约或任务链路。旧实现仅作为业务参考和迁移前历史，不作为新架构约束。

项目需要开源到 GitHub，并继续沿用当前 `hotkey-*` 多仓命名体系。`hotkey-server` 是后端主仓和 OpenAPI 契约事实源，`hotkey-web` 与 `hotkey-miniapp` 消费后端 OpenAPI 生成客户端。

## 2. 目标

1. 使用 `Go + PostgreSQL + pgvector + Redis + OpenAPI` 重建 `hotkey-server`。
2. 支持 AI 热点词设置、国内外多来源采集、相似事件聚合、来源证据链、热点详情展开和日报生成。
3. 将国外 AI 事实源作为一等来源，用事实层保证事件真实性，用传播层衡量热度和内容价值。
4. 保持 `hotkey-*` 仓库命名、跨仓职责、OpenAPI 事实源和全局 `AGENTS.md` 管理规则统一。
5. 把多租户、计费、复杂 RBAC、多服务拆分、秒级或近秒级实时、复杂消息队列、完整事件图谱写入目标架构，并按阶段实施。

## 3. 仓库命名与职责

仓库命名继续沿用 HotKey 现有体系：

```text
hotkey-server   # Go 后端，OpenAPI 事实源
hotkey-web      # Web 管理后台或创作者工作台，消费 OpenAPI
hotkey-miniapp  # 小程序端，消费 OpenAPI
```

`hotkey-server` 重建策略：

1. 保留仓库名和 GitHub 项目入口。
2. 清理旧 FastAPI 运行时实现，保留必要规范性文件、设计文档、开源治理文件和迁移说明。
3. 以 Go 模块化单体作为新服务起点。
4. OpenAPI 生成和导出仍由 `hotkey-server` 负责。
5. Web 与小程序不得手写后端 API 类型，必须从后端 OpenAPI 生成客户端。

## 4. 技术栈

| 层级 | 选型 |
| --- | --- |
| 后端语言 | Go |
| API | HTTP JSON API + OpenAPI 生成/导出 |
| 数据库 | PostgreSQL |
| 语义检索 | pgvector |
| 缓存与队列基础 | Redis |
| AI 能力 | OpenAI 兼容模型接口，后续可扩展多供应商 |
| 部署 | Docker / Docker Compose / 常规进程部署 |

Redis 是正式基础设施，不是可选组件。Redis 用于任务锁、限流、热点缓存、刷新队列、短期去重和后续消息队列演进入口。

## 5. 总体架构

新后端采用 Go 模块化单体，先在一个服务内按领域拆清边界，后续在流量、任务规模或团队协作需要时演进为 API + Worker 或多服务。

核心模块：

1. `identity`：小程序用户、管理员身份、后续租户身份。
2. `tenant`：组织、项目空间、租户隔离和配额边界。
3. `billing`：套餐、额度、用量、账单和支付回调。
4. `rbac`：角色、权限、策略、审计日志。
5. `keyword`：平台基础 AI 热点词库，用户关注、屏蔽、追加关键词。
6. `source`：国内外来源配置，事实层和传播层分类，可信等级、地区、语言和频率策略。
7. `collector`：来源适配器、采集调度、运行记录、失败重试。
8. `content`：原始内容标准化、hash 去重、语言识别、原始元数据保存。
9. `embedding`：内容向量化和 pgvector 相似召回。
10. `event`：关键词优先的热点聚合，逐步演进为完整事件图谱。
11. `llmreview`：高价值或边界模糊事件的 LLM 复核。
12. `trust`：事实证据、传播证据、来源可靠性、风险标签和冲突说明。
13. `ranking`：热度、相关度、可信度综合排序。
14. `report`：平台日报、用户日报、租户日报、定时或按需生成。
15. `api`：小程序 API、管理员 API、OpenAPI 契约导出。
16. `worker`：后续从模块化单体中拆出的异步任务执行单元。

## 6. 来源策略

来源分成两层。

事实层 `fact` 用于确认事件源头和可靠证据：

- OpenAI、Anthropic、Google DeepMind、Meta AI、Microsoft、NVIDIA。
- arXiv、GitHub Releases、GitHub Trending。
- 官方博客、RSS、权威英文技术媒体。

传播层 `signal` 用于判断传播速度、讨论量和内容创作价值：

- Bilibili、抖音、微博、小红书、视频号。
- YouTube、Hacker News、Reddit、X/Twitter、Product Hunt。
- 国内外科技媒体和公开社区。

来源接入必须遵循合规接入与授权优先原则。对无法稳定授权或存在平台限制的来源，先保留适配器接口、导入机制和配置位，不在开源仓库中提交绕过型采集逻辑、私钥、真实 token 或违反平台规则的实现。

## 7. 相似事件聚合

MVP 从关键词优先开始，后续演进为事件图谱。

聚合分三层：

1. 规则层：URL、hash、标题规范化、关键词命中、发布时间窗口、来源唯一性。
2. 向量层：使用 pgvector 对标题、摘要、正文片段做相似召回。
3. LLM 复核层：只复核高价值、跨源冲突或边界不清的候选事件，输出是否同一事件、合并理由和置信度。

低可信传播源只能贡献热度分，不得单独把事件推成高可信热点。同一事件如果同时命中国外官方事实源和国内传播层，应优先展示事实证据与传播证据的组合。

## 8. 核心数据模型

建议核心表：

1. `users`：用户、管理员、状态、登录来源。
2. `tenants`：租户、组织、套餐、状态。
3. `tenant_members`：用户与租户关系、角色。
4. `roles`、`permissions`、`role_bindings`：复杂 RBAC。
5. `billing_plans`、`usage_records`、`invoices`：计费、额度、账单。
6. `global_keywords`：平台基础词库、分类、同义词、优先级、启停。
7. `user_keyword_preferences`：关注、屏蔽、追加关键词。
8. `sources`：来源配置、事实层或传播层、地区、语言、可信等级、抓取频率和限流策略。
9. `source_items`：原始内容、URL、作者、发布时间、抓取时间、hash、原始元数据。
10. `content_embeddings`：向量、模型版本、向量对象引用。
11. `event_clusters`：事件簇标题、摘要、状态、主关键词、热度分、可信度分、相关度分。
12. `event_items`：事件和原始内容关联、匹配方式、相似度、是否主证据。
13. `trust_evidence`：官方证据、跨源印证、链接可达性、风险标签、争议说明。
14. `reports`、`report_items`：平台日报、用户日报、租户日报和事件引用。
15. `crawl_runs`、`analysis_runs`、`report_runs`：任务状态、错误、耗时和重试记录。
16. `audit_logs`：权限、配置、来源和计费相关操作审计。

## 9. 调度与实时策略

P0 不追求秒级实时，但目标架构必须支持后续近秒级能力。

分阶段策略：

1. P0：事实源每 2-4 小时执行一轮，传播源每 30-60 分钟执行一轮。用户手动刷新通过 Redis 限流和任务锁控制，同一用户或关键词在窗口期内返回缓存结果。
2. P1：对高优先级关键词、热度快速上升事件、付费租户提供更高频刷新。
3. P2：引入复杂消息队列、Worker 池、任务分片、失败补偿和任务优先级。
4. P3：支持秒级或近秒级热点检测，接入实时流、Webhook、平台推送或授权 API。

日报任务每日凌晨生成前一天平台日报。用户日报可按需生成，也可对活跃用户预生成。企业或租户日报在多租户能力完成后加入。

## 10. API 范围

小程序 API：

- `GET /api/v1/hotspots`
- `GET /api/v1/events/{id}`
- `POST /api/v1/keywords/follow`
- `POST /api/v1/keywords/block`
- `POST /api/v1/refresh`
- `GET /api/v1/reports/daily/platform`
- `GET /api/v1/reports/daily/me`

管理员 API：

- 平台关键词管理。
- 来源配置和启停。
- 采集任务查询和重试。
- 事件簇审查和合并。
- 日报生成和发布。
- 租户、RBAC、计费和审计管理。

契约 API：

- `hotkey-server` 必须生成并导出 OpenAPI。
- `hotkey-web` 和 `hotkey-miniapp` 必须从 OpenAPI 生成客户端。
- 接口变更必须先完成后端测试、OpenAPI 导出和端侧影响说明。

## 11. 分阶段实施

### P0：开源核心闭环

目标是让开源项目可运行、可验证、可扩展。

范围：

- Go 后端模块化单体。
- PostgreSQL schema、pgvector、Redis。
- OpenAPI 生成和导出。
- 平台关键词和用户关键词偏好。
- 至少一组国外事实源和一组传播源。
- 内容采集、标准化、去重。
- 规则 + pgvector 相似聚合。
- 基础证据链和来源可信标签。
- 平台日报和用户关注日报。
- 小程序 API 和基础管理员 API。

### P1：平台化能力

范围：

- 多租户、组织、项目空间。
- 复杂 RBAC。
- 审计日志。
- 租户级关键词、来源、日报和权限边界。
- 更完整的管理员后台 API。

### P2：商业化与规模化能力

范围：

- 计费、套餐、额度、用量统计和账单。
- 复杂消息队列。
- Worker 池、任务优先级、任务分片、失败补偿。
- API + Worker 拆分。
- 更多来源适配器和供应商治理。

### P3：高级实时与事件图谱

范围：

- 秒级或近秒级实时检测。
- 完整事件图谱。
- 跨语言事件归并。
- 传播路径分析。
- 事实源冲突仲裁。
- 多服务拆分和独立伸缩。

## 12. 全局 AGENTS.md 治理要求

重建后的项目必须在根目录维护全局 `AGENTS.md`，用于统一管理项目规则。该文件至少覆盖：

1. `hotkey-*` 仓库职责和跨仓协作顺序。
2. `hotkey-server` 是 OpenAPI 契约事实源。
3. Go 工程目录、模块边界、命名规则和单文件复杂度要求。
4. PostgreSQL、pgvector、Redis 和配置管理边界。
5. TDD、测试分层、验收标准和回归命令。
6. OpenAPI 导出、客户端生成和接口变更流程。
7. Git commit、PR、issue、release 和开源协作规范。
8. 文档目录、设计文档、计划文档和验收文档规则。
9. 密钥、token、平台授权和开源合规边界。
10. 多租户、计费、RBAC、消息队列和多服务拆分的阶段性原则。

## 13. 文档重编号与任务拆分治理

重建 `hotkey-server` 时必须同步整理 `docs/`，避免旧 FastAPI 时代的文档、计划编号和新 Go 架构混用。

### 13.1 文档目录

重建后长期文档按以下目录维护：

```text
docs/
  product/
    README.md
    prd/
      1-产品总览与阶段路线PRD.md
      2-Go服务基础工程与OpenAPI PRD.md
      3-关键词与用户偏好PRD.md
      4-来源与采集合规PRD.md
      5-内容标准化与去重PRD.md
      6-pgvector相似聚合PRD.md
  plans/
    1-产品总览与阶段路线计划.md
    2-Go服务基础工程与OpenAPI实现计划.md
    3-关键词与用户偏好实现计划.md
    4-来源与采集合规实现计划.md
    5-内容标准化与去重实现计划.md
    6-pgvector相似聚合实现计划.md
  engineering/
  acceptance/
  operations/
  archive/
```

`docs/product/prd/` 存放需求、范围、用户故事和验收口径；`docs/plans/` 存放与 PRD 一一对应的实施计划、任务拆解、测试清单、文件清单和回滚点。

### 13.2 PRD 与 Plan 配对规则

每个可实施任务必须先拆成一份 PRD 和一份 Plan。编号必须一致，便于追踪：

```text
docs/product/prd/N-能力名称PRD.md
docs/plans/N-能力名称实现计划.md
```

示例：

```text
docs/product/prd/3-关键词与用户偏好PRD.md
docs/plans/3-关键词与用户偏好实现计划.md
```

PRD 必须说明目标用户、业务范围、非目标、数据字段、API 影响、验收标准和风险。Plan 必须说明文件清单、任务拆解、TDD 测试清单、执行顺序、回滚点、OpenAPI 影响和验收命令。

### 13.3 重新编号规则

重建阶段属于全面重构，允许清理历史遗留文档并从 `1` 重新编号。新编号不保留 `00` 过渡编号，也不沿用旧 FastAPI 计划编号。

1. 新编号以 Go 重建后的目标架构为基准，不沿用旧 FastAPI 计划编号。
2. 新序列从 `1` 开始，所有 PRD 与 Plan 使用相同数字编号。
3. `1-13` 覆盖 P0 开源核心闭环。
4. `14-16` 覆盖 P1 平台化能力。
5. `17-19` 覆盖 P2 商业化与规模化能力。
6. `20-22` 覆盖 P3 高级实时与事件图谱。
7. 被替换但仍有参考价值的旧文档移入 `docs/archive/`，并在归档说明中标注被哪份新文档替代。
8. 与新架构冲突、无事实价值或仅为一次性过程记录的旧文档可删除，但删除前必须在计划中列出范围。

### 13.4 里程碑与 Issue 编排

一个 Epic 必须对应一个阶段里程碑，并同步到 GitHub 与 Linear。

1. P0 Epic 对应 `P0 Go 后端开源核心闭环` 里程碑。
2. P1 Epic 对应 `P1 平台化能力：多租户、RBAC、审计` 里程碑。
3. P2 Epic 对应 `P2 商业化与规模化能力` 里程碑。
4. P3 Epic 对应 `P3 高级实时与完整事件图谱` 里程碑。
5. Epic issue 和其下所有任务 issue 必须绑定同一个里程碑，并指定负责人。
6. 里程碑关闭前必须完成对应 PRD、Plan、实现、测试、验收证据、OpenAPI 影响和发布说明检查。
7. 里程碑推进过程中的临时记录、过程清单和一次性排查材料不进入 `docs/`。

### 13.5 首批任务建议

首批文档拆分应至少覆盖：

1. `1` 产品总览与阶段路线。
2. `2` Go 服务基础工程与 OpenAPI。
3. `3` 关键词与用户偏好。
4. `4` 来源与采集合规。
5. `5` 内容标准化与去重。
6. `6` pgvector 相似聚合。
7. `7` 事件证据链与可信度。
8. `8` 热点排序与详情 API。
9. `9` 日报生成。
10. `10` Redis 任务锁、限流与刷新队列。
11. `11` 小程序 API 契约。
12. `12` 管理员 API 契约。

P1、P2、P3 的多租户、复杂 RBAC、计费、复杂消息队列、多服务拆分、秒级实时和完整事件图谱，也必须按同样规则创建配对 PRD 与 Plan 后再实施。

## 14. 错误处理与降级

1. 单个来源失败不影响整轮任务，必须记录来源、错误类型、重试次数和影响范围。
2. AI、embedding 或 LLM 失败时，事件可用规则层结果入库，并标记 `analysis_pending` 或低置信。
3. Redis 不可用时，读接口返回历史结果；手动刷新、限流和任务锁进入降级状态并记录告警。
4. pgvector 检索失败时，退回关键词和标题规则聚合。
5. 低可信来源只能贡献热度，不能单独生成高可信事件。
6. 所有 AI 总结、日报和事件详情必须携带来源引用。

## 15. 验收标准

P0 完成时必须满足：

1. `hotkey-server` 已重建为 Go 服务，旧 FastAPI 运行时不再作为主实现。
2. 根目录有全局 `AGENTS.md`，且覆盖跨仓治理、OpenAPI 事实源、Go 工程规范和开源合规边界。
3. `docs/product/prd/` 和 `docs/plans/` 已按新架构重新编号，且每个实施任务都有配对 PRD 与 Plan。
4. 历史遗留文档已按计划归档或删除，不再与 Go 重建架构冲突。
5. PostgreSQL schema、pgvector、Redis 连接和本地启动路径可验证。
6. 可配置平台关键词和用户关键词偏好。
7. 可从至少一组国外事实源和一组传播源采集内容。
8. 可把相似内容聚合为事件簇，并展示相似度、匹配方式和合并理由。
9. 可展开热点事件，查看事实证据、传播证据和来源可信标签。
10. 可生成昨日平台日报和用户关注日报。
11. OpenAPI 可导出，Web 和小程序可基于契约生成客户端。
12. 任务失败、AI 失败、Redis 降级、pgvector 降级均有可追踪状态。

## 16. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-26 | StephenQiu30 | 1.0.0 | 初版，确认 Go 后端重建、hotkey-* 命名、完整目标架构与分阶段实施 |
| 2026-05-26 | StephenQiu30 | 1.0.1 | 增加 PRD 与 Plan 成对拆分、历史文档清理和从 1 重新编号规则 |
| 2026-05-26 | StephenQiu30 | 1.0.2 | 明确全面重构后新文档体系直接从 1 开始，不保留 00 过渡编号 |
| 2026-05-26 | StephenQiu30 | 1.1.0 | 全面清理旧 docs 后，将工程设计重新编号为 1 |
