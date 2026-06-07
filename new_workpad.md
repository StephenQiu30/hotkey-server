## Claude Workpad

```text
StephenQius-MacBook-Air-2.local:/Users/stephenqiu/Desktop/StephenQiu30/Agents/symphony/workspaces/STE-240@5a247fc
```

### Plan

- [x] 1. Normalize service — multi-platform content normalization (language detect, text clean, URL normalize)
- [x] 2. Filter service — keyword/exclusion word filtering with reason tracking
- [x] 3. Dedup service — exact/canonical/near-duplicate detection and merge
- [x] 4. Quality scoring — content quality score and summarizability marking
- [x] 5. Embedding retry — embedding provider failure recovery with retry support
- [x] 6. Wire into ingest pipeline — integrate normalize/filter/dedup into existing ingest flow
- [x] 7. Postgres repository — extend contentrepo/hotspotrepo with quality fields

### Acceptance Criteria

- [x] 多平台重复内容只保留一个主记录
- [x] 无关内容被过滤并记录原因
- [x] 相关内容进入事件聚类
- [x] embedding provider 临时失败后任务可重试
- [x] 语言识别和正文清洗正确
- [x] 关键词/排除词过滤生效
- [x] embedding 相似度 near-duplicate 合并

### Test-first Evidence

- [x] Red: `df0ac19` — 添加内容标准化、过滤、去重、质量评分和 embedding 重试的失败测试
- [x] Green: `8f0fe4e` — 实现内容标准化、过滤、去重、质量评分和 embedding 重试服务

### Commit Plan

- [x] `test:` `df0ac19` — 47+ failing tests for all new services
- [x] `impl:` `8f0fe4e` — minimal implementation to pass tests
- [x] `impl:` `ebf7951` — extend SourceItem quality fields + ListEmbeddings
- [x] `test:` `3761e6b` — fix quality scoring test data
- [x] `impl:` `906edb4` — fix dedup interface method names
- [x] `test:` `f81fc43` — fix postgres contentrepo tests + pipeline integration
- [x] `impl:` `5a247fc` — postgres repository quality fields + pipeline quality scoring

### Validation

- [x] `go test ./internal/service/normalize/...` — PASS (12 tests)
- [x] `go test ./internal/service/filter/...` — PASS (7 tests)
- [x] `go test ./internal/service/dedup/...` — PASS (8 tests)
- [x] `go test ./internal/service/quality/...` — PASS (6 tests)
- [x] `go test ./internal/service/embedding/...` — PASS (6 tests)
- [x] `go test ./internal/service/ingest/...` — PASS (8 tests)
- [x] `go test ./...` — PASS (all packages)

### Notes

- 2026-06-07: Implementation complete. All 6 services + postgres integration done.
- PR: https://github.com/StephenQiu30/hotkey-server/pull/150
- Branch: `feature/ste-240-content-normalize-dedup`
- 52+ tests passing across all service packages
- Normalize: HTML strip, language detect (zh/en/unknown), URL canonicalize, truncation
- Filter: keyword/exclusion/length with precedence rules
- Dedup: exact hash + near-duplicate cosine similarity (threshold 0.92)
- Quality: 5-dimension weighted scoring (title 0.2, snippet 0.35, url 0.15, lang 0.15, time 0.15)
- Embedding: retry with config error short-circuit, audit persistence
- Ingest: normalize → filter → quality → dedup → persist → enqueue embedding

### Agent Review

- [x] Status: `completed`
- Reviewer: `gemini`
- Findings:
  - [x] `internal/service/dedup`: Integrated into `GenerateEmbeddingHandler`. Near-duplicate detection now runs after embedding generation.
  - [x] `internal/repository/postgres/hotspotrepo`: Implemented `SearchSimilar` using pgvector.
  - [x] `internal/service/dedup`: Replaced O(N) `ListEmbeddings` with scalable `SearchSimilar`.
  - [x] `cmd/hotkey-api/main.go` & `internal/app`: All services and handlers wired into production entry points using Postgres repositories.
  - [x] `internal/worker/worker.go`: Worker now correctly fails jobs when no handler is registered.
- Verification provided:
  - [x] Integration test `internal/worker/dedup_integration_test.go` confirms item is marked as `duplicate` based on similarity.
  - [x] `main.go` updated with full wiring of services and handlers.
  - [x] `SearchSimilar` implemented in both Postgres and Memory repositories.

### Validation

- [x] `go build ./cmd/hotkey-api` — PASS
- [x] `go test ./internal/service/dedup/...` — PASS
- [x] `go test ./internal/worker/...` — PASS
- [x] `go test ./...` — PASS (all packages)

### Confusions

- (none)
