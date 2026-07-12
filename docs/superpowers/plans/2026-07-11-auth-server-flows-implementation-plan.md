# Email Authentication Server Flows Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Redis-backed email verification, PostgreSQL refresh sessions, 163 SMTP delivery, and the complete register/login/refresh/logout/password-reset API.

**Architecture:** Verification codes and one-time tickets use Redis atomic operations; durable users and refresh sessions use PostgreSQL transactions. Controllers remain transport-only, services own business orchestration, repositories own persistence, and the global ErrorCode/response layer from plan 1 owns external error rendering.

**Tech Stack:** Go 1.24, Gin, GORM, PostgreSQL, Redis/go-redis v9, TLS SMTP, Fx, Swaggo.

## Global Constraints

- Complete `2026-07-11-auth-foundation-implementation-plan.md` first.
- Follow `docs/design/018-邮箱认证与会话安全设计.md` exactly.
- Verification code: 6 digits, 10-minute TTL, 60-second resend lock, 5 sends/email/purpose/hour, 20 sends/IP/purpose/hour, invalid after 5 failed attempts.
- Access Token: 15 minutes. Refresh Session: sliding 7 days, absolute 30 days, rotate on every refresh.
- Password reset revokes every Refresh Session; already issued Access Tokens may remain valid for at most 15 minutes.
- SMTP uses `smtp.163.com:465`, implicit TLS, certificate verification, client authorization code, 5-second connect timeout, and 10-second send timeout.
- Redis verification operations fail closed; password login, access-token auth, refresh, and logout remain available when Redis is unavailable.
- Never return account existence from verification-send or password-reset request behavior.
- Apply TDD and commit after every task.

---

## File Map

**Create**

- `internal/repository/auth_session_repository.go`: transactional Session persistence and rotation.
- `internal/service/verification_service.go`: Redis verification/ticket/limit orchestration.
- `internal/service/session_service.go`: Session lifecycle and access/refresh token orchestration.
- `internal/platform/email/smtp_mailer.go`: 163 implicit-TLS mail transport.
- `internal/platform/email/template.go`: HTML/text authentication templates.
- `tests/testutil/redis.go`: test Redis setup/cleanup.
- `tests/testutil/fake/mailer.go`: deterministic mail capture.
- `tests/unit/verification/service_test.go`: verification behavior.
- `tests/unit/session/service_test.go`: Session behavior.
- `tests/unit/email/smtp_test.go`: TLS/mail serialization tests.

**Modify**

- `internal/repository/user_repository.go`: normalized email lookup, transactions, timestamps.
- `internal/service/auth_service.go`: verified registration, login, password reset.
- `internal/controller/auth_controller.go`: seven authentication endpoints.
- `internal/controller/swagger_response.go`: explicit unified wrappers.
- `internal/convert/auth_convert.go`: auth DTO-to-VO conversion.
- `internal/fxapp/app.go`: repositories/services/mailer wiring.
- `internal/controller/route_controller.go`: new dependencies/routes.
- `tests/testutil/router.go`: real auth stack with fake mailer and Redis.
- `tests/integration/api/main_test.go`: end-to-end auth scenarios.
- `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`: generated OpenAPI artifacts.

### Task 1: Implement Auth Session Repository

**Files:**
- Create: `internal/repository/auth_session_repository.go`
- Test: `tests/unit/database/auth_session_repository_test.go`

**Interfaces:**
- Produces: `CreateSession`, `RotateSession`, `RevokeSession`, `RevokeUserSessions`, `GetSession`.
- Consumers: `SessionService` and `AuthService`.

- [ ] **Step 1: Write failing repository integration tests**

Cover create, rotation, revoked-session rejection, idle expiry, absolute expiry, concurrent rotation with exactly one winner, and family revoke on reuse.

