# Authentication Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish the global response/ErrorCode contract, authentication model primitives, schema, configuration, and security utilities required by the email authentication flows.

**Architecture:** Extend the existing Gin + GORM layered application without introducing a parallel domain tree. Stable enums and transport models live under `internal/model`, cryptographic primitives under `internal/platform/security`, and PostgreSQL remains authoritative for users and refresh sessions.

**Tech Stack:** Go 1.24, Gin, GORM, PostgreSQL, `golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt`, Viper, Swaggo.

## Global Constraints

- Follow `docs/design/018-邮箱认证与会话安全设计.md` as the accepted contract.
- Keep `db/schema.sql` as the only canonical database structure file; do not create `db/migrations`.
- Use `internal/model/entity`, `dto`, `enum`, and `vo`; do not create a second model hierarchy.
- CRUD request names use `{Resource}AddRequest`, `{Resource}EditRequest`, `{Resource}QueryRequest`, `{Resource}DetailRequest`, `{Resource}DeleteRequest`; authentication actions retain semantic names.
- Passwords are 8-64 Unicode characters, at most 72 UTF-8 bytes, and contain at least one ASCII letter and one digit.
- Access Tokens expire after 15 minutes; Refresh Sessions slide for 7 days and have an absolute 30-day limit.
- Never persist or log plaintext passwords, verification codes, raw access/refresh tokens, cookies, SMTP authorization codes, or complete email addresses.
- Apply TDD and commit after every task.

---

## File Map

**Create**

- `internal/model/enum/auth.go`: verification purposes, account status, revoke reasons.
- `internal/model/dto/common_request.go`: shared page/ID/delete request shapes.
- `internal/model/entity/auth_session.go`: GORM refresh-session model.
- `internal/platform/security/password.go`: email normalization and password policy/hash functions.
- `internal/platform/security/token.go`: access/refresh token generation and parsing.
- `internal/platform/security/digest.go`: HMAC and SHA-256 helpers.
- `tests/unit/platform/security/password_test.go`: password and email tests.
- `tests/unit/platform/security/token_test.go`: JWT and refresh-token tests.

**Modify**

- `internal/model/enum/error_code.go`: global and auth-specific stable ErrorCode values.
- `internal/model/entity/user.go`: verification and password/login timestamps.
- `internal/model/dto/auth.go`: internal auth/session DTOs.
- `internal/model/dto/auth_request.go`: semantic auth request DTOs.
- `internal/model/vo/common.go`: one success/error/page envelope.
- `internal/model/vo/auth.go`: public authenticated-user/token/session VOs.
- `internal/platform/http/errors.go`: central ErrorCode registry.
- `internal/platform/http/response.go`: unified responders and global error handling.
- `internal/platform/http/middleware.go`: typed JWT validation, 404/405, safe recovery, CORS allowlist.
- `internal/config/server.go`, `internal/config/config.go`, `.env.example`: auth/CORS/token settings.
- `db/schema.sql`: users extension and `auth_sessions` table.
- `tests/testutil/db.go`: clean `auth_sessions` before `users`.
- `tests/integration/api/main_test.go`: new response-envelope assertions.

### Task 1: Freeze ErrorCode and Response Envelope

**Files:**
- Modify: `internal/model/enum/error_code.go`
- Modify: `internal/model/vo/common.go`
- Modify: `internal/platform/http/errors.go`
- Modify: `internal/platform/http/response.go`
- Test: `tests/integration/api/main_test.go`

**Interfaces:**
- Produces: `enum.ErrorCode`, `platformhttp.ErrorSpec`, `platformhttp.NewAppError(code, cause)`, `vo.ResponseBody`, `vo.PageBody`.
- Consumers: every later Server controller and middleware task.

- [ ] **Step 1: Write failing envelope and registry tests**

Add table-driven tests that call a test route returning `RespondOK`, `RespondPage`, and `c.Error(NewAppError(...))` and assert exact JSON keys:

```go
func TestUnifiedEnvelope(t *testing.T) {
    assertJSON := func(body []byte, code string, dataIsNull bool) {
        var got struct {
            Code string `json:"code"`
            Message string `json:"message"`
            Data json.RawMessage `json:"data"`
            RequestID string `json:"request_id"`
        }
        if err := json.Unmarshal(body, &got); err != nil { t.Fatal(err) }
        if got.Code != code || got.RequestID == "" { t.Fatalf("unexpected envelope: %s", body) }
        if dataIsNull && string(got.Data) != "null" { t.Fatalf("expected null data: %s", body) }
    }
    _ = assertJSON
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test ./tests/integration/api -run TestUnifiedEnvelope -count=1 -v`

Expected: FAIL because the current success body lacks `code/message` and the error body uses `error`.

- [ ] **Step 3: Add exact enums and central specs**

Define `SUCCESS`, common codes, and all auth codes from design 018. Use one registry:

```go
type ErrorSpec struct {
    HTTPStatus int
    Message string
    Retryable bool
    SecurityEvent bool
}

var errorSpecs = map[enum.ErrorCode]ErrorSpec{
    enum.ErrorCodeInvalidCredentials: {HTTPStatus: http.StatusUnauthorized, Message: "邮箱或密码错误", SecurityEvent: true},
    enum.ErrorCodeRateLimited: {HTTPStatus: http.StatusTooManyRequests, Message: "请求过于频繁，请稍后重试", Retryable: true},
    enum.ErrorCodeServiceUnavailable: {HTTPStatus: http.StatusServiceUnavailable, Message: "服务暂时不可用", Retryable: true},
}
```

`NewAppError` accepts only a stable code and internal cause; callers cannot override safe public messages.

- [ ] **Step 4: Implement one envelope for all responders**

```go
type ResponseBody struct {
    Code enum.ErrorCode `json:"code"`
    Message string `json:"message"`
    Data any `json:"data"`
    Meta *PageMeta `json:"meta,omitempty"`
    RequestID string `json:"request_id"`
}

func RespondOK(c *gin.Context, data any) {
    c.JSON(http.StatusOK, vo.ResponseBody{Code: enum.ErrorCodeSuccess, Message: "success", Data: data, RequestID: requestIDFromContext(c)})
}
```

Make `RespondCreated`, `RespondAccepted`, `RespondPage`, `RespondAppError`, unknown-error fallback, and panic recovery use the same shape with `data: null` on error.

- [ ] **Step 5: Run focused and package tests**

Run: `go test ./internal/platform/http ./tests/integration/api -count=1 -v`

Expected: PASS; every asserted body contains `code`, `message`, `data`, and `request_id`.

- [ ] **Step 6: Commit**

```bash
git add internal/model/enum/error_code.go internal/model/vo/common.go internal/platform/http/errors.go internal/platform/http/response.go tests/integration/api/main_test.go
git commit -m "refactor: unify api response and error contracts"
```

### Task 2: Add DTO, Enum, Entity, and Schema Primitives

**Files:**
- Create: `internal/model/enum/auth.go`
- Create: `internal/model/dto/common_request.go`
- Create: `internal/model/entity/auth_session.go`
- Modify: `internal/model/entity/user.go`
- Modify: `internal/model/dto/auth.go`
- Modify: `internal/model/dto/auth_request.go`
- Modify: `internal/model/vo/auth.go`
- Modify: `db/schema.sql`
- Modify: `tests/testutil/db.go`
- Test: `tests/unit/database/bootstrap_test.go`

**Interfaces:**
- Produces: `enum.VerificationPurpose`, `enum.AccountStatus`, `enum.SessionRevokeReason`, `entity.AuthSession`, auth Request DTOs and auth VOs.
- Consumers: repositories and services in plan 2.

- [ ] **Step 1: Write failing schema and model tests**

Assert `db/schema.sql` contains the three user columns, `auth_sessions`, the unique token hash, and required indexes. Add compile-time JSON tests proving VO JSON excludes `password_hash`, `token_hash`, and `family_id`.

```go
func TestAuthVOHidesSecrets(t *testing.T) {
    raw, err := json.Marshal(vo.AuthenticatedUserData{ID: 1, Email: "u@example.com"})
    if err != nil { t.Fatal(err) }
    for _, forbidden := range []string{"password", "token_hash", "family_id"} {
        if bytes.Contains(raw, []byte(forbidden)) { t.Fatalf("leaked %s", forbidden) }
    }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/unit/database ./internal/model/... -count=1 -v`

