# Authentication Response and End-to-End Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Standardize every API response as numeric `code` plus stable `error_code` and `data`, then make the complete Web email registration, login, refresh, current-user, logout, and password-reset flows executable and verified end to end.

**Architecture:** `hotkey-server` owns the ErrorCode enum, HTTP mapping registry, OpenAPI schema, and authentication behavior. `hotkey-web` consumes generated types, converts `error_code` into a centralized UI message, and owns access-token refresh/retry behavior; pages never branch on HTTP text or duplicate error copy. Delivery follows `server -> web -> runtime acceptance` and removes the legacy direct-email registration path.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL, Redis, Swaggo/OpenAPI, Next.js 16, React 19, TypeScript, Axios, Zustand, Vitest, browser acceptance.

## Global Constraints

- Follow `docs/design/018-邮箱认证与会话安全设计.md` version 1.1.0.
- JSON envelopes contain exactly `code`, `error_code`, and `data`, plus existing pagination fields where applicable; they contain neither `message` nor `request_id`.
- `code` equals the actual HTTP status; `error_code` comes from the Server registry; successful responses use `SUCCESS`; failed responses use `data: null`.
- Keep request tracing in `X-Request-Id`, request context, and logs only.
- Server OpenAPI is the contract source. Web does not hand-write server response or ErrorCode types.
- Registration requires `verification_ticket`; remove the legacy direct-email branch.
- Page components do not define API error copy. Unknown codes display `操作失败，请稍后重试`.
- Preserve 15-minute Access Tokens, HttpOnly Refresh Cookie rotation, 7-day sliding Session, and 30-day absolute Session cap.
- Apply red-green-refactor and keep test-only commits separate from implementation commits.

---

## File Map

### Server

- `internal/model/enum/error_code.go`: canonical public ErrorCode values.
- `internal/model/vo/common.go`: unified response and page envelopes.
- `internal/platform/http/errors.go`: ErrorCode-to-HTTP metadata registry and AppError.
- `internal/platform/http/response.go`: the only JSON response writer.
- `internal/controller/auth_controller.go`: transport-only auth handlers and cookie operations.
- `internal/model/dto/auth_request.go`: ticket-only registration and other auth requests.
- `internal/controller/swagger_response.go`: generated-client response schemas.
- `tests/unit/platform/http/router_test.go`: exact envelope and handler-contract tests.
- `tests/integration/api/auth_flow_test.go`: complete authentication API flow.
- `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`: generated OpenAPI artifacts.

### Web

- `src/lib/authErrors.ts`: centralized ErrorCode-to-Chinese-message map.
- `src/lib/request.ts`: envelope parser, typed API error, refresh, and one-time retry.
- `src/lib/authSession.ts`: in-memory Access Token lifecycle.
- `src/stores/authStore.ts`: initialization, login, logout, and authenticated user state.
- `src/app/register/page.tsx`: ticket registration flow.
- `src/app/login/page.tsx`: password login flow.
- `src/components/auth/EmailVerificationStep.tsx`: verification send/confirm flow.
- `src/app/forgot-password/page.tsx`, `src/app/reset-password/page.tsx`: ticket password reset.
- `src/services/auth.ts`, `src/services/typings.d.ts`: generated OpenAPI client.
- `src/lib/__tests__/request.test.ts`, `src/stores/__tests__/authStore.test.ts`: request/session tests.
- `src/components/auth/__tests__/authFlows.test.tsx`: page-level auth behavior.

---

### Task 1: Freeze the Server ErrorCode and Envelope Contract

**Files:**
- Modify: `internal/model/enum/error_code.go`
- Modify: `internal/model/vo/common.go`
- Modify: `internal/platform/http/errors.go`
- Modify: `internal/platform/http/response.go`
- Test: `tests/unit/platform/http/router_test.go`

**Interfaces:**
- Produces: `enum.ErrorCode`, `ErrorSpec`, `NewAppError`, `ResponseBody`, `PageBody`, unified responders.
- Consumes: standard `net/http` status constants and Gin context.

- [ ] **Step 1: Write failing exact-shape tests**

Add helpers that unmarshal into `map[string]json.RawMessage` and assert exact keys:

