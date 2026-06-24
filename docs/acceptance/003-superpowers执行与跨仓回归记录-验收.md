---
layer: Acceptance
doc_no: 003
audience: Dev, QA, Ops
feature_area: Codex执行治理与跨仓同步
purpose: 记录本轮 superpowers 执行、server 优先同步、browser 回归和代码审核的实际验收结果
canonical_path: docs/acceptance/003-superpowers执行与跨仓回归记录-验收.md
status: draft
version: v1.0
owner: Codex
inputs:
  - docs/design/008-superpowers执行与跨仓同步设计.md
  - docs/acceptance/002-superpowers执行与跨仓验收标准-验收.md
  - docs/plans/017-superpowers执行与跨仓同步计划.md
  - hotkey-server / hotkey-web / hotkey-miniapp 当前提交与验证结果
outputs:
  - 本轮 server -> web -> miniapp 执行证据
  - browser 回归记录
  - 残余风险与未验证边界
triggers:
  - 需要证明本轮跨仓同步已按验收标准执行
downstream: []
---

# 背景

本记录对应一次完整的 `server -> web -> miniapp -> 回归` 执行，目标是把 HotKey 当前的 superpowers 执行方式、工程化门禁、端侧统一错误处理和 browser 回归证据落成正式验收材料。

# 变更范围

## `hotkey-server`

1. 新增执行与验收标准文档：
   - `docs/design/008-superpowers执行与跨仓同步设计.md`
   - `docs/acceptance/002-superpowers执行与跨仓验收标准-验收.md`
   - `docs/plans/017-superpowers执行与跨仓同步计划.md`
2. 更新跨仓客户端生成与验收流程：
   - `docs/cross-repo-client-generation.md`
3. 统一 panic recover 错误结构：
   - `internal/platform/http/errors.go`
   - `internal/platform/http/middleware.go`
   - `tests/unit/platform/http/router_test.go`
4. 修复 `/healthz` 被鉴权中间件错误拦截的问题：
   - `internal/platform/http/middleware.go`
   - `tests/unit/platform/http/router_test.go`

## `hotkey-web`

1. 请求封装改为消费 server 统一 `{ error, code }` 错误体：
   - `src/lib/request.ts`
2. 增加契约测试，防止回退为纯状态码错误：
   - `tests/test_web_openapi_contract.py`
3. 修正 README 以满足仓库治理契约：
   - `README.md`

## `hotkey-miniapp`

1. 请求封装改为消费 server 统一 `{ error, code }` 错误体：
   - `src/utils/request.ts`
2. 增加契约测试，防止回退为纯状态码错误：
   - `tests/test_miniapp_taro_contract.py`
3. 修正 README 与 `CLAUDE.md` 以满足仓库治理契约：
   - `README.md`
   - `CLAUDE.md`

# 验收环境

1. 本地开发机
2. `hotkey-server`：Go + Docker Compose + 本地 `make dev`
3. `hotkey-web`：Next.js dev server (`http://localhost:3000`)
4. `hotkey-miniapp`：Taro 微信小程序构建

# `hotkey-server` 验收结果

## A. 代码与脚本验证

- [x] `make openapi`
- [x] `make openapi-validate`
- [x] `bash scripts/validate-repository.sh`
- [x] `go test ./tests/unit/platform/http -run 'TestHealthEndpoint|TestRecoverMiddlewareReturnsUnifiedErrorBody' -v`

结果：

1. OpenAPI 3.1.0 校验通过。
2. Go tests、build、docker compose config、runtime smoke 全绿。
3. `/healthz` 运行时复核返回 `200 OK`。

## B. 工程化规范检查

- [x] panic recover 返回统一错误结构
- [x] `/healthz` 不再被鉴权中间件错误拦截
- [x] HTTP 层失败路径具备测试证据

审核结论：

1. 已修复的高可用性问题：
   - recover 路径原先手写 JSON，未统一输出错误 `code`
   - 公共健康检查路径原先被鉴权拦截，导致运行态可观测性探针不可用
2. 当前未发现新的 handler / service / repository 混写问题

## C. 契约事实源检查

- [x] `docs/openapi.json` 已通过重新生成与校验
- [x] 已更新 `docs/cross-repo-client-generation.md` 为真实当前命令
- [x] 已明确下游同步点为 `web` / `miniapp` 请求错误处理统一消费 `{ error, code }`

