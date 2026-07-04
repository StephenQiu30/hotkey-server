---
layer: Design
doc_no: 008
audience: Dev, QA, Ops
feature_area: Codex执行治理与跨仓同步
purpose: 定义 HotKey 在 server 优先前提下使用 superpowers 执行任务、稳定 OpenAPI 契约并同步 web 与 miniapp 的统一设计
canonical_path: docs/design/008-superpowers执行与跨仓同步设计.md
status: draft
version: v1.0
owner: Codex
inputs:
  - ../README.md
  - ../AGENTS.md
  - docs/cross-repo-client-generation.md
  - ../hotkey-web/AGENTS.md
  - ../hotkey-miniapp/AGENTS.md
outputs:
  - 跨仓执行顺序与职责边界
  - server 到 web/miniapp 的同步规则
  - 验收标准与执行计划输入
triggers:
  - 新任务需要按 superpowers 规范执行
  - server 契约变更需要同步 web 与 miniapp
  - 需要统一工程化验收与浏览器回归门禁
downstream:
  - docs/acceptance/002-superpowers执行与跨仓验收标准-验收.md
  - docs/plans/017-superpowers执行与跨仓同步计划.md
---

# 背景

HotKey 当前由 `hotkey-server`、`hotkey-web`、`hotkey-miniapp` 三个独立仓组成，但协作顺序和验收标准尚未冻结为统一的长期规则。

现状中的主要风险是：

1. 任务执行可能直接从端侧开始，绕开 `hotkey-server` 这一契约事实源。
2. server 变更后，下游客户端生成、端侧错误处理和状态适配缺少固定同步节奏。
3. 交付时容易只给出测试通过结论，而缺少工程化检查、浏览器主链路回归和代码审核证据。

因此，需要一份正式设计，冻结 HotKey 在使用 `@superpowers` 执行任务时的唯一跨仓主线。

# 目标

1. 明确 HotKey 的默认执行顺序为 `server -> web -> miniapp -> 回归`。
2. 明确 `hotkey-server` 是唯一的接口契约事实源和跨仓执行入口。
3. 明确 server 完成后驱动 web 与 miniapp 同步的标准动作。
4. 明确工程化验收、`vercel:agent-browser` 回归和代码审核在交付中的必选地位。

# 非目标

1. 本文档不直接实现具体业务功能。
2. 本文档不替代各子仓的业务 PRD、模块设计或接口细节设计。
3. 本文档不改变 `hotkey-web` 与 `hotkey-miniapp` 作为端侧适配层的职责定位。

# 核心决策

## 决策 1：`hotkey-server` 是唯一上游执行入口

任何涉及以下内容的任务，都必须先在 `hotkey-server` 完成设计、计划、实现和验证，再进入下游仓同步：

1. API 路由与参数结构变更。
2. 响应字段、错误结构、状态码或鉴权方式变更。
3. 业务语义变化，例如监控、主题、提醒、摘要、收藏等核心能力行为变化。
4. OpenAPI 契约输出变化。

原因是：

1. 根目录 `AGENTS.md` 已冻结 `hotkey-server` 为 OpenAPI 契约事实源。
2. `hotkey-web` 与 `hotkey-miniapp` 都明确要求通过 `@umijs/openapi` 消费 server 契约。
3. 只有先稳定 server 契约，下游端侧的全局状态、错误处理和页面适配才有可靠事实源。

## 决策 2：执行流程固定为 `brainstorming -> writing-plans -> executing-plans/subagent-driven-development -> verification-before-completion`

HotKey 任务的 Codex 执行主线冻结如下：

1. `superpowers:brainstorming`
   - 收集事实、明确边界、提出方案、形成设计。
2. `superpowers:writing-plans`
   - 将设计拆为可执行任务，明确文件、接口、测试和提交粒度。
3. `superpowers:executing-plans` 或 `superpowers:subagent-driven-development`
   - 逐任务实现，保持测试先行和检查点节奏。