Expected: FAIL because `auth_sessions` and new model types do not exist.

- [ ] **Step 3: Add exact enum and DTO shapes**

```go
type VerificationPurpose string
const (
    VerificationPurposeRegister VerificationPurpose = "register"
    VerificationPurposeResetPassword VerificationPurpose = "reset_password"
)

type EmailRegisterRequest struct {
    VerificationTicket string `json:"verification_ticket" binding:"required"`
    Password string `json:"password" binding:"required"`
    DisplayName string `json:"display_name" binding:"required,max=80"`
}
```

Add these request types with explicit JSON/binding tags: `VerificationSendRequest`, `VerificationConfirmRequest`, `EmailRegisterRequest`, `EmailLoginRequest`, `PasswordResetRequest`, `TokenRefreshRequest`, and `LogoutRequest`. Add these public VO types with explicit JSON tags: `VerificationSendData`, `VerificationTicketData`, `AuthenticatedUserData`, `AuthTokenData`, `LoginData`, `SessionData`, and `OperationResultData`.

- [ ] **Step 4: Add schema and entities**

Add user timestamps and `auth_sessions` with foreign key cascade, unique `token_hash`, lookup indexes, and timestamps. Put `auth_sessions` before `users` in `tests/testutil/db.go` cleanup order.

- [ ] **Step 5: Verify GREEN and schema validation**

Run: `go test ./tests/unit/database ./internal/model/... -count=1 -v && bash scripts/validate-schema.sh && bash scripts/validate-architecture-boundaries.sh`

Expected: PASS and no `db/migrations` directory.

- [ ] **Step 6: Commit**

```bash
git add db/schema.sql internal/model tests/testutil/db.go tests/unit/database
git commit -m "feat: add authentication session models"
```

### Task 3: Implement Password, Digest, and Token Utilities

**Files:**
- Create: `internal/platform/security/password.go`
- Create: `internal/platform/security/digest.go`
- Create: `internal/platform/security/token.go`
- Create: `tests/unit/platform/security/password_test.go`
- Create: `tests/unit/platform/security/token_test.go`

**Interfaces:**
- Produces: `NormalizeEmail(string) (string, error)`, `ValidatePassword(string) error`, `HashPassword`, `ComparePassword`, `HMACDigest`, `SHA256Digest`, `NewRefreshToken`, `SignAccessToken`, `ParseAccessToken`.
- Consumers: verification, auth, and session services.

- [ ] **Step 1: Write failing table-driven security tests**

```go
func TestValidatePassword(t *testing.T) {
    cases := []struct{name, value string; valid bool}{
        {"valid", "example123", true},
        {"no digit", "abcdefgh", false},
        {"no letter", "12345678", false},
        {"too short", "abc123", false},
        {"bcrypt bytes", strings.Repeat("你", 25)+"a1", false},
    }
    for _, tc := range cases { t.Run(tc.name, func(t *testing.T) { if (security.ValidatePassword(tc.value) == nil) != tc.valid { t.Fatalf("valid mismatch") } }) }
}
```

JWT tests freeze algorithm, issuer, audience, 15-minute expiry, and reject wrong issuer/audience/algorithm.

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/unit/platform/security -count=1 -v`

Expected: FAIL because the security package does not exist.

- [ ] **Step 3: Implement minimal utilities**

Use `mail.ParseAddress`, lower-case normalized email, Unicode rune count plus the 72-byte bcrypt limit, `crypto/rand`, `crypto/hmac`, `sha256`, bcrypt, and typed JWT claims:

```go
type AccessClaims struct {
    SessionID int64 `json:"sid"`
    jwt.RegisteredClaims
}
```

Never normalize or trim passwords.

- [ ] **Step 4: Verify GREEN and race safety**

Run: `go test -race ./tests/unit/platform/security -count=1 -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/security tests/unit/platform/security
git commit -m "feat: add authentication security primitives"
```

### Task 4: Add Auth, SMTP, Cookie, and CORS Configuration

**Files:**
- Modify: `internal/config/server.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `.env.example`

**Interfaces:**
- Produces configuration for JWT issuer/audience, verification pepper, allowed origins, cookie security, and SMTP.
- Consumers: middleware and services in plans 1 and 2.