```go
func assertEnvelope(t *testing.T, rr *httptest.ResponseRecorder, status int, errorCode string, data string) {
    t.Helper()
    if rr.Code != status { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
    var body map[string]json.RawMessage
    if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil { t.Fatal(err) }
    if len(body) != 3 { t.Fatalf("unexpected keys: %v", body) }
    if string(body["code"]) != strconv.Itoa(status) { t.Fatalf("code=%s", body["code"]) }
    if string(body["error_code"]) != strconv.Quote(errorCode) { t.Fatalf("error_code=%s", body["error_code"]) }
    if string(body["data"]) != data { t.Fatalf("data=%s", body["data"]) }
    if _, exists := body["message"]; exists { t.Fatal("message must be absent") }
    if _, exists := body["request_id"]; exists { t.Fatal("request_id must be absent") }
}
```

Cover HTTP 200, 201, 202, 400, 401, 403, 404, 405, 409, 429, and 500, plus paginated success.

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/unit/platform/http -run 'Envelope|Respond' -count=1 -v`

Expected: FAIL because the current body has `message` and lacks `error_code`.

- [ ] **Step 3: Define the canonical enums and registry**

Use exact public values from design 018, including:

```go
const (
    ErrorCodeSuccess ErrorCode = "SUCCESS"
    ErrorCodeInvalidInput ErrorCode = "AUTH_INVALID_INPUT"
    ErrorCodeInvalidCredentials ErrorCode = "AUTH_INVALID_CREDENTIALS"
    ErrorCodeEmailAlreadyRegistered ErrorCode = "AUTH_EMAIL_ALREADY_REGISTERED"
    ErrorCodeVerificationInvalid ErrorCode = "AUTH_VERIFICATION_INVALID"
    ErrorCodeVerificationExpired ErrorCode = "AUTH_VERIFICATION_EXPIRED"
    ErrorCodeSessionExpired ErrorCode = "AUTH_SESSION_EXPIRED"
    ErrorCodeSessionRevoked ErrorCode = "AUTH_SESSION_REVOKED"
    ErrorCodeTokenInvalid ErrorCode = "AUTH_TOKEN_INVALID"
    ErrorCodeTokenReused ErrorCode = "AUTH_TOKEN_REUSED"
    ErrorCodeAccountDisabled ErrorCode = "AUTH_ACCOUNT_DISABLED"
    ErrorCodePasswordPolicyViolation ErrorCode = "AUTH_PASSWORD_POLICY_VIOLATION"
)
```

Keep `ErrorSpec` limited to HTTP status, retryability, and security-event metadata. Unknown codes map to `INTERNAL_ERROR` and HTTP 500.

- [ ] **Step 4: Implement the minimal envelope**

```go
type ResponseBody struct {
    Code int `json:"code"`
    ErrorCode enum.ErrorCode `json:"error_code"`
    Data any `json:"data"`
}
```

All responders derive `code` from the selected HTTP status and never accept message text. Preserve `X-Request-Id` middleware behavior.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./tests/unit/platform/http ./tests/integration/api -count=1 -v && go vet ./...`

Expected: PASS with exact envelope keys and no response-body trace ID.

- [ ] **Step 6: Commit**

```bash
git add internal/model/enum/error_code.go internal/model/vo/common.go internal/platform/http/errors.go internal/platform/http/response.go tests/unit/platform/http/router_test.go tests/integration/api
git commit -m "test: 固定统一状态响应契约"
git commit -m "impl: 统一数字状态与业务错误码响应"
```

### Task 2: Make Auth Services Return Typed AppErrors

**Files:**
- Modify: `internal/service/auth_service.go`
- Modify: `internal/service/verification_service.go`
- Modify: `internal/service/session_service.go`
- Modify: `internal/controller/auth_controller.go`
- Test: `tests/unit/auth/service_test.go`
- Test: `tests/unit/verification/service_test.go`
- Test: `tests/unit/session/service_test.go`

**Interfaces:**
- Consumes: `platformhttp.NewAppError(enum.ErrorCode, cause)` or a transport-neutral domain error carrying `enum.ErrorCode`.
- Produces: auth operations whose external failure is determined once, outside controllers.

- [ ] **Step 1: Write failing error-mapping tests**

