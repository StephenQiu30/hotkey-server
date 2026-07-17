---
layer: Operations
doc_no: "009"
audience: [Dev, QA, Ops]
feature_area: 数据库升级
purpose: 将 PLAN-008 的模型 Profile 与监控匹配关系升级为可审计的相关性快照、反馈、建议和有界召回索引
canonical_path: docs/operations/plan009-schema-upgrade.md
status: accepted
version: v0.4
owner: HotKey Server Team
inputs:
  - db/schema.sql
  - docs/plans/archive/009-多语言相关性匹配与反馈计划.md
outputs:
  - 可恢复的 relevance_review 与相关性持久化 Schema 升级流程
triggers:
  - 已由 PLAN-008 Schema 初始化的数据库进入 PLAN-009 Task 1、Task 2 或 Task 3
downstream:
  - docs/acceptance/archive/009-多语言相关性匹配与反馈验收.md
---

# PLAN-009 Schema 升级

## 最终状态

PLAN-009 的 Schema 升级、回退和最终 integration race 已由 Acceptance-009 在 `d4efda5` 基线验收并经独立复审 APPROVED；本手册为已接受的可重复升级流程，不代表任何生产库已执行升级。

## 范围与停止条件

本手册以 commit `a7fc805`（PLAN-008 archived/done）为唯一历史基线，一次性升级两类契约：

- `ai_model_profiles` 新增仅 OpenAI 可用、无 embedding 维度的 `relevance_review`；
- `monitor_matches` 由当前关系升级为输入快照，并新增反馈和待审核建议事实。
- `monitor_sources` 与已批准 `monitor_rules` 新增有界 source/lexical 候选倒排索引。

新环境始终使用当前 release 的 `go run ./cmd/hotkey db init --empty-only --confirm-empty`，不得运行本手册。升级会获取 `ai_model_profiles`、`monitor_matches` 的 DDL 锁；先停止相关 API/worker。任何 backup、preflight、DDL、verify 或 restore 失败时，保持服务停止并进入“回退”。不要手工拼接部分 DDL 或修复数据后继续。

## 备份与只读 preflight

`HOTKEY_DATABASE_URL` 指向维护窗口中的目标数据库。操作者须有 `pg_dump`、`pg_restore`、Git worktree 与 DDL 权限；dump、CSV 仅保存在受保护位置且绝不提交。

```bash
export PLAN008_BASELINE=a7fc805
export PLAN008_WORKTREE="$(mktemp -d /tmp/hotkey-plan009-plan008.XXXXXX)"
git worktree add --detach "$PLAN008_WORKTREE" "$PLAN008_BASELINE"
trap 'git worktree remove --force "$PLAN008_WORKTREE"' EXIT

pg_dump "$HOTKEY_DATABASE_URL" --format=custom --file=/secure-backups/hotkey-before-plan009.dump
pg_restore --list /secure-backups/hotkey-before-plan009.dump
HOTKEY_DATABASE_URL="$HOTKEY_DATABASE_URL" go -C "$PLAN008_WORKTREE" run ./cmd/hotkey db verify

# 回退后的历史 monitor_matches 必须逐字节恢复。
psql "$HOTKEY_DATABASE_URL" --csv -c "
  SELECT id, monitor_id, monitor_config_version_id, content_id, rule_score,
         semantic_score, llm_score, final_score, decision, reason_codes,
         manual_locked, algorithm_version
  FROM monitor_matches
  ORDER BY id" > /secure-backups/hotkey-before-plan009-monitor-matches.csv
```

随后执行只读 preflight。任一计数非零即停止：Profile 不应在本次迁移中被猜测修复，旧 explanation 必须已经是 JSON object，且若存在 `provenance`，它也必须是 object，才能安全追加 legacy 标记。

```sql
SELECT
  (SELECT count(*) FROM ai_model_profiles
   WHERE task_type NOT IN ('embedding', 'term_expansion')) AS unsupported_tasks,
  (SELECT count(*) FROM ai_model_profiles
   WHERE (task_type = 'embedding' AND embedding_dimensions IS DISTINCT FROM 1024)
      OR (task_type = 'term_expansion' AND embedding_dimensions IS NOT NULL)) AS invalid_dimensions,
  (SELECT count(*) FROM monitor_matches
   WHERE jsonb_typeof(explanation) IS DISTINCT FROM 'object') AS non_object_explanations,
  (SELECT count(*) FROM monitor_matches
   WHERE explanation ? 'provenance'
     AND jsonb_typeof(explanation->'provenance') IS DISTINCT FROM 'object') AS non_object_provenance;
```

