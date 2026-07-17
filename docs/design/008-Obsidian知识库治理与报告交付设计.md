---
layer: Design
doc_no: "008"
audience: [PM, Dev, QA, Ops]
feature_area: Obsidian知识库治理与报告交付
purpose: 定义本地Vault知识投影、AI变更审核、日报周报、邮件和RSS/Atom交付契约
canonical_path: docs/design/008-Obsidian知识库治理与报告交付设计.md
status: review
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/design/archive/001-AI热点事件监控平台需求分析.md
  - docs/design/archive/002-后端单体架构设计.md
  - docs/design/archive/003-数据库与数据生命周期设计.md
  - docs/design/009-事件发现聚类与生命周期设计.md
  - docs/design/010-热度趋势与排序设计.md
  - docs/design/archive/011-AI任务证据与模型运行设计.md
  - docs/design/012-监控调度与River流水线设计.md
outputs:
  - Vault目录和Markdown契约
  - 知识变更提案、冲突和修订流程
  - 日报周报、邮件和RSS/Atom交付规则
triggers:
  - 修改Vault目录、Frontmatter、知识审核或报告交付规则
  - 新增知识文档或订阅渠道
downstream:
  - Knowledge模块实施计划
  - Report与Delivery模块实施计划
  - Obsidian和订阅端到端验收
---

# Obsidian 知识库治理与报告交付设计

## 1. 目标

1. 把数据库中的事件、长期主题和报告稳定投影为本地Markdown。
2. 自动生成事件笔记、日报和周报，不要求用户执行Git操作。
3. AI对长期知识只提出差异，由用户在Web中审核。
4. 保护人工编辑，防止后台任务静默覆盖。
5. 让报告可以通过邮件和RSS/Atom稳定交付并追踪失败。

## 2. 非目标

- 不把Obsidian文件作为并发业务数据库。
- 不在运行流程中执行Git commit、branch、push或pull request。
- 不实现Obsidian插件。
- 不支持任意模板执行代码。
- 不把MinIO对象公开为无鉴权永久链接。
- 不在V1提供Slack、Teams、Webhook等额外投递渠道。

## 3. 事实源与写入权限

| 数据 | 事实源 | 写入者 |
|---|---|---|
| 事件、主题、实体、主张、报告和审核状态 | PostgreSQL | Application用例 |
| 原始响应、正文、截图、附件和修订快照 | MinIO | ObjectStore适配器 |
| 事件、主题和报告Markdown | 本地Vault | Knowledge Worker和用户 |

数据库记录决定业务状态；Vault中的人工文字是受保护内容。系统扫描到人工编辑后创建新修订并更新哈希，而不是把文件恢复成上一次生成结果。

## 4. Vault配置和目录

Vault根目录由本地配置 `knowledge.vault_path` 提供，必须：

1. 解析为存在或可创建的绝对目录。
2. 禁止路径穿越和符号链接逃逸到允许根目录之外。
3. 启动时验证读写权限和原子重命名能力。
4. 禁止把凭据、数据库文件和MinIO数据目录放入托管子目录。

系统只管理：

```text
HotKey/
  Inbox/
  Events/
  Topics/
  Reports/
    Daily/
    Weekly/
  Archive/
```

目录职责：

| 目录 | 内容 |
|---|---|
| `Inbox` | 等待人工处理的冲突说明和可选审核入口笔记 |
| `Events` | 自动维护的热点事件笔记 |
| `Topics` | 经审核维护的长期主题笔记 |
| `Reports/Daily` | 已发布日报 |
| `Reports/Weekly` | 已发布周报 |
| `Archive` | 归档知识文档，不作为删除回收站 |

## 5. 路径和文件名

文件路径由文档类型、日期和稳定业务键生成，不直接信任模型输出或用户标题。

```text
HotKey/Events/YYYY/MM/YYYY-MM-DD-<event_key>.md
HotKey/Topics/<topic_key>.md
HotKey/Reports/Daily/YYYY/MM/YYYY-MM-DD-<scope>.md
HotKey/Reports/Weekly/YYYY/YYYY-Www-<scope>.md
HotKey/Archive/<document_type>/<original-relative-path>.md
```