Add table cases for invalid credentials, account disabled, invalid/expired/claimed Ticket, password-policy failure, Session expired/revoked, token reuse, rate limit, Redis unavailable, and unknown dependency failure. Each case asserts exact `ErrorCode` and HTTP status.

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/unit/auth ./tests/unit/verification ./tests/unit/session -run 'Error|Failure|Expired|Reused' -count=1 -v`

Expected: FAIL because sentinel errors are currently remapped by controller switches.

- [ ] **Step 3: Centralize mapping at service boundaries**

Return one typed error per failure and preserve internal causes for logs. Remove public message strings and make unknown failures become `INTERNAL_ERROR`.

- [ ] **Step 4: Make controllers transport-only**

Each handler performs request binding, calls the service, sets/clears cookies on success, and calls `c.Error(err)` on failure. Delete business-error switch blocks and custom error text.

- [ ] **Step 5: Verify GREEN**

Run: `go test -race ./tests/unit/auth ./tests/unit/verification ./tests/unit/session ./tests/unit/platform/http -count=1`

Expected: PASS with no handler-owned business mapping.

- [ ] **Step 6: Commit**

```bash
git add internal/service internal/controller/auth_controller.go tests/unit
git commit -m "test: 覆盖认证业务错误映射"
git commit -m "refactor: 集中认证错误状态处理"
```

### Task 3: Remove Legacy Registration and Add Full Server Auth Integration

**Files:**
- Modify: `internal/controller/auth_controller.go`
- Modify: `internal/model/dto/auth_request.go`
- Create: `tests/integration/api/auth_flow_test.go`
- Modify: `tests/integration/api/main_test.go`

**Interfaces:**
- Consumes: verification send/confirm, `RegisterVerified`, login, refresh, me, logout, reset.
- Produces: one executable API acceptance flow and ticket-only register contract.

- [ ] **Step 1: Write a failing legacy-registration rejection test**

```go
func TestRegisterRejectsDirectEmailPayload(t *testing.T) {
    rr := performJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
        "email": "bypass@example.com", "password": "Passw0rd!", "display_name": "Bypass",
    })
    assertEnvelope(t, rr, http.StatusBadRequest, "AUTH_INVALID_INPUT", "null")
}
```

- [ ] **Step 2: Write the complete failing flow**

Use real PostgreSQL and Redis with a deterministic fake mailer/code generator:

```text
send register code -> confirm -> register with ticket -> me -> logout
-> login -> expire access token -> refresh cookie -> me -> reset password
-> old session refresh rejected -> login with new password
```

Assert every response status, `code`, `error_code`, `data`, `Set-Cookie`, and rotated refresh value.

- [ ] **Step 3: Verify RED**

Run: `go test ./tests/integration/api -run 'RegisterRejectsDirect|CompleteAuthFlow' -count=1 -v`

Expected: FAIL because direct registration remains and the integration harness does not yet execute the full verified flow.

- [ ] **Step 4: Delete the legacy branch**

Bind only `dto.EmailRegisterRequest` at `/api/v1/auth/register`; remove JSON shape detection and calls to `AuthService.Register` from HTTP paths.

- [ ] **Step 5: Complete integration wiring and verify GREEN**

Run: `go test -race ./tests/integration/api -count=1 -v`

Expected: PASS for the full flow and all negative states.

- [ ] **Step 6: Commit**

```bash
git add internal/controller/auth_controller.go internal/model/dto/auth_request.go tests/integration/api
git commit -m "test: 添加完整认证链路集成验收"
git commit -m "impl: 强制验证码票据注册"
```

### Task 4: Publish ErrorCode and Envelope Through OpenAPI

**Files:**
- Modify: `internal/controller/swagger_response.go`
- Modify: `internal/platform/http/errors.go`
- Modify: `scripts/validate-openapi.sh` or create it if absent.
- Modify: `Makefile`
- Generate: `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`
- Test: `tests/integration/api/openapi_contract_test.go`

**Interfaces:**
- Produces: OpenAPI schemas containing numeric `code`, enum `error_code`, nullable `data`, and no `message`/`request_id`.
- Consumers: Web OpenAPI generation.

- [ ] **Step 1: Write failing OpenAPI assertions**

Parse `docs/swagger.json`; assert `ErrorBody.required == ["code", "error_code", "data"]`, `code.type == integer`, `error_code` exposes the enum, and register accepts only `EmailRegisterRequest`.

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/integration/api -run OpenAPIContract -count=1 -v`