4. `superpowers:verification-before-completion`
   - 对照验收门禁做完成前核查，不允许只凭“测试绿了”宣布结束。

任何跳过上述链路的实现，都不视为符合本项目的标准执行方式。

## 决策 3：server 完成的定义必须同时覆盖代码、契约、工程和审核

`hotkey-server` 的任务只有在以下条件同时成立时，才允许进入下游同步阶段：

1. 代码验证完成
   - 相关单元测试、集成测试、构建和仓内验证脚本通过。
2. 契约验证完成
   - `docs/swagger.json` 已更新，且变更与实际实现一致。
3. 工程化检查完成
   - 全局异常处理、统一错误响应、关键边界层次和可观测性要求满足。
4. 代码审核完成
   - 已对修改代码进行 review，明确记录高可用性风险和修复结论。
5. 验收记录完成
   - 有正式验收文档记录本次变更范围、验证步骤、结论、残余风险和下游同步要求。

如果以上任一项缺失，server 任务仍处于“未完成”状态。

# 跨仓职责边界

## `hotkey-server`

负责：

1. API、任务调度、数据存储、采集、主题、趋势、提醒、通知、摘要等后端能力。
2. 统一错误结构和异常出口。
3. OpenAPI 契约导出与验证。
4. 为 web 与 miniapp 提供稳定、可生成的客户端事实源。

不负责：

1. Next.js 或 Taro 页面实现。
2. 页面级全局状态树设计。
3. 端侧用户交互细节与视图降级呈现。

## `hotkey-web`

负责：

1. 基于 server OpenAPI 生成并消费客户端。
2. 统一 Web 端请求封装、错误展示、登录失效处理和页面降级。
3. 将 server 新能力或契约变化同步到创作者工作台。

## `hotkey-miniapp`

负责：

1. 基于 server OpenAPI 生成并消费客户端。
2. 统一小程序端登录适配、错误展示、页面状态和端侧限制处理。
3. 将 server 新能力或契约变化同步到小程序轻量链路。

# 同步机制

server 完成后，下游同步必须遵循固定顺序。

## 第 1 步：冻结 server 契约

在 `hotkey-server` 中完成：

1. 代码实现与测试。
2. `docs/swagger.json` 导出与校验。
3. 变更影响说明：
   - 新增或修改了哪些接口。
   - 哪些 DTO、字段、状态码、错误结构发生变化。
   - 哪些旧端侧代码可能失效。

## 第 2 步：同步 `hotkey-web`

在 `hotkey-web` 中至少完成：

1. 基于最新 `docs/swagger.json` 重新生成客户端。
2. 更新受影响的页面、hooks、服务封装和全局状态接入点。
3. 覆盖加载态、空态、错误态和 AI 失败降级态。
4. 运行本仓测试、类型检查和构建。
5. 使用 `vercel:agent-browser` 对受影响 Web 主链路逐项回归。

## 第 3 步：同步 `hotkey-miniapp`

在 `hotkey-miniapp` 中至少完成：

1. 基于最新 `docs/swagger.json` 重新生成客户端。
2. 更新受影响的页面、状态接入点和端侧登录适配。
3. 覆盖加载态、空态、错误态和登录失效态。
4. 运行本仓测试和构建。
5. 若当前存在可浏览器承载的调试或 H5 入口，使用 `vercel:agent-browser` 执行等价主链路验证；若不存在，则在验收文档中显式记录小程序专属验证路径与结果。

## 第 4 步：跨仓回归

在 server、web、miniapp 各自完成后，执行跨仓回归，确保：

1. 新 OpenAPI 契约与两个端侧生成客户端一致。
2. 主链路在 Web 与小程序端都可用。
3. 已知兼容性差异被显式记录，而不是隐含留给后续排查。

# 工程化规范

本设计将以下内容冻结为“交付前必须检查”的工程化规范。

## server 工程化门禁

1. 全局异常处理
   - HTTP 请求必须有统一 recover / error mapping 机制。
   - 业务错误和基础设施错误要有稳定映射规则。