## 受控升级

只能整体执行下列 transaction。对历史行，`input_hash` 使用 `legacy-monitor-match-v1` 加原始 ID/版本的确定性 SHA-256；`scoring_version=legacy-v1`、`degraded=true`，并只追加 `provenance.legacy_backfill=true`，不改写旧分数、decision、reason code 或算法版本。

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

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM monitor_matches
        WHERE jsonb_typeof(explanation) IS DISTINCT FROM 'object'
           OR explanation ? 'provenance'
              AND jsonb_typeof(explanation->'provenance') IS DISTINCT FROM 'object'
    ) THEN
        RAISE EXCEPTION 'PLAN-009 requires monitor_matches explanation and provenance to be JSON objects';
    END IF;
END;
$$;

ALTER TABLE monitor_matches
  ADD COLUMN input_hash char(64),
  ADD COLUMN scoring_version varchar(64),
  ADD COLUMN recall_paths text[] NOT NULL DEFAULT '{}',
  ADD COLUMN degraded boolean NOT NULL DEFAULT false,
  ADD COLUMN decision_origin varchar(16) NOT NULL DEFAULT 'rule',
  ADD COLUMN embedding_model_profile_id bigint,
  ADD COLUMN embedding_model_profile_version bigint,
  ADD COLUMN embedding_model_version varchar(64),
  ADD COLUMN review_ai_run_id bigint;

UPDATE monitor_matches
SET input_hash = encode(
      sha256(convert_to(concat_ws(':', 'legacy-monitor-match-v1', id, monitor_config_version_id, content_id, algorithm_version), 'UTF8')),
      'hex'
    ),
    scoring_version = 'legacy-v1',
    degraded = true,
    decision_origin = 'rule',
    explanation = jsonb_set(
      explanation,
      '{provenance}',
      COALESCE(explanation->'provenance', '{}'::jsonb) || '{"legacy_backfill":true}'::jsonb,
      true
    );

ALTER TABLE monitor_matches
  ALTER COLUMN input_hash SET NOT NULL,
  ALTER COLUMN scoring_version SET NOT NULL,
  DROP CONSTRAINT IF EXISTS monitor_matches_monitor_config_version_id_content_id_key,
  ADD CHECK (decision_origin IN ('rule','ai')),
  ADD CHECK (jsonb_typeof(explanation) = 'object'),
  ADD CHECK (
    embedding_model_profile_id IS NULL AND embedding_model_profile_version IS NULL AND embedding_model_version IS NULL
    OR embedding_model_profile_id IS NOT NULL AND embedding_model_profile_version IS NOT NULL AND embedding_model_version IS NOT NULL
  ),
  ADD FOREIGN KEY (embedding_model_profile_id) REFERENCES ai_model_profiles(id) ON DELETE RESTRICT,
  ADD FOREIGN KEY (review_ai_run_id) REFERENCES ai_runs(id) ON DELETE RESTRICT,
  ADD UNIQUE (monitor_config_version_id, content_id, input_hash, scoring_version),
  ADD UNIQUE (id, monitor_config_version_id, content_id);

DROP INDEX IF EXISTS monitor_matches_list_idx;
CREATE INDEX monitor_matches_latest_snapshot_idx
  ON monitor_matches(monitor_config_version_id, content_id, created_at DESC, id DESC);
CREATE INDEX monitor_matches_list_idx
  ON monitor_matches(monitor_id, decision, final_score DESC, id DESC);
CREATE INDEX monitor_matches_content_active_idx
  ON monitor_matches(content_id, monitor_id, final_score DESC, id DESC);

CREATE INDEX monitor_sources_relevance_active_source_idx
  ON monitor_sources(source_connection_id, priority, config_version_id)
  WHERE enabled;
CREATE INDEX monitor_rules_relevance_approved_lexical_idx
  ON monitor_rules(lower(value), config_version_id, origin, weight DESC, id)
  WHERE enabled AND approval_status = 'approved'
    AND rule_type IN ('keyword','phrase','entity','exclude_keyword');

