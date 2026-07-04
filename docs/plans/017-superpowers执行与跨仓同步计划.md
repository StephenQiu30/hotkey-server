---
layer: Plan
doc_no: 017
audience: Dev, QA, Ops
feature_area: Codex执行治理与跨仓同步
purpose: 将 superpowers 执行方式、server 优先同步机制和跨仓验收门禁拆解为可执行任务
canonical_path: docs/plans/017-superpowers执行与跨仓同步计划.md
status: draft
version: v1.0
owner: Codex
inputs:
  - docs/design/008-superpowers执行与跨仓同步设计.md
  - docs/acceptance/002-superpowers执行与跨仓验收标准-验收.md
  - docs/cross-repo-client-generation.md
  - README.md
  - scripts/validate-repository.sh
outputs:
  - server 优先执行基线
  - web 与 miniapp 同步流程
  - browser 回归与代码审核交付基线
triggers:
  - 需要按 superpowers 执行 server 优先任务
  - server 契约变更需要同步 web 与 miniapp
downstream: []
---

# Superpowers 执行与跨仓同步计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 HotKey 的任务执行固定为 `server -> web -> miniapp -> 回归`，并把工程化规范、`vercel:agent-browser` 回归、代码审核和提交要求落实为可执行流程。

**Architecture:** 先在 `hotkey-server` 冻结契约事实源与工程化门禁，再基于 `docs/swagger.json` 驱动 `hotkey-web` 与 `hotkey-miniapp` 同步生成客户端和页面适配，最后做浏览器回归、代码审核和按仓提交。

**Tech Stack:** Go, OpenAPI 3.1, Bash validation scripts, Next.js, Taro, @umijs/openapi, vercel:agent-browser, git

## Global Constraints

- 必须遵循 `superpowers:brainstorming -> writing-plans -> executing-plans/subagent-driven-development -> verification-before-completion`。
- 默认执行顺序固定为 `hotkey-server -> hotkey-web -> hotkey-miniapp -> 跨仓回归`。
- `hotkey-server/docs/swagger.json` 是唯一契约事实源。
- 端侧不得绕过生成客户端手写漂移 API 类型。
- 交付前必须同时完成工程化检查、`vercel:agent-browser` 回归和代码审核。
- 提交信息必须使用项目允许的前缀：`test:`、`docs:`、`impl:`、`feat:`、`chore:`、`refactor:`。

---

### Task 1: 冻结 server 执行入口与契约门禁

**Files:**
- Modify: `hotkey-server/docs/design/008-superpowers执行与跨仓同步设计.md`
- Modify: `hotkey-server/docs/acceptance/002-superpowers执行与跨仓验收标准-验收.md`
- Modify: `hotkey-server/docs/cross-repo-client-generation.md`
- Test: `hotkey-server/scripts/validate-swagger.sh`

**Interfaces:**
- Consumes: `make swagger`, `make swagger-validate`, `bash scripts/validate-repository.sh`
- Produces: server 完成判定标准，供 Task 2、Task 3、Task 4 使用

- [ ] **Step 1: 补一条失败用例说明，先证明 server 未冻结契约时不允许进入下游**