Expected: FAIL because current wrappers still publish `message` and omit `error_code`.

- [ ] **Step 3: Update wrappers and deterministic generation**

Make `make openapi` run the repository-pinned Swaggo version and write all three artifacts. Add `make openapi-validate` that regenerates and fails on a diff.

- [ ] **Step 4: Verify GREEN**

Run: `make openapi && make openapi-validate && go test ./tests/integration/api -run OpenAPIContract -count=1 -v`

Expected: PASS and generated schemas match runtime envelopes.

- [ ] **Step 5: Commit**

```bash
git add Makefile scripts internal/controller/swagger_response.go internal/platform/http/errors.go docs tests/integration/api
git commit -m "test: 固定OpenAPI状态响应契约"
git commit -m "chore: 发布认证错误码OpenAPI契约"
```

### Task 5: Generate the Web Contract and Centralize UI Error Messages

**Files:**
- Modify: `../hotkey-web/openapi2ts.config.ts`
- Generate: `../hotkey-web/src/services/auth.ts`
- Generate: `../hotkey-web/src/services/typings.d.ts`
- Modify: `../hotkey-web/src/lib/authErrors.ts`
- Modify: `../hotkey-web/src/lib/request.ts`
- Test: `../hotkey-web/src/lib/__tests__/request.test.ts`

**Interfaces:**
- Consumes: Server `docs/swagger.json` and generated `HotKeyAPI.ErrorCode`.
- Produces: `HotKeyAPIError { status, errorCode }` and `errorMessage(errorCode)`.

- [ ] **Step 1: Write failing request/error-map tests**

```ts
expect(new HotKeyAPIError(401, "AUTH_INVALID_CREDENTIALS")).toMatchObject({
  status: 401,
  errorCode: "AUTH_INVALID_CREDENTIALS",
});
expect(errorMessage("AUTH_INVALID_CREDENTIALS")).toBe("邮箱或密码错误");
expect(errorMessage("UNKNOWN" as never)).toBe("操作失败，请稍后重试");
```

Also assert the parser ignores response headers for business data and rejects malformed envelopes.

- [ ] **Step 2: Verify RED**

Run: `npm run test:unit -- src/lib/__tests__/request.test.ts`

Expected: FAIL because the current client parses numeric `code` as the business code and reads backend message text.

- [ ] **Step 3: Generate clients and implement the central map**

Run `npm run openapi:generate`. Replace hand-written mismatched enums with the generated ErrorCode type. `authErrors.ts` owns all Chinese messages; pages receive only `errorCode`.

- [ ] **Step 4: Implement strict envelope parsing**

```ts
export class HotKeyAPIError extends Error {
  constructor(public status: number, public errorCode: HotKeyAPI.ErrorCode) {
    super(errorMessage(errorCode));
    this.name = "HotKeyAPIError";
  }
}
```

Keep single-flight refresh and one retry. Refresh failure produces `AUTH_SESSION_EXPIRED` locally and clears in-memory authentication.

- [ ] **Step 5: Verify GREEN**

Run: `npm run typecheck && npm run test:unit`

Expected: PASS; no source file references response `message`, `request_id`, or `requestId`.

- [ ] **Step 6: Commit in `hotkey-web`**

```bash
git add openapi2ts.config.ts src/services src/lib
git commit -m "test: 固定前端认证错误状态映射"
git commit -m "impl: 集中展示认证错误状态"
```

### Task 6: Complete Web Registration, Login, Refresh, Logout, and Reset Flows

**Files:**
- Modify: `../hotkey-web/src/stores/authStore.ts`
- Modify: `../hotkey-web/src/app/register/page.tsx`
- Modify: `../hotkey-web/src/app/login/page.tsx`
- Modify: `../hotkey-web/src/components/auth/EmailVerificationStep.tsx`
- Modify: `../hotkey-web/src/app/forgot-password/page.tsx`
- Modify: `../hotkey-web/src/app/reset-password/page.tsx`
- Test: `../hotkey-web/src/stores/__tests__/authStore.test.ts`
- Create: `../hotkey-web/src/components/auth/__tests__/authFlows.test.tsx`