```go
func TestRotateSessionOnlyOneConcurrentWinner(t *testing.T) {
    db := testutil.SetupTestDB(t)
    repo := repository.NewAuthSessionRepo(db)
    session := seedSession(t, db, time.Now().Add(7*24*time.Hour), time.Now().Add(30*24*time.Hour))
    var successes atomic.Int32
    var wg sync.WaitGroup
    for range 2 {
        wg.Add(1)
        go func() {
            defer wg.Done()
            if _, err := repo.RotateSession(context.Background(), session.ID, session.TokenHash, "next-hash", time.Now()); err == nil { successes.Add(1) }
        }()
    }
    wg.Wait()
    if successes.Load() != 1 { t.Fatalf("successes=%d", successes.Load()) }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/unit/database -run AuthSession -count=1 -v`

Expected: FAIL because the repository does not exist.

- [ ] **Step 3: Implement repository with explicit transactions**

Use GORM transactions and `clause.Locking{Strength: "UPDATE"}`. `RotateSession` verifies current hash and time bounds before replacing the hash; distinguish not found, revoked, expired, and token mismatch with sentinel repository errors that services map to stable ErrorCodes.

- [ ] **Step 4: Verify GREEN with race detector**

Run: `go test -race ./tests/unit/database -run AuthSession -count=1 -v`