# `hotkey-web` 验收结果

## A. 自动化验证

- [x] `npm run test`
- [x] `npm run typecheck`
- [x] `npm run build`
- [x] `bash scripts/validate-repository.sh`

## B. 工程化检查

- [x] 请求封装可解析 JSON 错误体中的 `error` 与 `code`
- [x] 统一抛出 `HotKeyAPIError`
- [x] 契约测试覆盖统一错误体消费要求

## C. `vercel:agent-browser` 回归

页面/链路：

1. `http://localhost:3000/`
2. 登录页 -> 点击“进入工作台” -> 工作台主界面

前置条件：

1. `hotkey-server` 本地 dev 运行于 `:8080`
2. `hotkey-web` 本地 dev 运行于 `:3000`
3. 当前工作台登录链路为前端状态驱动 demo 流程

结果：

1. 首页成功加载。
2. 登录按钮可进入工作台页。
3. 工作台页可见热点榜单、快速理解、内容选题、通知配置等关键模块。
4. 点击热点卡片可切换右侧详情展示。

注意：

1. 当前 `web` 的登录并未真正调用后端登录 API，而是组件内状态切换。
2. 因此前端页面回归已完成，但“真实后端登录端到端链路”不在本次通过范围内。

# `hotkey-miniapp` 验收结果

## A. 自动化验证

- [x] `npm run test`
- [x] `npm run typecheck`
- [x] `npm run build:weapp`
- [x] `bash scripts/validate-repository.sh`

## B. 工程化检查

- [x] 请求封装可解析 JSON 错误体中的 `error` 与 `code`
- [x] 统一抛出 `HotKeyAPIError`
- [x] 契约测试覆盖统一错误体消费要求

## C. 回归验证

- [ ] 使用 `vercel:agent-browser` 完成同等级页面回归
- [x] 已记录当前验证边界

边界说明：

1. 当前 `hotkey-miniapp` 只配置了微信小程序编译链路（`build:weapp` / `dev:weapp`）。
2. 仓内没有可直接承载到浏览器页面的 H5 调试入口。
3. 因此本轮只能完成构建、类型检查、契约测试和仓库治理校验，无法对 miniapp 做与 web 同等级的 `agent-browser` 页面回归。

# 代码审核结论

## 已修复

1. `hotkey-server` recover 路径统一错误响应结构。
2. `hotkey-server` 公共健康检查匿名访问能力恢复。
3. `hotkey-web` / `hotkey-miniapp` 请求封装改为消费 server 统一错误体。

## 未修复但已明确记录

1. `hotkey-web` 当前登录链路仍是前端状态驱动，不是调用真实后端登录 API。
2. `hotkey-miniapp` 当前没有浏览器可承载入口，无法完成 `agent-browser` 页面回归。
3. `hotkey-web` 仓仍存在与本次任务无关的现有工作区变更，未被本次提交带入。

# Git 提交状态

## `hotkey-server`

1. `fdaf813` `docs: 新增superpowers跨仓执行与验收标准`
2. `3ff4d10` `docs: 新增superpowers跨仓同步计划`
3. `8e06e6b` `docs: 更新跨仓客户端生成与验收流程`
4. `7b72d25` `impl: 统一panic恢复错误响应`
5. `33ec69c` `impl: 放开健康检查匿名访问`

## `hotkey-web`

1. `9e96033` `impl: 同步web统一错误响应处理`

## `hotkey-miniapp`

1. `b2ae027` `impl: 同步miniapp统一错误响应处理`

# PR 状态

1. `hotkey-server`：未创建 PR
2. `hotkey-web`：未创建 PR
3. `hotkey-miniapp`：未创建 PR

# 验收结论

本轮已完成：

1. server 优先执行与验收标准定义
2. server 工程化关键缺陷修复
3. web 与 miniapp 对 server 统一错误体的同步消费
4. web 的 `agent-browser` 主链路回归
5. 三仓按项目规范完成相关提交

本轮未完全完成：

1. miniapp 无法进行同等级 `agent-browser` 页面回归
2. 三仓尚未创建 PR，PR 状态仍为空

# 变更记录

| 版本 | 日期 | 作者 | 变更说明 |
| --- | --- | --- | --- |
| v1.0 | 2026-06-25 | Codex | 记录本轮 superpowers 执行、server/web/miniapp 同步与回归结果 |
