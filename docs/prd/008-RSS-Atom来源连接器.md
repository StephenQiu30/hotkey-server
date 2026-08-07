---
layer: PRD
scope: backend
doc_no: "008"
title: RSS-Atom来源连接器
status: implemented
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/prd/008-RSS-Atom来源连接器.md
design: docs/design/008-RSS-Atom来源连接器设计.md
plan: docs/plans/008-RSS-Atom来源连接器计划.md
---

# RSS-Atom来源连接器

## 用户问题

RSS/Atom HTTPS 连接器、分页、ETag/Last-Modified、SSRF 防护、标准化入口和来源配置界面已经实现。

## 产品目标

将拥有公开或授权 Feed 的站点作为首要稳定事实源，并提供可追溯、可降级且不会无限消耗资源的采集结果。

## 范围

- RSS/Atom 解析和增量采集
- 条件请求、分页和检查点
- 正文与元数据标准映射
- 来源健康和诊断

## 用户故事

- 作为普通用户，我能够看懂当前状态、结果来源和下一步动作。
- 作为编辑者，我能够在权限范围内完成配置或处理任务，并获得明确反馈。
- 作为管理员，我能够治理配置、失败、成本与审计，而不直接修改数据库事实。

## 功能要求

- FR-008-1：一个 SourceConnection 绑定一个不可变 HTTPS Feed 根端点；翻页只能同源并重复执行安全校验。
- FR-008-2：支持 RSS 2.0、RSS 1.0 与 Atom，保留 guid/id、canonical URL、作者、发布时间、摘要、正文和 enclosure 元数据；附件只保留安全元数据，不在本条下载。
- FR-008-3：使用 ETag、Last-Modified 和 checkpoint 增量拉取；单次页数、4 MiB Feed 响应、项目数和超时均有硬上限。
- FR-008-4：正文、摘要降级和仅元数据分别标记为 `full_body`、`summary_only`、`metadata_only`，并随 CapturedItem 持久化；关闭正文保存时降级为 `metadata_only`。
- FR-008-5：来源没有公开 Feed 时不抓取网页，改为等待授权 Feed 或由用户提供合法订阅地址。

## 非功能要求

- 所有写操作具备认证、授权、输入校验、幂等或乐观锁保护和审计。
- 列表支持稳定分页；第三方调用具备超时、配额、重试和降级。
- Web 覆盖桌面与移动端，并满足键盘操作、可见焦点和减弱动效。

## 非目标

不使用未授权抓取补齐来源能力，不为本条引入与仓库规范冲突的新基础设施。

## 验收标准

- AC-008-1：重复窗口不会重复创建 Content
- AC-008-2：恶意地址、私网解析和跨域重定向被拒绝
- AC-008-3：无正文 Feed 可降级为摘要且标明证据完整度