```text
反例:
- 仅修改 `internal/*/http.go` 或 `internal/platform/http/*.go`
- 未运行 `make swagger`
- 未更新 `docs/swagger.json`
- 却尝试在 web / miniapp 先改页面

预期:
- 该任务在验收记录中判定为“不允许进入下游同步”
```

- [ ] **Step 2: 运行契约验证命令，确认当前 server 基线可作为上游事实源**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
make swagger
make swagger-validate
bash scripts/validate-repository.sh
```

Expected: `docs/swagger.json` 成功生成并通过校验；仓内验证脚本通过或给出明确失败原因

- [ ] **Step 3: 若契约校验失败，先修复最小问题再重新验证**

```go
// 例如在 cmd/hotkey/main.go 或 internal/platform/http/*.go 中
// 保持导出的 spec 与当前 router/handler 注册一致
func ExportOpenAPI() error {
    // 只保留当前真实启用的路由与组件
    return nil
}
```

- [ ] **Step 4: 更新跨仓生成指南，使其直接引用当前固定命令**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
make swagger
make swagger-validate
```

- [ ] **Step 5: 提交 server 契约门禁文档或修复**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
git add docs/design/008-superpowers执行与跨仓同步设计.md \
        docs/acceptance/002-superpowers执行与跨仓验收标准-验收.md \
        docs/cross-repo-client-generation.md \
        docs/swagger.json cmd/hotkey/main.go internal/platform/http/*.go
git commit -m "docs: 冻结server契约门禁与同步入口"
```

### Task 2: 建立 server 工程化检查与代码审核闭环

**Files:**
- Modify: `hotkey-server/internal/platform/http/errors.go`
- Modify: `hotkey-server/internal/platform/http/middleware.go`
- Modify: `hotkey-server/internal/platform/http/openapi_coverage_test.go`
- Modify: `hotkey-server/scripts/validate-repository.sh`
- Test: `hotkey-server/internal/platform/http/openapi_coverage_test.go`

**Interfaces:**
- Consumes: Task 1 的 server 完成判定标准
- Produces: server 工程化检查入口，供 Task 5 交付说明引用

- [ ] **Step 1: 先写一个失败测试，覆盖统一错误出口或契约覆盖缺口**

```go
func TestOpenAPICoverageIncludesAllRegisteredRoutes(t *testing.T) {
    spec := loadSpec(t, "docs/swagger.json")
    routes := registeredAPIRoutes()
    for _, route := range routes {
        if !spec.HasPath(route.Path) {
            t.Fatalf("missing openapi path for %s", route.Path)
        }
    }
}
```

- [ ] **Step 2: 运行单测，确认它先失败**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go test ./internal/platform/http -run TestOpenAPICoverageIncludesAllRegisteredRoutes -v
```

Expected: FAIL，并明确指出缺失的 path 或错误出口不一致

- [ ] **Step 3: 做最小实现，统一错误映射和覆盖校验**

```go
func WriteAPIError(ctx *gin.Context, err error) {
    mapped := MapError(err)
    ctx.JSON(mapped.Status, mapped.Body)
}
```

- [ ] **Step 4: 重新运行相关测试和全仓验证**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go test ./internal/platform/http -v
go test ./...
bash scripts/validate-repository.sh
```

Expected: PASS

- [ ] **Step 5: 记录代码审核结论并提交**

```text
审核检查:
- 是否还有未兜底错误出口
- 是否有 handler/service/repository 边界混写
- 是否只覆盖 happy path
- 是否有高可用性残余风险
```

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
git add internal/platform/http/errors.go \
        internal/platform/http/middleware.go \
        internal/platform/http/openapi_coverage_test.go \
        scripts/validate-repository.sh \
        docs/acceptance/002-superpowers执行与跨仓验收标准-验收.md
git commit -m "impl: 补齐server工程化校验与审核闭环"
```

### Task 3: 同步 hotkey-web 的客户端生成与端侧门禁

**Files:**
- Modify: `hotkey-web/openapi2ts.config.ts`
- Modify: `hotkey-web/src/services/hotkey/hotkey-server/*.ts`
- Modify: `hotkey-web/scripts/validate-repository.sh`
- Modify: `hotkey-web/docs/acceptance/` 下受影响验收文档
- Test: `hotkey-web/package.json`

**Interfaces:**
- Consumes: Task 1 产出的 `hotkey-server/docs/swagger.json`
- Produces: Web 端契约同步与工程化通过状态，供 Task 5 浏览器回归使用

- [ ] **Step 1: 先运行生成命令和现有验证，确认当前 web 基线**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-web
npm run openapi:generate
npm run typecheck
npm run build
bash scripts/validate-repository.sh
```

Expected: 成功生成 `src/services/hotkey/hotkey-server/*`，类型检查和构建通过

- [ ] **Step 2: 如有字段或接口变化，先写失败测试或最小断言**

```python
def test_generated_client_contains_hotkey_namespace():
    with open("src/services/hotkey/hotkey-server/index.ts", "r", encoding="utf-8") as fh:
        content = fh.read()
    assert "HotKeyAPI" in content
```

- [ ] **Step 3: 同步全局请求封装、错误处理和状态接入点**

```ts
export async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(url, options);
  if (!response.ok) {
    throw await mapAPIError(response);
  }
  return response.json() as Promise<T>;
}
```

- [ ] **Step 4: 重新执行生成、类型检查、构建和仓内校验**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-web
npm run openapi:generate
npm run test
npm run typecheck
npm run build
bash scripts/validate-repository.sh
```

Expected: PASS

- [ ] **Step 5: 提交 Web 同步结果**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-web
git add openapi2ts.config.ts \
        src/services/hotkey/hotkey-server \
        scripts/validate-repository.sh \
        docs/acceptance
git commit -m "impl: 同步web契约与端侧门禁"
```

### Task 4: 同步 hotkey-miniapp 的客户端生成与端侧门禁

**Files:**
- Modify: `hotkey-miniapp/openapi2ts.config.ts`
- Modify: `hotkey-miniapp/src/services/hotkey/hotkey-server/*.ts`
- Modify: `hotkey-miniapp/scripts/validate-repository.sh`
- Modify: `hotkey-miniapp/docs/acceptance/` 下受影响验收文档
- Test: `hotkey-miniapp/package.json`

**Interfaces:**
- Consumes: Task 1 的 `hotkey-server/docs/swagger.json`
- Produces: Miniapp 端契约同步与工程化通过状态，供 Task 5 汇总

- [ ] **Step 1: 先运行生成命令和当前验证，确认 miniapp 基线**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-miniapp
npm run openapi:generate
npm run typecheck
npm run build:weapp
bash scripts/validate-repository.sh
```

Expected: 生成客户端成功，类型检查与小程序构建通过

- [ ] **Step 2: 对登录失效态或错误态先补最小失败断言**

```python
def test_generated_client_has_request_entry():
    with open("src/services/hotkey/hotkey-server/index.ts", "r", encoding="utf-8") as fh:
        content = fh.read()
    assert "request" in content
```

- [ ] **Step 3: 同步端侧请求封装、错误处理和状态接入点**

```ts
export async function request<T>(options: RequestOptions): Promise<T> {
  const res = await Taro.request<T>(options);
  if (res.statusCode >= 400) {
    throw mapMiniappAPIError(res);
  }
  return res.data;
}
```

- [ ] **Step 4: 重新执行生成、测试、类型检查和构建**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-miniapp
npm run openapi:generate
npm run test
npm run typecheck
npm run build:weapp
bash scripts/validate-repository.sh
```

Expected: PASS

- [ ] **Step 5: 提交 Miniapp 同步结果**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-miniapp
git add openapi2ts.config.ts \
        src/services/hotkey/hotkey-server \
        scripts/validate-repository.sh \
        docs/acceptance
git commit -m "impl: 同步miniapp契约与端侧门禁"
```

### Task 5: 执行 browser 回归、汇总代码审核并按仓提交

**Files:**
- Modify: `hotkey-server/docs/acceptance/002-superpowers执行与跨仓验收标准-验收.md`
- Modify: `hotkey-web/docs/acceptance/*`
- Modify: `hotkey-miniapp/docs/acceptance/*`
- Test: `hotkey-web`, `hotkey-miniapp` 运行中的页面或调试入口

**Interfaces:**
- Consumes: Task 2、Task 3、Task 4 的验证结果
- Produces: 最终交付说明、browser 证据和按仓提交状态

- [ ] **Step 1: 启动 server 与 web 调试环境**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
make dev

cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-web
npm run dev
```

Expected: server 在本地可访问；web 页面可在浏览器打开

- [ ] **Step 2: 使用 `vercel:agent-browser` 跑 Web 主链路**

```bash
agent-browser open http://localhost:3000
agent-browser wait --load networkidle
agent-browser snapshot -i
```

Expected: 至少覆盖登录、热点榜单、热点详情、AI 快速理解或本次受影响链路

- [ ] **Step 3: 记录 miniapp 的等价验证路径**

```text
若存在浏览器可承载入口:
- 使用 agent-browser 做同等链路验证

若不存在:
- 记录小程序调试入口
- 记录步骤、预期结果、实际结果和阻塞点
```

- [ ] **Step 4: 汇总代码审核结论与残余风险**

```text
必须写入:
- 修改了什么
- 如何验证
- 工程化检查结果
- browser 回归结果
- 代码审核结论
- 未验证内容或残余风险
- 关键文件
- Git 提交状态和 PR 状态
```

- [ ] **Step 5: 检查各仓工作区并完成最终提交**

```bash
git -C /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server status --short
git -C /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-web status --short
git -C /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-miniapp status --short
```

Expected: 每个仓只包含本次任务相关文件；提交信息遵守 `docs:` / `impl:` / `feat:` 等前缀