规则：

- `event_key`、`topic_key`和scope只能包含小写字母、数字和连字符。
- 标题只写入Frontmatter和正文，不用于稳定路径。
- 数据库中的 `vault_path` 保存相对路径，禁止保存Vault外绝对路径。
- Windows保留名、控制字符、尾随点和尾随空格必须拒绝。
- 大小写折叠后路径必须唯一，兼容Windows文件系统。

## 6. 通用Frontmatter

所有系统管理文档包含：

```yaml
---
hotkey_id: event:12345
hotkey_document_id: 67890
hotkey_type: event
hotkey_revision: 7
hotkey_status: active
title: 示例事件
aliases: []
tags: [hotkey, event]
created_at: 2026-07-15T02:00:00Z
updated_at: 2026-07-15T08:00:00Z
generated_at: 2026-07-15T08:00:00Z
evidence_count: 12
content_hash: <sha256>
---
```

约束：

- `hotkey_id`和`hotkey_document_id`写入后不可改变。
- `hotkey_revision`必须与数据库当前修订一致。
- `content_hash`计算时排除自身字段，避免循环变化。
- 时间使用UTC ISO 8601。
- 标签和别名必须去重、排序并限制数量。
- 未识别的用户Frontmatter字段默认保留；系统保留字段由系统管理。

## 7. 事件笔记

事件笔记由系统自动创建和更新，结构固定为：

```markdown
# 事件标题

## 摘要

## 当前状态

## 时间线

## 关键实体

## 事实主张与争议

## 来源证据

## 相关主题

## 人工备注
```

自动更新区域使用稳定注释边界：

```markdown
<!-- hotkey:auto:summary:start -->
系统生成内容
<!-- hotkey:auto:summary:end -->
```

规则：

1. 事件标题、状态、时间线、实体、主张和证据区域可自动更新。
2. `人工备注`和边界之外的用户内容不得被自动替换。
3. 人工修改自动区域后，扫描任务把文档标记为人工变更并进入冲突判断。
4. 已合并事件保留原笔记，并链接到目标事件。
5. 来源被删除后，证据区域重建并显示证据状态变化。

## 8. 长期主题笔记

主题笔记结构：

```markdown
# 主题标题

## 主题说明

## 关键实体

## 关系

## 里程碑事件

## 争议与未决问题

## 人工结论
```

长期主题具有更高人工治理等级：

- 创建、合并、归档以及实体/主题关系修改必须经过提案审核。
- AI可以生成摘要和候选关系，但不能直接应用。
- 人工确认的关系标记 `confirmed=true`，后续模型不得降级或删除。
- 归档移动到 `Archive/topics`，数据库状态保留。

## 9. 知识变更提案

### 9.1 提案内容

提案必须保存：

- 目标文档和变更类型
- 基础修订号和基础哈希
- 提议的Frontmatter和正文
- 结构化Diff摘要
- 变更原因、模型和证据
- 请求人、审核人和状态时间

### 9.2 状态

```text
pending -> approved -> applied
        \-> rejected
        \-> conflict
approved -> failed
```

`approved`只表示业务审核通过；写文件和创建修订成功后才能进入`applied`。

### 9.3 审核操作

Web只开放：

- 查看统一Diff和证据
- 批准
- 拒绝并填写原因
- 在冲突中选择保留用户内容、应用提案或人工合并后的内容
- 重新基于当前修订生成提案

禁止通过公共API直接修改`base_hash`、审核人或`applied`状态。

## 10. 人工编辑检测

Vault扫描流程：

1. 枚举系统托管路径，拒绝跟随逃逸符号链接。
2. 解析Frontmatter中的文档ID和修订。
3. 计算规范化内容哈希。
4. 与`knowledge_documents.content_hash`比较。
5. 哈希变化时创建`source=user`的KnowledgeRevision。
6. 更新数据库文档哈希和修订号。
7. 将所有基于旧哈希的待处理提案标记为`conflict`。

