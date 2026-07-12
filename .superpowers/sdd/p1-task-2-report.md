# Task 2: Add DTO, Enum, Entity, and Schema Primitives

## Summary

Added authentication-related enum types, DTOs, entities, VOs, and database schema for user authentication sessions. Implemented TDD: RED (failing tests) -> GREEN (all passing).

## Changes

### Created files

- `internal/model/enum/auth.go` -- `VerificationPurpose` (register, reset_password), `AccountStatus` (active, disabled, unverified), `SessionRevokeReason` (logout, password_reset, token_reuse, admin)
- `internal/model/dto/common_request.go` -- `PageRequest`, `IDRequest`, `DeleteRequest` with JSON/form/binding tags
- `internal/model/entity/auth_session.go` -- `AuthSession` with GORM tags, indexes, foreign key to users
- `tests/unit/database/auth_schema_test.go` -- Schema validation tests and auth VO secret-hiding test

### Modified files

- `internal/model/entity/user.go` -- Added `VerificationStatus`, `EmailVerifiedAt`, `PasswordChangedAt`, `LastLoginAt` fields
- `internal/model/dto/auth.go` -- Added `VerificationSendInput`, `VerificationConfirmInput`, `TokenRefreshInput`, `PasswordResetInput`
- `internal/model/dto/auth_request.go` -- Added `VerificationSendRequest`, `VerificationConfirmRequest`, `EmailRegisterRequest`, `EmailLoginRequest`, `PasswordResetRequest`, `TokenRefreshRequest`, `LogoutRequest` with JSON/binding tags
- `internal/model/vo/auth.go` -- Added `VerificationSendData`, `VerificationTicketData`, `AuthenticatedUserData`, `AuthTokenData`, `SessionData`, `OperationResultData` (JSON tags, no secrets exposed)
- `db/schema.sql` -- Added `verification_status`, `email_verified_at`, `password_changed_at`, `last_login_at` columns to `users`; created `auth_sessions` table with FK cascade, unique token_hash, indexes
- `tests/testutil/db.go` -- Added `auth_sessions` before `users` in cleanTables order

## Test results

- `go test ./tests/unit/database/... -v -count=1` -- all 11 tests PASS
- `go test ./internal/model/... -v -count=1` -- all pass (no test files, compilation OK)
- `make lint` -- PASS
- `make test` -- all tests PASS
- `scripts/validate-schema.sh` -- PASS (30 tables validated)

## Code review fixes (commit 7f92625)

Applied fixes for findings from the Task 2 code review:

**Critical: TestAuthVOHidesSecrets was vacuous**
- File: `tests/unit/database/auth_schema_test.go` (TestAuthVOHidesSecrets)
- Root cause: The test used an anonymous struct (`struct { ID int64; Email string }`) instead of the actual `vo.AuthenticatedUserData` type, so it was not actually testing the VO types. It passed vacuously because the anonymous struct could never contain the forbidden fields.
- Fix: Replaced with `json.Marshal(vo.AuthenticatedUserData{ID: 1, Email: "u@example.com"})` so the test genuinely confirms that password_hash, token_hash, and family_id are excluded from the VO's JSON output.

**Important: AuthSession.UserID missing GORM FK cascade constraint**
- File: `internal/model/entity/auth_session.go` (AuthSession.UserID)
- Root cause: The GORM tag had `not null;index:idx_auth_sessions_user_id` but no `constraint:OnDelete:CASCADE`, while the SQL schema specifies `on delete cascade`. If GORM were used for schema migration (auto-migrate), the FK would lack cascade behavior.
- Fix: Added `constraint:OnDelete:CASCADE` to the GORM tag.

**Important: AuthSession.LastRefreshedAt would bypass SQL default now()**
- File: `internal/model/entity/auth_session.go` (AuthSession.LastRefreshedAt)
- Root cause: The field was `time.Time` (non-pointer) with `not null` GORM tag. On insert, GORM always writes the Go zero-value `time.Time{}` (0001-01-01), overriding the SQL `default now()`. Only pointer (`*time.Time`) fields are skipped by GORM when nil, allowing the DB default to apply.
- Fix: Changed to `*time.Time` and removed `not null` from the GORM tag.

**Verification:**
- `go test ./tests/unit/database/... -v -count=1` -- all 11 tests PASS
- `make lint` -- PASS
- `bash scripts/validate-schema.sh` -- PASS
- `bash scripts/validate-architecture-boundaries.sh` -- PASS
