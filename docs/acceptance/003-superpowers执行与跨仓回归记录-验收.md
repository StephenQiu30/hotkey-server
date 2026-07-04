---
layer: Acceptance
doc_no: 003
audience: Dev, QA, Ops
feature_area: Codex执行治理与跨仓同步
purpose: 记录本轮 superpowers 执行、server 优先同步、browser 回归和代码审核的实际验收结果
canonical_path: docs/acceptance/003-superpowers执行与跨仓回归记录-验收.md
status: draft
version: v1.1
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
3. 新增 H5 浏览器承载能力与首页模板：
   - `package.json`
   - `src/index.html`
   - `src/pages/index/index.tsx`
4. 修正 README 与 `CLAUDE.md` 以满足仓库治理契约：
   - `README.md`
   - `CLAUDE.md`

# 验收环境

1. 本地开发机
2. `hotkey-server`：Go + Docker Compose + 本地 `make dev`
3. `hotkey-web`：Next.js dev server (`http://localhost:3000`)
4. `hotkey-miniapp`：Taro 微信小程序构建 + H5 浏览器承载入口（`http://localhost:10086`）

# `hotkey-server` 验收结果

## A. 代码与脚本验证

- [x] `make swagger`
- [x] `make swagger-validate`
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

- [x] `docs/swagger.json` 已通过重新生成与校验
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
- [x] `npm run build:h5`
- [x] `npm run build:weapp`
- [x] `bash scripts/validate-repository.sh`

## B. 工程化检查

- [x] 请求封装可解析 JSON 错误体中的 `error` 与 `code`
- [x] 统一抛出 `HotKeyAPIError`
- [x] 契约测试覆盖统一错误体消费要求
- [x] H5 承载入口具备 `index.html` 模板，不再回落到目录索引页
- [x] H5 环境对 `Taro.login` / `requestSubscribeMessage` 提供兼容兜底，不再因平台 API 缺失导致页面崩溃

## C. 回归验证

- [x] 使用 `vercel:agent-browser` 完成页面回归
- [x] 已记录当前验证边界

页面/链路：

1. `http://localhost:10086/#/pages/index/index`
2. 平台登录 -> 热点榜单 -> 切换热点详情 -> 收藏关注 -> 订阅消息提醒入口

结果：

1. 根路径已返回真实应用页，不再出现目录索引页。
2. 点击“平台登录”后可进入已登录态，并展示热点榜单、快速理解、内容选题、通知列表。
3. 点击第二条热点后，快速理解区域会切换到对应热点详情。
4. 点击“收藏关注”后，按钮文本会切换为“已收藏关注”。
5. 点击“订阅消息提醒入口”后，H5 环境不再抛运行时错误，而是走非阻塞提示。

边界说明：

1. H5 登录属于浏览器承载环境下的演示兜底，不代表已打通真实微信登录换会话链路。
2. 订阅消息能力在 H5 环境仅做能力提示；真实订阅仍需在微信小程序端结合平台配置验收。

# 代码审核结论

## 已修复

1. `hotkey-server` recover 路径统一错误响应结构。
2. `hotkey-server` 公共健康检查匿名访问能力恢复。
3. `hotkey-web` / `hotkey-miniapp` 请求封装改为消费 server 统一错误体。
4. `hotkey-miniapp` H5 承载入口恢复，可完成 `agent-browser` 页面回归。

## 未修复但已明确记录

1. `hotkey-web` 当前登录链路仍是前端状态驱动，不是调用真实后端登录 API。
2. `hotkey-miniapp` H5 登录与订阅消息是浏览器承载兜底，不是真实微信端会话与订阅链路。
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
2. 待提交：H5 承载入口与跨端兼容修复

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
5. miniapp 的 `agent-browser` 主链路回归
6. `hotkey-server` 与 `hotkey-web` 已按项目规范完成相关提交

本轮未完全完成：

1. `web` 与 `miniapp` 的登录仍是演示链路，未接入真实后端认证闭环
2. 三仓尚未创建 PR，PR 状态仍为空

# 变更记录

| 版本 | 日期 | 作者 | 变更说明 |
| --- | --- | --- | --- |
| v1.1 | 2026-06-25 | Cursor | Gin/GORM 单轨合并后复跑计划 017：server 门禁全绿；记录 OpenAPI 契约缺口与 agent-browser 回归证据 |
| v1.0 | 2026-06-25 | Codex | 记录本轮 superpowers 执行、server/web/miniapp 同步与回归结果 |

