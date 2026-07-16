---
layer: Operations
doc_no: "009"
audience: [Dev, QA, Ops]
feature_area: 数据库升级
purpose: 将 PLAN-008 AI 模型 Profile 契约受控升级为支持 relevance_review
canonical_path: docs/operations/plan009-schema-upgrade.md
status: review
version: v0.1
owner: HotKey Server Team
inputs:
  - db/schema.sql
  - docs/plans/009-多语言相关性匹配与反馈计划.md
outputs:
  - 可恢复的 relevance_review Profile Schema 升级流程
triggers:
  - 已由 PLAN-008 Schema 初始化的数据库进入 PLAN-009 Task 1
downstream:
  - docs/acceptance/009-多语言相关性匹配与反馈验收.md
---

# PLAN-009 Task 1 Schema 升级

## 范围与停止条件

本手册只覆盖 Task 1 对 `ai_model_profiles` 的兼容性变更：新增 OpenAI 专用的 `relevance_review` 任务类型，并禁止它写入 embedding 维度。它以 commit `a7fc805`（PLAN-008 archived/done）为唯一历史基线；新环境始终使用当前 release 的 `go run ./cmd/hotkey db init --empty-only --confirm-empty`，不得运行本手册。

升级会短暂获取 `ai_model_profiles` 的 DDL 锁。先停止所有创建或修改模型 Profile 的 API/worker；任何 backup、preflight、DDL、verify 或 restore 失败时，保持服务停止并进入“回退”。本次变更不重写现有 AI run、账本或 embedding 行；任一既有 Profile 不符合 PLAN-008 语义时停止处理，不能借此迁移猜测或修复数据。

## 备份与只读 preflight

`HOTKEY_DATABASE_URL` 指向维护窗口中的目标数据库。操作者须有 `pg_dump`、`pg_restore`、Git worktree 与 DDL 权限；dump 必须保存在受保护位置且绝不提交。先从固定历史 worktree 验证 backup 的可恢复基线：

```bash
export PLAN008_BASELINE=a7fc805
export PLAN008_WORKTREE="$(mktemp -d /tmp/hotkey-plan009-plan008.XXXXXX)"
git worktree add --detach "$PLAN008_WORKTREE" "$PLAN008_BASELINE"
trap 'git worktree remove --force "$PLAN008_WORKTREE"' EXIT

pg_dump "$HOTKEY_DATABASE_URL" --format=custom --file=/secure-backups/hotkey-before-plan009-task1.dump
pg_restore --list /secure-backups/hotkey-before-plan009-task1.dump
HOTKEY_DATABASE_URL="$HOTKEY_DATABASE_URL" go -C "$PLAN008_WORKTREE" run ./cmd/hotkey db verify

# 保存受保护的 monitor_matches 还原清单；回退后必须逐字节相同。
psql "$HOTKEY_DATABASE_URL" --csv -c "
  SELECT id, monitor_id, monitor_config_version_id, content_id, rule_score,
         semantic_score, llm_score, final_score, decision, reason_codes,
         manual_locked, algorithm_version
  FROM monitor_matches
  ORDER BY id" > /secure-backups/hotkey-before-plan009-task1-monitor-matches.csv
```

以上命令全部成功后，执行只读 preflight。两个计数都必须为 `0`；否则停止，不得篡改 Profile 后继续升级：

```sql
SELECT
  (SELECT count(*) FROM ai_model_profiles
   WHERE task_type NOT IN ('embedding', 'term_expansion')) AS unsupported_tasks,
  (SELECT count(*) FROM ai_model_profiles
   WHERE (task_type = 'embedding' AND embedding_dimensions IS DISTINCT FROM 1024)
      OR (task_type = 'term_expansion' AND embedding_dimensions IS NOT NULL)) AS invalid_dimensions;
```

## 受控升级

只能整体执行下列 transaction，不能拆分或手工替换约束。它同时支持通过 PLAN-008 新库 Schema 创建的默认约束名，以及 PLAN-008 历史升级手册使用的单数约束名。

```bash
psql "$HOTKEY_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;

ALTER TABLE ai_model_profiles
  DROP CONSTRAINT IF EXISTS ai_model_profiles_task_type_check,
  DROP CONSTRAINT IF EXISTS ai_model_profiles_check1,
  DROP CONSTRAINT IF EXISTS ai_model_profiles_embedding_dimensions_check,
  DROP CONSTRAINT IF EXISTS ai_model_profiles_embedding_dimension_check;

ALTER TABLE ai_model_profiles
  ADD CONSTRAINT ai_model_profiles_task_type_check
    CHECK (task_type IN ('embedding', 'term_expansion', 'relevance_review')),
  ADD CONSTRAINT ai_model_profiles_embedding_dimensions_check CHECK (
    (task_type = 'embedding' AND embedding_dimensions = 1024)
    OR (task_type IN ('term_expansion', 'relevance_review') AND embedding_dimensions IS NULL)
  );

COMMIT;
SQL

go run ./cmd/hotkey db verify
```

验证完成后，管理员可创建使用 `gpt-5.6sol`、`env:OPENAI_API_KEY` 且不带 `embedding_dimensions` 的 `relevance_review` Profile。ONNX 或任何 embedding dimension 都必须被拒绝。

## 回退

Task 1 仅替换检查约束，没有新增表、列、索引或数据回填。若升级后的 verifier 或受控演练失败，保持服务停止，直接恢复上述 upgrade 前 custom dump；不要反向手写 Profile DDL。PostgreSQL 18 的 `pg_restore` 会将 PLAN-008 的 `ai_runs_reuse_inflight_uq` predicate 以等价、但旧版严格 catalog verifier 不接受的括号形式重建。因此 restore 后必须按固定 PLAN-008 定义重建该索引，之后才运行固定 worktree verifier：

```bash
pg_restore --single-transaction --clean --if-exists --no-owner \
  --dbname="$HOTKEY_DATABASE_URL" /secure-backups/hotkey-before-plan009-task1.dump

psql "$HOTKEY_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;

DROP INDEX IF EXISTS ai_runs_reuse_inflight_uq;
CREATE UNIQUE INDEX ai_runs_reuse_inflight_uq
  ON ai_runs(reuse_key)
  WHERE status IN ('queued','running','validating','retry_wait');

COMMIT;
SQL

HOTKEY_DATABASE_URL="$HOTKEY_DATABASE_URL" go -C "$PLAN008_WORKTREE" run ./cmd/hotkey db verify

psql "$HOTKEY_DATABASE_URL" --csv -c "
  SELECT id, monitor_id, monitor_config_version_id, content_id, rule_score,
         semantic_score, llm_score, final_score, decision, reason_codes,
         manual_locked, algorithm_version
  FROM monitor_matches
  ORDER BY id" > /secure-backups/hotkey-after-rollback-monitor-matches.csv
cmp /secure-backups/hotkey-before-plan009-task1-monitor-matches.csv \
  /secure-backups/hotkey-after-rollback-monitor-matches.csv
```

`cmp` 必须成功；否则历史 `monitor_matches` 没有按 backup 精确恢复，保持服务停止。只有 catalog verifier 与数据清单都成功后才能恢复 PLAN-008 服务。演练证据由 `TestPlan009SchemaUpgradeAndRollbackUsesPinnedPlan008Worktree` 保存：它真实初始化固定 PLAN-008 worktree、写入非空 `monitor_matches` 历史、运行本文 transaction、验证目标 Schema 和历史行、写入合法/非法 Profile，再还原 backup 并使用同一历史 verifier 和关键历史行复核。