**Interfaces:**
- Consumes: generated auth services, `HotKeyAPIError`, `errorMessage`, in-memory session helpers.
- Produces: complete page-level authentication behavior without duplicated API copy.

- [ ] **Step 1: Write failing component/store tests**

Cover ticket passed into registration, registered Session stored without a second login request, invalid credentials shown from centralized mapping, app initialization refreshes then calls `/auth/me`, logout clears state even when server is unavailable, reset Ticket stays in memory, and unknown errors use the fallback.

- [ ] **Step 2: Verify RED**

Run: `npm run test:unit -- src/stores/__tests__/authStore.test.ts src/components/auth/__tests__/authFlows.test.tsx`

Expected: FAIL on second-login registration, session initialization, and page-owned error fallback behavior.

- [ ] **Step 3: Implement minimal flow state machines**

Registration consumes the `LoginResponse` returned by ticket registration and stores its Access Token directly. Do not persist verification/reset Tickets in URL, localStorage, or sessionStorage. Store only in component memory; refresh returns to step one.

- [ ] **Step 4: Verify GREEN and storage safety**

Run: `npm run typecheck && npm run test:unit && rg -n 'localStorage|sessionStorage' src/app src/components src/lib src/stores`

Expected: tests pass and no Access Token, refresh token, verification Ticket, or reset Ticket is persisted.

- [ ] **Step 5: Commit in `hotkey-web`**

```bash
git add src/app src/components/auth src/stores
git commit -m "test: 覆盖Web完整认证状态流"
git commit -m "impl: 跑通Web注册登录与会话恢复"
```

### Task 7: Runtime Acceptance and Cross-Repository Closeout

**Files:**
- Modify: `docs/acceptance/004-auth-response-and-flow-acceptance.md`
- Modify: `openspec/changes/2026-07-12-fix-auth-response-contract/tasks.md`

**Interfaces:**
- Consumes: running Server, PostgreSQL, Redis, test/real SMTP, and built Web app.
- Produces: reproducible acceptance evidence and clean repository diffs.

- [ ] **Step 1: Run fresh Server gates**

Run:

```bash
make openapi
make openapi-validate
bash scripts/validate-repository.sh
go test -race ./...
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 2: Run fresh Web gates**

Run:

```bash
npm run openapi:generate
npm run typecheck
npm run test:unit
npm run build
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 3: Execute browser acceptance**

With real PostgreSQL/Redis and a deterministic SMTP test account, verify registration, authenticated dashboard, logout, login failure mapping, login success, forced Access Token expiration and automatic refresh, password reset, and old-session rejection. Capture screenshots and network evidence showing `code`, `error_code`, `data` only.

- [ ] **Step 4: Scan forbidden contract residue**

Run:

```bash
rg -n 'json:"message"|json:"request_id"|request_id\?:|requestId|AUTH_EMAIL_TAKEN|AUTH_WEAK_PASSWORD' internal tests ../hotkey-web/src
```

Expected: no runtime response or obsolete frontend enum residue; logging/request-context references are explicitly excluded or reviewed.

- [ ] **Step 5: Record evidence and commit closeout**

Document commands, results, screenshots, environment limits, repository commit IDs, and any SMTP delivery constraint in the acceptance file. Mark every OpenSpec task complete only after runtime evidence exists.

```bash
git add docs/acceptance openspec/changes/2026-07-12-fix-auth-response-contract
git commit -m "docs: 记录认证状态响应与完整链路验收"
```

---

## Plan Self-Review

- Every design 018 v1.1.0 envelope field has a Server test, OpenAPI assertion, generated Web type, and runtime acceptance check.
- Legacy direct registration is rejected explicitly and removed from integration fixtures.
- Each authentication state has one Server ErrorCode and one centralized Web message.
- Refresh rotation is verified at service, API, request-layer, and browser levels.
- No task introduces `message`, response-body `request_id`, localStorage tokens, or a hand-written duplicate API enum.