- [ ] **Step 1: Write failing config tests**

Assert production config rejects empty verification pepper, wildcard credentialed origin, short JWT secret, and empty SMTP authorization code; assert local test config accepts explicit non-production values.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/config -count=1 -v`

Expected: FAIL because fields and validations are absent.

- [ ] **Step 3: Add grouped config fields and env bindings**

```go
type AuthConfig struct {
    JWTSecret string `mapstructure:"JWT_SECRET"`
    JWTIssuer string `mapstructure:"JWT_ISSUER"`
    JWTAudience string `mapstructure:"JWT_AUDIENCE"`
    VerificationPepper string `mapstructure:"AUTH_VERIFICATION_PEPPER"`
    WebAllowedOrigins []string `mapstructure:"WEB_ALLOWED_ORIGINS"`
    CookieSecure bool `mapstructure:"AUTH_COOKIE_SECURE"`
}

type SMTPConfig struct {
    Host string `mapstructure:"SMTP_HOST"`
    Port int `mapstructure:"SMTP_PORT"`
    Username string `mapstructure:"SMTP_USERNAME"`
    AuthCode string `mapstructure:"SMTP_AUTH_CODE"`
    FromEmail string `mapstructure:"SMTP_FROM_EMAIL"`
    FromName string `mapstructure:"SMTP_FROM_NAME"`
}
```

Bind every env key explicitly and document 163 defaults without committing any credential.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/config -count=1 -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config .env.example
git commit -m "feat: configure secure authentication services"
```

### Task 5: Harden Middleware, CORS, 404, and 405

**Files:**
- Modify: `internal/platform/http/middleware.go`
- Modify: `internal/controller/route_controller.go`
- Test: `tests/integration/api/main_test.go`

**Interfaces:**
- Consumes: typed token parser and central ErrorCode registry.
- Produces: safe global middleware chain and explicit public-auth path registration.

- [ ] **Step 1: Write failing HTTP security tests**

Add cases for allowed/disallowed Origin, credentialed preflight, missing/invalid/wrong-audience JWT, `404`, `405`, and panic recovery. Assert every body uses the unified envelope and never contains internal cause text.

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/integration/api -run 'Test(CORS|JWT|NotFound|MethodNotAllowed|Recovery)' -count=1 -v`

Expected: FAIL because CORS currently returns wildcard plus credentials and 404/405 are not unified.

- [ ] **Step 3: Implement exact middleware behavior**

Replace permissive CORS with configured origin matching, `Vary: Origin`, allowed headers, and credentials. Register `NoRoute` and `NoMethod` responders. Parse typed JWT claims with fixed algorithm, issuer, audience, `exp`, and `nbf`.

Public paths include only health, Swagger/schema assets, and the seven explicit authentication endpoints; do not make all `/api/v1/auth/*` public by prefix.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./tests/integration/api ./internal/platform/http -count=1 -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/http internal/controller/route_controller.go tests/integration/api/main_test.go
git commit -m "feat: harden global http middleware"
```

### Task 6: Foundation Verification Gate

**Files:**
- Modify: files already listed in Tasks 1-5 when a verification failure proves the implementation does not match their declared interface; do not expand feature scope.

**Interfaces:**
- Produces a stable foundation consumed by the authentication-service plan.

- [ ] **Step 1: Generate and validate Swagger**

Run: `make swagger && bash scripts/validate-repository.sh`

Expected: generated docs match the unified envelope and repository validation passes.

- [ ] **Step 2: Run full Server verification**

Run: `go test -race ./... && go vet ./... && git diff --check`

Expected: PASS with zero race reports, vet findings, or whitespace errors.

- [ ] **Step 3: Review sensitive-data boundaries**

Run: `rg -n "password|verification|token|SMTP_AUTH_CODE" internal tests | rg "zap\.|Printf|Sprintf|Errorf"`

Expected: no log statement includes raw password, code, token, cookie, auth code, or complete email.

- [ ] **Step 4: Commit generated contract changes**

```bash
git add docs/docs.go docs/swagger.json docs/swagger.yaml
git commit -m "docs: publish authentication foundation contract"
```