CREATE TABLE monitor_match_feedbacks (
    id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY, version bigint NOT NULL DEFAULT 1,
    monitor_id bigint NOT NULL, monitor_config_version_id bigint NOT NULL,
    content_id bigint NOT NULL REFERENCES contents(id) ON DELETE RESTRICT,
    monitor_match_id bigint, actor_user_id bigint NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    feedback_type varchar(16) NOT NULL CHECK (feedback_type IN ('relevant','irrelevant','false_positive','false_negative')),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (monitor_config_version_id, monitor_id) REFERENCES monitor_config_versions(id, monitor_id) ON DELETE RESTRICT,
    FOREIGN KEY (monitor_match_id, monitor_config_version_id, content_id)
      REFERENCES monitor_matches(id, monitor_config_version_id, content_id) ON DELETE RESTRICT,
    UNIQUE (monitor_config_version_id, content_id, actor_user_id)
);

CREATE TABLE monitor_feedback_suggestions (
    id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY, version bigint NOT NULL DEFAULT 1,
    monitor_id bigint NOT NULL, monitor_config_version_id bigint NOT NULL,
    suggestion_type varchar(24) NOT NULL CHECK (suggestion_type IN ('add_term','add_exclude','add_entity')),
    value varchar(500) NOT NULL, support_count integer NOT NULL CHECK (support_count >= 2),
    status varchar(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    reviewed_by_user_id bigint REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (monitor_config_version_id, monitor_id) REFERENCES monitor_config_versions(id, monitor_id) ON DELETE RESTRICT,
    CHECK (
      status = 'pending' AND reviewed_by_user_id IS NULL
      OR status IN ('approved','rejected') AND reviewed_by_user_id IS NOT NULL
    )
);
CREATE UNIQUE INDEX monitor_feedback_suggestions_pending_uq
  ON monitor_feedback_suggestions(monitor_config_version_id, suggestion_type, value) WHERE status = 'pending';

COMMIT;
SQL

go run ./cmd/hotkey db verify
```

验证通过后，管理员可创建使用 `gpt-5.6sol`、`env:OPENAI_API_KEY`、不带 `embedding_dimensions` 的 `relevance_review` Profile。ONNX 或任何 embedding dimension 都必须被拒绝。旧 `monitor_matches` 保持可读，但会清楚标记为退化的 legacy 快照；新写入必须带精确 `input_hash` 与 `scoring_version`。

## 回退

本次升级新增列、约束、索引和两张业务表，不能反向手写完整 DDL。恢复前只删除 backup 中不存在、且会阻止旧 `users`/`monitor_matches` 主键还原的两张新表；其余对象由 custom dump 原样重建。PostgreSQL 18 的 `pg_restore` 会将 PLAN-008 的 `ai_runs_reuse_inflight_uq` predicate 以等价、但旧版严格 catalog verifier 不接受的括号形式重建。因此 restore 后必须按固定 PLAN-008 定义重建该索引，之后才运行固定 worktree verifier：

```bash
psql "$HOTKEY_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;

DROP TABLE IF EXISTS monitor_match_feedbacks;
DROP TABLE IF EXISTS monitor_feedback_suggestions;
DROP INDEX IF EXISTS monitor_sources_relevance_active_source_idx;
DROP INDEX IF EXISTS monitor_rules_relevance_approved_lexical_idx;
ALTER TABLE monitor_matches
  DROP CONSTRAINT IF EXISTS monitor_matches_embedding_model_profile_id_fkey,
  DROP CONSTRAINT IF EXISTS monitor_matches_review_ai_run_id_fkey;

COMMIT;
SQL

pg_restore --single-transaction --clean --if-exists --no-owner \
  --dbname="$HOTKEY_DATABASE_URL" /secure-backups/hotkey-before-plan009.dump

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
cmp /secure-backups/hotkey-before-plan009-monitor-matches.csv \
  /secure-backups/hotkey-after-rollback-monitor-matches.csv
```

`cmp` 必须成功；否则历史 `monitor_matches` 没有按 backup 精确恢复，保持服务停止。演练证据由 `TestPlan009SchemaUpgradeAndRollbackUsesPinnedPlan008Worktree` 保存：它真实初始化固定 PLAN-008 worktree、写入非空历史、运行本文 transaction、验证目标 catalog 与 backfill，再还原 backup 并用同一历史 verifier 和历史行复核。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.4 | 2026-07-17 | Acceptance-009 在 `d4efda5` 完成自动门禁并经独立复审 APPROVED；升级手册标记 accepted。 |
