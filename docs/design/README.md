---
layer: Design
doc_no: "000"
audience: [PM, Dev, QA, Ops]
feature_area: AI热点事件监控平台
purpose: 管理 HotKey Server 权威设计文档及其状态
canonical_path: docs/design/README.md
status: review
version: v1.3
owner: HotKey Server Team
inputs:
  - docs/README.md
outputs:
  - 设计文档索引
triggers:
  - 新增、替换或归档设计
  - 设计状态或下游任务变化
downstream:
  - docs/prd/README.md
  - docs/plans/README.md
---

# 后端设计索引

本目录记录 AI 热点事件监控平台后端的长期设计决策。`AGENTS.md` 定义所有开发必须遵守的架构约束，本目录解释需求、模型、流程和技术取舍。

## 文档规则

- 每份文档只解决一个清晰的设计问题
- 所有文档必须包含完整 YAML frontmatter
- 文档成熟度只使用 draft、review、accepted、archived
- 实施进度、临时任务和排查记录不得写入本目录
- 架构设计变更必须同步更新 `AGENTS.md` 和相关设计；实施时再同步完整 `db/schema.sql`、数据库记录模型和 OpenAPI
- Design 只定义长期技术决策，不写执行文件清单、提交拆分或测试结果

## 当前权威文档

| 文档 | 说明 | 状态 |
|---|---|---|
| [`archive/001-AI热点事件监控平台需求分析.md`](archive/001-AI热点事件监控平台需求分析.md) | 本地热点监控、MinIO证据、Obsidian治理、报告与订阅需求基线 | accepted |
| [`archive/002-后端单体架构设计.md`](archive/002-后端单体架构设计.md) | 模块化单体、API/Worker角色、River、存储端口和统一CRUD边界 | accepted |
| [`archive/003-数据库与数据生命周期设计.md`](archive/003-数据库与数据生命周期设计.md) | 完整业务/运行表、Repository、约束、索引、保留和单一Schema事实源 | accepted |
| [`archive/004-Result响应与全局异常设计.md`](archive/004-Result响应与全局异常设计.md) | Result 契约、业务码和全局错误转换 | accepted |
| [`archive/005-数据来源查询规划与采集设计.md`](archive/005-数据来源查询规划与采集设计.md) | Connector、共享查询、调度、限流与合规 | accepted |
| [`archive/006-内容标准化去重与证据设计.md`](archive/006-内容标准化去重与证据设计.md) | 统一内容、三层去重、证据和删除同步 | accepted |
| [`archive/007-多语言匹配与相关性设计.md`](archive/007-多语言匹配与相关性设计.md) | 双语检索、混合评分、解释和模型版本 | accepted |
| [`archive/008-Obsidian知识库治理与报告交付设计.md`](archive/008-Obsidian知识库治理与报告交付设计.md) | 本地Vault、知识提案、冲突修订、日报周报、邮件和RSS/Atom | accepted |
| [`archive/009-事件发现聚类与生命周期设计.md`](archive/009-事件发现聚类与生命周期设计.md) | 候选召回、跨语言聚类、事件键、生命周期、合并拆分和人工锁 | accepted |
| [`archive/010-热度趋势与排序设计.md`](archive/010-热度趋势与排序设计.md) | 跨来源归一化、事件热度、趋势、Monitor排序、防刷和重算 | accepted |
| [`archive/011-AI任务证据与模型运行设计.md`](archive/011-AI任务证据与模型运行设计.md) | AI任务目录、JSON Schema、证据引用、幂等复用、预算和降级 | accepted |
| [`archive/012-监控调度与River流水线设计.md`](archive/012-监控调度与River流水线设计.md) | Monitor调度、River任务图、事务入队、检查点、重试、取消和恢复 | accepted |
| [`archive/013-身份认证会话与权限设计.md`](archive/013-身份认证会话与权限设计.md) | 邮箱验证、密码、可撤销会话、刷新轮换和最小角色授权；服务端验收见 Acceptance 004 | accepted |
| [`archive/014-监控配置发布与预览设计.md`](archive/014-监控配置发布与预览设计.md) | 版本化 Monitor 发布、来源引用安全、纯配置预览、签名、权限与审计契约 | accepted |
| [`archive/015-LangChainGo多Provider与本地模型设计.md`](archive/015-LangChainGo多Provider与本地模型设计.md) | DeepSeek、Ollama、Qwen3 Embedding 与 LangChainGo 运行边界 | accepted |

001–015 的现有设计均已随对应实施与长期验收证据归档。已落地设计见 [`archive/`](archive/README.md)。