Expected: PASS and exactly one concurrent rotation winner.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/auth_session_repository.go tests/unit/database/auth_session_repository_test.go
git commit -m "feat: persist rotating authentication sessions"
```

### Task 2: Implement Redis Verification Store and Service

**Files:**
- Create: `internal/service/verification_service.go`
- Create: `tests/testutil/redis.go`
- Create: `tests/unit/verification/service_test.go`

**Interfaces:**
- Produces: `Send(ctx, VerificationSendInput)`, `Confirm(ctx, VerificationConfirmInput)`, `ClaimTicket`, `CompleteTicket`, `ReleaseTicket`.
- Consumes: Redis UniversalClient, HMAC digest helper, Mailer, clock, random-code generator, user existence query.

- [ ] **Step 1: Write failing verification tests**

Use a real disposable Redis configured by `TEST_REDIS_ADDR`; skip only when unavailable. Cover exact TTLs, separate purposes, 60-second lock, 5/hour email limit, 20/hour IP limit, fifth failure deletion, one-time ticket, unknown reset email generic success, and Redis failure closed.

```go
func TestFifthWrongCodeDeletesVerification(t *testing.T) {
    svc, store := newVerificationHarness(t)
    sendCode(t, svc, "user@example.com", enum.VerificationPurposeRegister)
    for i := 0; i < 5; i++ {
        _, _ = svc.Confirm(context.Background(), dto.VerificationConfirmInput{Email: "user@example.com", Purpose: enum.VerificationPurposeRegister, Code: "000000"})
    }
    if store.CodeExists(t, "user@example.com", enum.VerificationPurposeRegister) { t.Fatal("code still exists") }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/unit/verification -count=1 -v`

Expected: FAIL because verification service/store is absent.

- [ ] **Step 3: Implement atomic scripts and key derivation**

Keep complete email addresses out of Redis keys. Implement Lua scripts for send reservation, failed-attempt increment/delete, code consume plus Ticket issue, Ticket claim, completion, and release.

Represent Ticket state explicitly:

```go
type ticketState struct {
    Email string `json:"email"`
    Purpose enum.VerificationPurpose `json:"purpose"`
    Status string `json:"status"` // ready, processing, completed
}
```

Values may contain the normalized email because Redis is protected application storage; keys and logs must remain HMAC-derived/masked. Apply 10-minute TTL to ready Tickets and a short 2-minute replay marker to completed Tickets.

- [ ] **Step 4: Implement send semantics**

Write code state before SMTP. Delete it on definite SMTP failure. Preserve it and charge one quota unit on ambiguous timeout. Return the same accepted DTO when registration email exists or reset email does not exist.

- [ ] **Step 5: Verify GREEN**

Run: `go test -race ./tests/unit/verification -count=1 -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/verification_service.go tests/testutil/redis.go tests/unit/verification/service_test.go
git commit -m "feat: add redis email verification flow"
```

### Task 3: Implement 163 SMTP Mailer and Templates

**Files:**
- Create: `internal/platform/email/smtp_mailer.go`
- Create: `internal/platform/email/template.go`
- Create: `tests/testutil/fake/mailer.go`
- Create: `tests/unit/email/smtp_test.go`

**Interfaces:**
- Produces: `Mailer.Send(ctx, Message) error`, `RegistrationCodeMessage`, `PasswordResetCodeMessage`, `PasswordChangedMessage`.
- Consumers: VerificationService and AuthService.

- [ ] **Step 1: Write failing template and transport tests**

Assert HTML escaping, plain-text alternative, no password/Token/Ticket, code and 10-minute copy, TLS `ServerName=smtp.163.com`, and context deadlines.

```go
func TestTemplateEscapesDisplayName(t *testing.T) {
    msg := email.PasswordChangedMessage("u@example.com", `<script>alert(1)</script>`)
    if strings.Contains(msg.HTML, "<script>") { t.Fatal("unescaped html") }
    if msg.Text == "" { t.Fatal("missing text alternative") }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/unit/email -count=1 -v`

Expected: FAIL because the email package does not exist.

- [ ] **Step 3: Implement implicit-TLS SMTP**

Dial with a 5-second timeout, wrap with `tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}`, authenticate with the 163 authorization code, write a multipart/alternative message, and enforce the 10-second operation context. Do not expose credentials in returned errors.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./tests/unit/email -count=1 -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/email tests/testutil/fake/mailer.go tests/unit/email
git commit -m "feat: send secure 163 authentication email"
```

### Task 4: Implement Session Service

**Files:**
- Create: `internal/service/session_service.go`
- Create: `tests/unit/session/service_test.go`

**Interfaces:**
- Produces: `Create`, `Refresh`, `Logout`, `RevokeAll`, `SessionTokens`.
- Consumes: AuthSessionRepository, token utilities, clock.
- Consumers: AuthService and auth controller.

- [ ] **Step 1: Write failing Session tests**

Cover 15-minute Access Token, 7-day idle/30-day absolute timestamps, rotation, logout idempotency, expired Session, revoked Session, old-token reuse family revoke, and no 7-day extension past absolute expiry.

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/unit/session -count=1 -v`

Expected: FAIL because SessionService does not exist.

- [ ] **Step 3: Implement SessionService with injectable clock**

```go
type SessionService struct {
    repo AuthSessionRepository
    tokens TokenManager
    now func() time.Time
}

type SessionTokens struct {
    AccessToken string
    AccessExpiresAt time.Time
    RefreshToken string
    RefreshExpiresAt time.Time
}
```

Hash refresh tokens before repository calls. On token mismatch/reuse, revoke the family and return `AUTH_TOKEN_REUSED`.

- [ ] **Step 4: Verify GREEN**

Run: `go test -race ./tests/unit/session -count=1 -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/session_service.go tests/unit/session
git commit -m "feat: manage rotating refresh sessions"
```

### Task 5: Refactor Auth Service Around Verified Registration

**Files:**
- Modify: `internal/service/auth_service.go`
- Modify: `internal/repository/user_repository.go`
- Modify: `tests/unit/auth/service_test.go`
- Modify: `tests/testutil/fake/auth/repo.go`

**Interfaces:**
- Produces: `RegisterVerified`, `Login`, `ResetPassword`, `CurrentUser`.
- Consumes: VerificationService, SessionService, UserRepository, password utility, transaction boundary, Mailer.

- [ ] **Step 1: Replace old auth tests with failing target-flow tests**

Cover normalized email, valid Ticket requirement, duplicate race, password policy, direct authenticated registration result, dummy bcrypt compare for unknown email, disabled account generic credentials error, password reset all-session revoke, and security-email failure not rolling back password.

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/unit/auth -count=1 -v`

Expected: FAIL because the old service directly registers email/password.

- [ ] **Step 3: Implement repository transaction hooks**

Add transaction-aware user create/update methods, unique-violation detection, `email_verified_at`, `password_changed_at`, and `last_login_at`. Keep repository return values as DTO, never VO.

- [ ] **Step 4: Implement service orchestration**

Registration sequence: claim Ticket, database transaction creates user and Session, commit, complete Ticket. Database failure releases Ticket. Login performs dummy bcrypt compare when the email is absent. Reset transaction updates password and revokes every Session; notification mail occurs after commit.

- [ ] **Step 5: Verify GREEN**

Run: `go test -race ./tests/unit/auth -count=1 -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/auth_service.go internal/repository/user_repository.go tests/unit/auth tests/testutil/fake/auth
git commit -m "feat: implement verified email authentication"
```

### Task 6: Expose Complete Auth HTTP API

**Files:**
- Modify: `internal/controller/auth_controller.go`
- Modify: `internal/controller/swagger_response.go`
- Modify: `internal/convert/auth_convert.go`
- Modify: `internal/controller/route_controller.go`
- Modify: `internal/fxapp/app.go`
- Modify: `tests/testutil/router.go`
- Modify: `tests/integration/api/main_test.go`

**Interfaces:**
- Produces the eight routes and exact OpenAPI contracts from design 018.
- Consumes all services from Tasks 2-5 and the global response/error foundation.

- [ ] **Step 1: Write failing end-to-end HTTP tests**

Test send -> confirm -> register -> me -> refresh -> logout; login; reset -> old refresh rejected; cookie attributes; generic unknown-email response; bad Origin; rate limits; exact ErrorCodes and envelopes.

Use an `http.Client` with `cookiejar.Jar` so Refresh Cookie behavior is exercised rather than manually injecting a header.

- [ ] **Step 2: Verify RED**

Run: `go test ./tests/integration/api -run Auth -count=1 -v`

Expected: FAIL because the target routes do not exist.

- [ ] **Step 3: Implement transport-only handlers**

Handlers bind DTO, call service, call `convert.*DTOToVO`, set/clear the refresh Cookie through one helper, and report errors with `c.Error(err)`. No handler switches over domain errors or returns `err.Error()`.

Register:

```text
POST /api/v1/auth/verifications
POST /api/v1/auth/verifications/confirm
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/auth/token/refresh
GET  /api/v1/auth/me
POST /api/v1/auth/logout
POST /api/v1/auth/password/reset
```

- [ ] **Step 4: Wire Fx dependencies**

Provide AuthSessionRepository, SMTP Mailer, VerificationService, SessionService, and revised AuthService. Reuse `module.NewRedis`; do not create a second Redis client in auth wiring.

- [ ] **Step 5: Verify GREEN**

Run: `go test -race ./tests/integration/api -run Auth -count=1 -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/controller internal/convert/auth_convert.go internal/fxapp/app.go tests/testutil/router.go tests/integration/api/main_test.go
git commit -m "feat: expose secure email authentication api"
```

### Task 7: Publish Contract and Run Server Acceptance

**Files:**
- Modify generated: `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`
- Modify: files already listed in Tasks 1-6 when a verification failure proves the implementation does not match their declared interface; do not expand feature scope.

**Interfaces:**
- Produces the stable OpenAPI contract consumed by hotkey-web.

- [ ] **Step 1: Generate Swagger**

Run: `make swagger`

Expected: generated auth paths, DTO schemas, unified responses, and Bearer security definitions are present.

- [ ] **Step 2: Validate repository and race tests**

Run: `bash scripts/validate-repository.sh && go test -race ./... && go vet ./... && git diff --check`

Expected: PASS.

- [ ] **Step 3: Run real dependency smoke**

Start local PostgreSQL and Redis, configure a test SMTP server, then run: `make smoke`

Expected: health, verification, register, login, refresh, authenticated monitor access, logout, and password-reset scenarios pass.

- [ ] **Step 4: Inspect generated contract and logs**

Run: `rg -n 'verifications|password/reset|token/refresh|AUTH_INVALID_CREDENTIALS' docs/swagger.json && ! rg -n 'Passw0rd|123456|hk_refresh|SMTP_AUTH_CODE' . -g '*.log'`

Expected: contract entries exist; no sensitive sample values appear in logs.

- [ ] **Step 5: Commit generated artifacts**

```bash
git add docs/docs.go docs/swagger.json docs/swagger.yaml
git commit -m "docs: publish email authentication api contract"
```
