---
layer: Design
scope: shared
doc_no: "001"
title: HotKey产品需求分析与总体架构设计
status: accepted
version: v1.0
owner: HotKey Team
canonical_path: docs/design/001-HotKey产品需求分析与总体架构设计.md
prd: docs/prd/001-HotKey产品需求分析与总体架构.md
plan: docs/plans/001-HotKey产品需求分析与总体架构计划.md
---

# HotKey 产品需求分析与总体架构设计

## 现状

HotKey 已是包含 Go 后端、根目录 Python Agent 与 Next.js Web 工作台的单仓库项目。后端由 `cmd/hotkey` 单一入口启动，以 `all`、`api`、`worker` 三种角色运行模块化单体；业务模块遵循 Transport、Application、Domain、Infrastructure 的依赖方向。PostgreSQL 保存业务事实，Redis 保存验证码和短期状态，MinIO 保存原始证据，本地 Vault 保存人类可读投影，River 兼容表与 Go Worker 承担持久任务。根目录 `agent/` 已落地 Python 数据分析服务骨架、`analysis.v1` 版本化内部契约、服务认证、请求/响应边界、健康检查与双端契约测试；当前仅以 `deterministic.v1` 降级运行时接入可选 Shadow 比较，尚未进入 Live 决策或业务事实写入。前端通过发布 OpenAPI 生成客户端，并使用 Axios、Zustand 与 Recharts 组织请求、状态和可视化。

当前仓库已经存在多种 AI Provider、Embedding、pgvector 数据与相关召回实现。这些是必须保护和盘点的当前事实，不代表新基线继续承诺同一产品边界，也不表示它们已经迁移或移除。任何替换都必须通过独立 Plan、数据处置、兼容验证和回滚门禁完成。

用户当前面对的主要问题仍然成立：公开信号分散、关键词噪声高、孤立内容不能直接表达事件、绝对互动量不能说明近期升温、自动摘要容易脱离出处、长期观察结果难以持续整理。新基线需要把这些问题收敛成一条可解释、可恢复、可审计的热点情报闭环。

## 目标

| 需求 ID | 目标 |
|---|---|
| BR-001-001 | 系统必须把分散的授权公开信息转化为可追溯、可治理的热点事件，而不是不可解释的内容列表。 |
| BR-001-002 | AI 只能提供结构化建议；业务事实、权限和最终状态必须由 Go 应用规则与人工治理决定。 |
| BR-001-003 | 日报和长期知识必须同时支持自动生成效率、证据审计和人工持续维护。 |
| BR-001-004 | P0 必须可在本地或单机自托管环境运行，不以新增分布式基础设施为前提。 |

上述需求在设计上进一步约束为：业务事实由 Go 模块化单体与 PostgreSQL 事务拥有，异步执行通过 River 与 Go Worker 恢复；Python Agent 只处理有界数据分析并返回版本化结构建议；用户可见结论追溯到输入、运行与 Evidence；前端通过生成的 OpenAPI Client 交互并展示五态；Agent 不可用时，授权采集、证据保存、确定性去重和既有事实查询仍可继续。

目标主链为：

```text
Next.js Web 工作台
  -> Go API Transport
  -> Application 用例与 Domain 规则
  -> PostgreSQL 业务事实 / River 持久任务
  -> Go Worker 读取已授权的有界上下文
  -> 内部 Python Agent 数据分析服务（无业务存储权限）
  -> Go Application/Domain 校验并提交建议
  -> 官方来源 Connector、MinIO 证据、本地 Vault 投影
  -> 邮件与 WebSocket 等受控交付
```

智能任务的批准运行面是根目录 `agent/` 中的 Python 数据分析服务。Go Worker 从 PostgreSQL 读取任务身份，并经拥有模块取得必要的有界上下文，以内部版本化 HTTP 契约调用 Agent；Agent 只执行相关性、聚类候选、摘要、实体/主题提取与模型编排，返回经 Pydantic/JSON Schema 校验的结构化建议。Go Application 和 Domain 再执行授权、Evidence 白名单、幂等、状态迁移与最终写入。Agent 不接收数据库 DSN、来源凭据、MinIO/Vault 凭据或用户会话，不形成第二事实源。当前工程骨架和 Shadow 接线证明了服务与契约边界，但真实模型质量、Live 灰度和切回仍以 003 Plan 的 Golden、Shadow、Live 门禁为准。

## 非目标

- 不把附件中的架构图或实施命令视为已批准指令。
- 除批准的 Python Agent 分析服务外，不继续拆分业务微服务，不建立第二套业务后端或第二套 Schema。
- 不引入 Kafka、内部事件总线、Temporal 或 Elasticsearch。
- 不引入 Keycloak 替换当前身份系统。
- 不创建 `db/migrations/`、Goose、Atlas 或 GORM `AutoMigrate`；`backend/db/schema.sql` 继续是唯一 Schema。
- 不在本设计中声明现有多 Provider、Embedding 或 pgvector 已删除。
- 不承诺自动判断事实真伪，不以热度、相关性或评论情绪代替证据状态。
- 不采集私密内容，不绕过登录、验证码、反爬或平台访问限制。

