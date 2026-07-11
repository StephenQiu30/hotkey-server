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