扫描不得根据文件名猜测文档ID。缺少有效文档ID的文件进入Inbox提示，不自动绑定。

## 11. 原子写入和锁

单文档写入：

1. 获取规范化相对路径锁。
2. 重新读取文件并计算哈希。
3. 校验数据库版本、修订和基础哈希。
4. 在同一目录写入随机临时文件。
5. Flush并关闭文件；支持时同步目录元数据。
6. 使用原子Rename替换目标文件。
7. 重新读取并校验最终哈希。
8. 更新数据库修订、哈希和写入时间。
9. 释放路径锁。

临时文件命名包含文档ID和随机值，不使用可预测固定名称。进程启动时清理超过24小时且没有运行任务引用的临时文件。

路径锁至少在进程内有效；多Worker模式下还必须使用PostgreSQL advisory lock或等价数据库租约保护同一文档ID。

## 12. 修订和恢复

每次成功变化创建KnowledgeRevision，快照正文写入MinIO，数据库保存对象键和哈希。

修订来源：

- `system`：事件和报告自动更新
- `ai`：审核后应用的提案
- `user`：扫描检测到人工编辑
- `restore`：用户恢复历史版本

恢复历史版本不是删除新修订，而是以旧快照内容创建新的`restore`修订。修订序号严格递增。

## 13. 日报和周报

### 13.1 报告范围

- `global`：所有活动监控主题的综合报告
- `monitor`：单个监控主题报告

日报按订阅时区的自然日生成，周报按ISO周生成。数据库时间范围转换为UTC后持久化。

### 13.2 报告结构

```markdown
# 报告标题

## 今日/本周摘要

## 重点事件

## 上升最快

## 持续关注

## 争议与信息缺口

## 新增实体和关系

## 数据来源与覆盖说明
```

每个事件条目包含：

- 发布时标题和摘要快照
- 入选原因、排名、热度和趋势
- 来源数量及主要证据链接
- 单一来源、多源印证或争议状态
- Obsidian事件笔记链接

### 13.3 生成和发布

1. 按范围和周期查询候选事件。
2. 使用确定性规则计算基础排名和章节。
3. 冻结ReportItem快照。
4. LLM从结构化条目生成摘要，不自行添加无证据事实。
5. 校验Markdown结构、引用和非空章节。
6. 标记报告为published。
7. 创建KnowledgeDocument、邮件投递和Vault任务。

已发布报告不可直接覆盖。同一周期重新生成时增加`version_no`，历史文件保留或归档。

## 14. 邮件交付

### 14.1 内容

- 主题包含报告类型、日期和可选监控名称。
- 同时发送HTML和纯文本版本。
- HTML模板变量必须转义。
- 邮件提供报告摘要、重点事件和本地/配置公开地址；不嵌入MinIO永久凭据。

### 14.2 幂等和重试

`report_id + subscription_id`唯一确定一个ReportDelivery。成功后不得再次自动发送。

默认最多尝试5次：

```text
1分钟 -> 5分钟 -> 30分钟 -> 120分钟 -> 次日补偿
```

分类：

- 网络超时、临时SMTP错误：重试。
- 认证失败：停止渠道任务并提示管理员。
- 永久拒收或地址错误：停止自动重试并标记订阅需处理。
- 发送结果不确定：使用同一幂等记录重试，并保存提供商消息ID。

日志只记录投递ID、脱敏收件人、响应类别和耗时。

## 15. RSS/Atom

RSS/Atom不是推送任务，不创建虚假投递成功记录。

Feed规则：

- 只包含已发布报告。
- Item GUID由报告ID和版本生成，永久稳定。
- `published`使用报告发布时间。
- 支持`ETag`和`Last-Modified`条件请求。
- 分页或最多返回最近50份报告。
- 私有Feed令牌至少256位随机数，数据库只保存SHA-256。
- 令牌使用恒定时间比较；轮换后旧令牌立即失效。
- 响应和日志不泄露令牌。