## 核心决策

### DEC-001-001：保持模块化单体

Go Core 继续是唯一业务后端和事实拥有者。`all` 用于小规模一体运行，`api` 与 `worker` 允许在不改变代码边界和数据所有权的前提下独立扩容。跨模块只调用目标模块 Application 接口或只读查询端口。

### DEC-001-002：使用 PostgreSQL 持久任务编排

长链路拆成有界、幂等的 River 任务；队列载荷只携带 ID、版本、时间窗和输入哈希，正文与证据从拥有模块重新读取。任务状态、尝试记录和业务事实同库可核对，不增加分布式事件基础设施。

### DEC-001-003：Python Agent 执行分析，Go Core 保留业务决策

Python Agent 只返回经版本化契约校验的建议，不直接读写 PostgreSQL、Redis、MinIO 或 Vault。Go Worker 保留任务恢复、超时、重试和幂等，Go Domain 保留阈值、状态迁移、引用白名单、预算和最终写入决定。现有 Go AI Provider、Codex CLI Adapter 与向量事实的迁移方式属于未决实施项；迁移完成前不得破坏现有读取、审计和回滚能力。

### DEC-001-004：保持现有前后端契约

业务 JSON 只返回 `code`、`message`、`data`，成功业务码为 `0`，无数据为 `data: null`。OpenAPI 由后端注解生成到 `backend/openapi/docs.go` 与 `docs/openapi/swagger.json`，前端不得手写路径或 DTO。

### DEC-001-005：三类存储各司其职

- PostgreSQL：业务事实、状态、任务、审计和可重建搜索字段。
- MinIO：原始响应、正文快照和其他大对象证据。
- 本地 Vault：供人阅读和维护的批准后投影，不反向成为核心业务事实源。
- Redis：验证码、短期票据、缓存和限流，不保存长期事实。

### DEC-001-006：关键写入与列表读取使用稳定并发语义

关键写操作在 Application 边界依次完成认证、服务端授权、输入与资源版本校验，并使用业务幂等键或乐观锁阻止重复副作用和静默覆盖；成功、拒绝与版本冲突均形成不可覆盖的审计事实。列表接口使用不可变排序字段与唯一 ID 组成稳定顺序和游标，限制页大小并对无效、过期或越权游标返回稳定脱敏错误；并发新增不得回流到已经越过的游标区间，同一次连续遍历不得因并列排序产生重复或遗漏。

### 未采纳候选与附件差异

根据产品负责人对交付设计的复核，Python Agent 是获批的数据分析运行面，不再作为禁用候选。Kafka、Temporal、Elasticsearch/ELK、Keycloak、版本化 migrations 和完整业务微服务拓扑仍不采用。Agent 必须保持无状态、内网可达和最小权限，不能成为第二事实源；本地优先、证据可追溯、任务可恢复和自动判断受控继续由 Go Core 与 Agent 的明确契约共同保证。

## 数据与状态

- `backend/db/schema.sql` 是结构唯一事实源；服务启动只验证兼容性，空库通过显式命令初始化。
- 业务记录使用当前 bigint identity、版本字段、时间戳、软删除或追加事实约束；本设计不引入平行 UUID 身份体系。
- River 任务至少区分可执行、执行中、成功终止、失败终止和取消，并保存尝试历史；具体枚举以当前 Schema 与队列包为准。
- 监控、采集、文档、匹配、微事件、证据状态、通知和审计分别由拥有模块维护，禁止用一个通用状态字段代替领域状态。
- 目标设计、PRD 和 Plan 的状态不得被解释为代码实现状态；只有 Acceptance 保存验证证据。

## 接口与交互

- Web 首要工作流为创建或编辑监控、预览规则、发布、查看扫描、阅读内容证据、查看热点事件、接收通知和执行治理操作。
- API 使用现有认证上下文、角色门禁、统一 Result 和全局错误映射。
- 关键写入使用业务幂等键或资源版本前置条件；列表使用稳定排序元组、唯一 ID 游标和有界页大小，不使用易受并发插入影响的无序偏移。
- 当前服务端已使用 Viewer/Analyst/Editor/Admin 四角色；Analyst 由 Admin 显式分配，只能管理自有 Monitor、手动扫描和相关性反馈，不继承 Editor 的审核能力或 Admin 的用户、来源与运行治理能力。
- 长耗时操作返回持久任务或运行身份，前端轮询或通过 WebSocket 获取进度，不维持超长 HTTP 请求。
- 所有新接口先更新后端 DTO、错误码、OpenAPI 与 Transport 测试，再生成前端 Client。
- 前端只展示服务端提交的 Relevance、Heat 与 Evidence 事实及其解释，不在浏览器内重算或覆盖权威状态。
- 桌面与移动端都必须提供键盘操作、可见焦点、语义化标签、合理对比度、`prefers-reduced-motion` 支持和清晰的空/错/权限状态。

## 安全与合规