2. 统一错误结构
   - 对外错误响应必须维持一致格式，不允许各 handler 自行拼装。
3. 边界清晰
   - handler、service、repository、worker 的职责不能混写。
4. 契约一致
   - 实现、测试和 OpenAPI 输出必须对应同一事实。
5. 关键失败路径可验证
   - 至少对典型失败场景有测试或验收步骤证明，而不是只验证成功路径。

## web / miniapp 工程化门禁

1. 全局状态边界清晰
   - 用户态、筛选态、会话态、异步加载态等必须有统一接入点，避免页面散落维护。
2. 全局异常处理完整
   - 接口失败、鉴权失效、关键渲染失败要有统一降级和用户可见反馈。
3. 页面状态完整
   - 加载态、空态、错误态、失效态必须覆盖。
4. 契约消费一致
   - 不允许绕开生成客户端手写漂移类型。
5. 端侧降级可回归
   - AI 摘要失败、网络异常、空数据等路径要能通过手工或自动验收复现。

# `vercel:agent-browser` 的定位

`vercel:agent-browser` 是 HotKey 端侧最终回归器，不替代单元测试、集成测试或构建检查。

它的职责是：

1. 验证真实页面主链路可用性。
2. 验证端侧状态变化是否符合预期。
3. 验证服务端契约变更没有破坏页面交互。

它不负责替代：

1. server 的 Go 测试和仓内验证脚本。
2. 类型检查和构建验证。
3. 代码审核。

因此，正确使用顺序是：

1. 先完成 server 验证。
2. 再完成 web / miniapp 的代码级验证。
3. 最后用 `vercel:agent-browser` 做页面逐项回归并记录证据。

# 代码审核规则

HotKey 的修改代码在交付前必须有一次显式审核，审核重点不是风格，而是可用性和高可用性风险。

最低检查项包括：

1. 是否存在未被全局异常处理兜住的错误出口。
2. 是否存在状态来源不清、重复状态或跨层泄漏。
3. 是否存在只覆盖 happy path、未覆盖失败路径的实现。
4. 是否存在接口契约、端侧类型和真实响应不一致的风险。
5. 是否存在可能导致回归破坏的改动点未被验证。

审核结论必须被写入验收记录或交付说明，而不是只停留在聊天过程。

# 文档与证据策略

本设计要求任何一次跨仓任务结束时，都至少留下两类长期证据：

1. 设计 / 计划证据
   - 说明为什么这样改、影响哪些仓、如何分阶段执行。
2. 验收 / 回归证据
   - 说明怎么验证、实际结果如何、有哪些残余风险。

临时对话中的说明不视为长期证据。

# 验收门禁

本文档设计完成的门禁如下：

1. 已明确 `server -> web -> miniapp -> 回归` 为唯一默认执行顺序。
2. 已明确 `hotkey-server` 为唯一 OpenAPI 契约事实源。
3. 已明确 server 完成的判定标准与下游同步触发条件。
4. 已明确工程化规范、`vercel:agent-browser` 回归与代码审核的必选地位。
5. 已指定下游验收标准文档与执行计划文档作为后续落地入口。

# 风险与边界

1. 若 server 契约未真正冻结就提前同步端侧，会重复返工并削弱验收有效性。
2. 若 `vercel:agent-browser` 被当作唯一验证手段，会掩盖未覆盖的单元测试和构建问题。
3. 若代码审核只做形式检查，不记录高可用性风险，后续回归成本会继续上升。

# 待确认问题

1. miniapp 在当前阶段的浏览器可承载验证入口是否需要单独补一份操作手册。
2. 后续是否需要把常见 `agent-browser` 回归命令进一步沉淀到 `operations/` 文档。

# 变更记录

| 版本 | 日期 | 作者 | 变更说明 |
| --- | --- | --- | --- |
| v1.0 | 2026-06-25 | Codex | 首版定义 superpowers 执行顺序、server 优先跨仓同步规则和工程化验收设计 |
