# Proposal: Add Runtime API Smoke Test to Validation Gate

## Problem

`scripts/validate-repository.sh` passes without catching three runtime failures:
1. `POST /api/v1/auth/register` returns empty user fields (stubAuthRepo returns zero-value User)
2. `GET /api/v1/monitors` always 401 (authMiddleware hardcoded to reject)
3. `/api/v1/monitors/{id}/posts|topics|trends` return 404 (PostHandler/TopicHandler/TrendHandler not wired in main.go)

## Goal

Add a main-program-level smoke test that builds and starts the server, then validates critical API endpoints respond correctly. Fix the wiring issues that cause the three failures.

## Non-Goal

- Full JWT authentication implementation
- Real database integration for stubs
- Comprehensive API test suite

## Approach

1. Fix `stubAuthRepo.Create` to return a User with populated fields
2. Add `SMOKE_TEST` env bypass to authMiddleware for testing
3. Wire PostHandler, TopicHandler, TrendHandler in main.go
4. Write `scripts/smoke-api.sh` that validates endpoints
5. Integrate smoke test into `scripts/validate-repository.sh`

## Verification

- `scripts/smoke-api.sh` passes against a locally-started server
- `bash scripts/validate-repository.sh` includes smoke test execution