- 来源只允许官方 API、RSS、Atom 或明确授权 Feed；每次调用前检查来源权利、凭据、配额、端点和 SSRF 规则。
- JWT、认证 HMAC 和来源凭据主密钥按环境独立管理；任何 API、日志或文档不得回显明文凭据。
- Agent 上下文中的来源文本一律是不可信输入；Agent 镜像不挂载仓库源码，不获得数据库 DSN、来源 Token、MinIO/Vault 凭据或任意未批准网络访问权。
- Evidence ID 只能从任务清单引用；未知引用、越权对象、超长结果或无效 Schema 必须拒绝。
- 审计、删除、保留和恢复必须覆盖 PostgreSQL、MinIO 与 Vault 的关联关系。

## 失败与降级

- 单一来源失败只影响该采集目标；其他来源和已保存结果继续处理，界面显示覆盖缺口。
- Worker 重启后从持久任务与拥有模块事实恢复，不依赖进程内消息。
- Agent 不可用、超时或输出无效时，将运行记为稳定失败或待重试，不写入伪造的智能事实；确定性链路继续。
- MinIO 写入失败时不得把缺少原始证据的内容标为已完成证据链。
- Vault 写入失败只延迟人类可读投影，不回滚已提交业务事实。
- WebSocket 中断后前端通过最后已见身份补拉 PostgreSQL 中的通知事实。

## 验收边界

| 验收 ID | 边界 |
|---|---|
| AC-001-001 | 授权用户可完成 Monitor→扫描→研判→事件→通知→日报→知识→检索闭环；每步状态可见，最终对象可追溯到 Monitor、运行和 Evidence；日报每个事实性 Claim 均引用允许的 Evidence ID，知识自动区域更新不覆盖人工区域，且全链路不绕过模块化单体契约。 |
| AC-001-002 | Viewer、Analyst、Editor、Admin 的读、写、审核和管理权限由服务端按真实角色执行；Analyst 未迁移时明确拒绝，不由前端伪造。 |
| AC-001-003 | P0 代码、Compose、Schema 与依赖只允许根目录 `agent/` 的 Python 分析服务，不新增第二业务后端、Kafka、Temporal、其他业务微服务、Elasticsearch、Keycloak 或 migrations 目录；Agent 不持有业务存储/来源凭据且不暴露公网端口；代码/Schema/OpenAPI 的当前事实、Design/PRD/Plan 的目标状态与 Acceptance 的已验收状态相互一致。 |
| AC-001-004 | 核心页面在桌面与移动端均能区分正常、空、加载、错误和权限不足状态，并通过键盘操作、可见焦点、语义化标签、合理对比度与 `prefers-reduced-motion` 检查。 |
| AC-001-005 | 任一 API 变更的 Result、错误码、DTO、Swaggo、`docs/openapi/swagger.json`、生成客户端与契约测试一致，Schema 仍只有 `backend/db/schema.sql` 一套。 |
| AC-001-006 | 候选容量与性能基线报告记录数据量、并发、硬件、缓存、统计窗口、P50/P95/P99 和排除条件；同时在隔离环境完成 PostgreSQL、MinIO、Vault 与持久任务的联合恢复演练，报告真实 RPO/RTO，未达标数字不得改写为已承诺 SLA。 |
| AC-001-007 | 多来源扫描遇到单来源失败或 Python Agent 不可用时，已采集事实和确定性计算不丢失，运行显示部分成功/待分析，恢复后幂等补算；Agent 只返回建议，必须经 Go Application/Domain 校验及授权人工治理，不能直写业务事实、权限或最终状态。 |
| AC-001-008 | 已发布报告和 Vault 知识可按关键词、标签、实体、时间与状态检索权限内高亮结果；链路不调用向量、Embedding 或 RAG。自动知识区域和检索投影可从 PostgreSQL 重建，人工区域只能从受保护的 Vault、Knowledge Revision 或备份恢复，不得以“重建”为由清空。 |
| AC-001-009 | 对关键写入重复提交、重放同一幂等键、使用过期资源版本或并发冲突时，服务端在副作用前完成认证、授权和校验；重复请求不产生重复事实，过期写入返回稳定冲突，成功与拒绝均留下不可覆盖且不含秘密的审计事实。 |
| AC-001-010 | 对带并列排序值并伴随并发新增的列表连续翻页时，稳定排序元组与唯一 ID 游标保证已越过区间不回流、同一遍历无重复或漏项；无效、过期、越权游标和超限页大小返回稳定脱敏错误。 |

## 未决项

- OPEN-001-001：现有多 Provider、Embedding 和 pgvector 能力是兼容保留、分阶段替换还是仅停止新写，需以真实数据量和调用方清单评审。
- OPEN-001-002：Python Agent 的服务认证轮换、模型 Provider 凭据注入、网络出口和单机并发上限需在 003 与 005 中形成可验证契约。
- OPEN-001-003：首版用户成功指标、容量目标和可用性目标需要产品基线和部署测量后确定，不能直接沿用附件估算。
- OPEN-001-004：哪些现有但未注册的源码模块继续进入目标运行面，需按 002–004 的 PRD 逐项取舍。