Atom和RSS内容来源相同，只在序列化格式上不同。

## 16. 跨存储对账

定时对账检查：

- 数据库文档记录存在但Vault文件缺失
- Vault文件存在但数据库文档记录缺失
- 文件哈希与数据库不一致
- 修订快照对象缺失
- MinIO存在无数据库引用且超过宽限期的对象
- 报告已发布但缺少知识文档或邮件投递实例

对账只自动修复确定性问题。人工内容冲突、未知文件绑定和缺失证据进入Operations待处理列表。

## 17. 安全边界

- Vault路径和文件名必须通过规范化和根目录检查。
- Markdown中的外部URL保留原地址，但Web渲染必须过滤危险协议。
- 提案Diff和Markdown预览不得执行HTML脚本。
- MinIO对象通过服务端受权读取或短期签名URL访问。
- RSS私有令牌、SMTP密码和对象存储密钥不得进入Frontmatter。
- 报告引用原文只保留必要短片段，遵守来源协议和版权边界。

## 18. 可观测性

指标：

- Vault扫描、写入、冲突和缺失文件数量
- 文档写入耗时和失败类型
- 提案pending、conflict、approved和applied数量
- 报告生成耗时、空报告和校验失败数量
- 邮件发送成功率、重试次数和永久失败数量
- RSS请求、304命中率和令牌失败数量

日志关联`document_id`、`proposal_id`、`report_id`、`delivery_id`、`job_id`和`trace_id`，不记录正文和秘密。

## 19. 测试

### 19.1 单元测试

- 路径规范化和逃逸拒绝
- Frontmatter解析、保留和稳定排序
- 内容哈希的确定性
- 自动区域合并和人工区域保护
- 报告周期、时区和文件名
- RSS GUID、ETag和令牌验证
- 邮件错误分类和重试计划

### 19.2 集成测试

- 临时Vault中的原子创建、更新和归档
- 人工编辑后提案进入conflict
- 写文件成功但数据库更新失败后的对账恢复
- MinIO修订快照写入和恢复
- 报告发布同时创建文档和唯一投递
- Fake SMTP验证HTML/纯文本和重试
- RSS/Atom条件请求返回304

### 19.3 故障测试

- 进程在临时文件写入后退出
- 同一文档两个Worker并发写入
- MinIO不可用、磁盘只读或空间不足
- Vault文件被删除、重命名或Frontmatter损坏
- SMTP超时且发送结果未知

## 20. 验收门禁

- 事件笔记、日报和周报写入固定目录并具有稳定Frontmatter。
- 自动更新不覆盖人工备注。
- 人工修改后，旧提案不能直接应用。
- 每次修改都有可恢复修订和MinIO快照。
- 事件合并和主题归档保留历史链接。
- 已发布报告条目不因事件后续变化而漂移。
- 同一报告和订阅不会重复成功投递。
- 邮件失败可追踪，RSS/Atom支持ETag、Last-Modified和令牌轮换。
- 知识库流程在没有Git的情况下完整运行。
- 对账可以识别数据库、MinIO和Vault之间的不一致。

## 21. 待确认问题

本文范围内没有阻塞实施的待确认问题。邮件服务商和Feed对外Base URL属于本地运行配置。

## 22. 关联文档

- [需求分析](archive/001-AI热点事件监控平台需求分析.md)
- [后端单体架构设计](archive/002-后端单体架构设计.md)
- [数据库与数据生命周期设计](archive/003-数据库与数据生命周期设计.md)
- [内容标准化去重与证据设计](archive/006-内容标准化去重与证据设计.md)
- [事件发现、聚类与生命周期设计](009-事件发现聚类与生命周期设计.md)
- [热度、趋势与排序设计](010-热度趋势与排序设计.md)
- [AI任务、证据与模型运行设计](archive/011-AI任务证据与模型运行设计.md)
- [监控调度与 River 流水线设计](012-监控调度与River流水线设计.md)

## 23. 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-15 | 建立本地Vault、审核式长期知识治理、日报周报、邮件和RSS/Atom交付契约 |