---

# v1.1 补充验收（Gin/GORM 单轨合并后）

执行依据：`docs/plans/017-superpowers执行与跨仓同步计划.md`
执行顺序：`server -> web -> miniapp -> agent-browser 回归`

## Task 1：server 契约门禁 — 通过

```bash
cd hotkey-server
make swagger && make swagger-validate && bash scripts/validate-repository.sh
```

结果：全部通过（OpenAPI 3.1.0、schema components: 5、runtime smoke 全绿）。

## Task 2：server 工程化 — 通过

- `internal/platform/http/openapi_coverage_test.go` 已存在且 `go test ./internal/platform/http` 通过
- panic recover 统一错误体、`/healthz` 匿名访问已在先前提交中落地

## Task 3/4：web / miniapp 客户端重新生成 — 阻塞

对当前 `hotkey-server/docs/swagger.json`（静态 5-schema 精简版）执行 `npm run openapi:generate` 后：

| 仓 | `openapi:generate` | `typecheck` | 说明 |
|---|---|---|---|
| hotkey-web | 成功 | **失败** | 生成 `monitors.ts` 等 M1 路由客户端，但缺少 `HotspotRead` / `EmailLoginRequest` 等 UI 契约类型 |
| hotkey-miniapp | 成功 | **失败** | 同上 |

**判定：** 不允许进入“已同步完成”状态。下游仍依赖仓内已提交的富客户端（`hotspots.ts`、`analytics.ts` 等），与当前 server OpenAPI 事实源不一致。

**恢复基线（本轮验收用）：** 删除误生成的 `content.ts`/`monitors.ts`/`topics.ts`/`trends.ts`，保留 HEAD 已提交客户端后：

- `hotkey-web`：`npm run typecheck` + `npm run test`（19 tests）通过
- `hotkey-miniapp`：`npm run typecheck` + `npm run test`（9 tests）通过

**后续必须项：** 扩充 `internal/platform/http/openapi.go`（或等价契约源），使 `docs/swagger.json` 覆盖 web/miniapp 已提交客户端所需的 schema 与 path 参数，再重新执行跨仓 `openapi:generate`。

## Task 5：`agent-browser` 回归 — 部分通过

### Server

| 链路 | URL | 结果 |
|---|---|---|
| 健康检查 | `http://127.0.0.1:8080/healthz` | 200，`{"status":"ok"}` |

启动方式：`SMOKE_TEST=1 HTTP_ADDR=:8080 go run ./cmd/hotkey`

### Web（主链路 — 通过）

| 步骤 | 操作 | 结果 |
|---|---|---|
| 1 | `agent-browser open http://localhost:3000` | 标题「HotKey 创作者工作台」 |
| 2 | 点击「进入工作台」 | 进入工作台 |
| 3 | 验证模块 | 可见「热点榜单」「快速理解」「内容选题」「收藏关注」「趋势分析」「通知配置」 |
| 4 | 点击第二条热点 | 右侧详情切换为「搜索 API 聚合让小团队也能做热点雷达」 |

截图：`docs/acceptance/evidence/2026-06-25-web-workbench.png`

边界：登录仍为前端状态驱动 demo，未调用真实 `/api/v1/auth/login`。

### Miniapp H5（部分 — 页面可加载）

| 步骤 | URL | 结果 |
|---|---|---|
| 1 | `http://localhost:10086/#/pages/index/index` | 标题「HotKey」，页面文本可见「平台登录」「订阅消息提醒入口」 |
| 2 | 交互探测 | `agent-browser` 仅暴露单一可点击节点，未能完成登录后榜单切换（需小程序端或更细粒度选择器） |

截图：`docs/acceptance/evidence/2026-06-25-miniapp-h5-index.png`

## v1.1 结论

| 项 | 状态 |
|---|---|
| server 工程化门禁 | ✅ 通过 |
| web/miniapp 自动化测试（保留已提交客户端） | ✅ 通过 |
| 跨仓 OpenAPI 重新生成同步 | ❌ 阻塞（契约过薄） |
| web agent-browser 主链路 | ✅ 通过 |
| miniapp agent-browser 完整主链路 | ⚠️ 仅页面加载证据 |
| 真实后端登录 E2E | ❌ 未覆盖 |
