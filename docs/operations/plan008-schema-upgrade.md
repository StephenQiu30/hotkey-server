---
layer: Operations
doc_no: "008"
audience: [Dev, QA, Ops]
feature_area: 数据库升级
purpose: 将空 AI 运行历史的 PLAN-007 数据库受控升级至 PLAN-008 AI Provider 与 Embedding Schema，并可精确回退
canonical_path: docs/operations/plan008-schema-upgrade.md
status: review
version: v0.1
owner: HotKey Server Team
inputs:
  - db/schema.sql
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/007-多语言匹配与相关性设计.md
  - docs/design/011-AI任务证据与模型运行设计.md
  - docs/plans/008-AIProvider与Embedding基础计划.md
outputs:
  - 可恢复的 PLAN-008 结构升级流程
triggers:
  - 已由 PLAN-007 Schema 初始化且尚无 AI 历史的数据库进入 PLAN-008
downstream:
  - docs/acceptance/008-AIProvider与Embedding基础验收.md
---

# PLAN-008 既有数据库受控升级

## 适用范围、停止条件和前置版本

本手册只适用于已经由 PLAN-007 或更早的完整 Schema 初始化、且尚未写入任何 AI profile、运行、证据或向量的 PostgreSQL 16+ / pgvector 数据库。新环境仍使用目标 release 的 `go run ./cmd/hotkey db init --empty-only --confirm-empty`，不能执行本手册。

此文档描述 PLAN-008 Task 1 目标 Schema，不能在 Task 1 未合入、`db/schema.sql` 未包含对应 catalog contract 时提前执行。服务启动绝不自动执行本手册。操作前停止 API/worker 中所有可能写 AI profile、run、ledger 或 embedding 的进程；任何 backup、preflight、DDL、verify 或 restore 失败时立即停止，不启动服务，并进入“回退”。

手册有意拒绝带 AI 历史的库：旧 Schema 没有 model version、reuse key、预算预留和结构化错误的可靠事实，不能猜测回填。此类库必须由新的数据迁移 Plan 单独处理。

## 备份与只读 preflight

`HOTKEY_DATABASE_URL` 必须指向维护窗口中的目标库，操作者须具备 `pg_dump`、`pg_restore` 和 DDL 权限。dump 放在受保护且可恢复的位置，绝不提交到仓库。

```bash
pg_dump "$HOTKEY_DATABASE_URL" --format=custom --file=/secure-backups/hotkey-before-plan008.dump
pg_restore --list /secure-backups/hotkey-before-plan008.dump
```

两条命令均成功后，执行以下只读查询。五个值都必须为 `0`；非零即停止并保留 backup，不得删除、截断或自行回填 AI 数据。

```sql
SELECT
  (SELECT count(*) FROM ai_model_profiles) AS profiles,
  (SELECT count(*) FROM ai_runs) AS runs,
  (SELECT count(*) FROM ai_run_evidences) AS run_evidences,
  (SELECT count(*) FROM content_embeddings) AS content_vectors,
  (SELECT count(*) FROM monitor_embeddings) +
  (SELECT count(*) FROM event_embeddings) +
  (SELECT count(*) FROM topic_embeddings) AS other_vectors;
```

在从该 custom backup 恢复的可丢弃数据库完整演练以下升级、目标 `hotkey db verify`、预备回退和历史 release verify 后，才能在目标库执行。

## 受控升级

下列 block 必须整体通过 `psql -v ON_ERROR_STOP=1` 运行，不能拆分或手工调整列顺序。`db/schema.sql` 的 `ai_model_profiles` 新列必须位于既有 `deleted_at` 后，`ai_runs` 新列必须位于既有 `finished_at` 后，四张 embedding 表的新 `model_profile_version` 必须位于既有 `created_at` 后；PostgreSQL 不能移动 ADD COLUMN 的物理位置，而 `hotkey db verify` 有意校验 catalog contract。

```bash
psql "$HOTKEY_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;

ALTER TABLE ai_model_profiles
  DROP CONSTRAINT IF EXISTS ai_model_profiles_task_type_check,
  ADD CONSTRAINT ai_model_profiles_task_type_check
    CHECK (task_type IN ('classification','clustering','extraction','summarization','report','embedding','term_expansion')),
  ADD COLUMN model_version varchar(64) NOT NULL,
  ADD COLUMN embedding_dimensions smallint,
  ADD COLUMN max_attempts smallint NOT NULL DEFAULT 1 CHECK (max_attempts BETWEEN 1 AND 3),
  ADD COLUMN max_cost numeric(12,4) CHECK (max_cost IS NULL OR max_cost > 0),
  ADD CONSTRAINT ai_model_profiles_provider_check CHECK (provider IN ('openai','onnx')),
  ADD CONSTRAINT ai_model_profiles_endpoint_check CHECK (endpoint IS NULL),
  ADD CONSTRAINT ai_model_profiles_parameters_check CHECK (parameters = '{}'::jsonb),
  ADD CONSTRAINT ai_model_profiles_credential_check CHECK (
    (provider = 'openai' AND credential_ref = 'env:OPENAI_API_KEY')
    OR (provider = 'onnx' AND credential_ref IS NULL)
  ),
  ADD CONSTRAINT ai_model_profiles_embedding_dimension_check CHECK (
    (task_type = 'embedding' AND embedding_dimensions = 1024)
    OR (task_type <> 'embedding' AND embedding_dimensions IS NULL)
  ),
  ADD CONSTRAINT ai_model_profiles_provider_task_check CHECK (
    (provider = 'onnx' AND task_type = 'embedding')
    OR provider = 'openai'
  ),
  ADD CONSTRAINT ai_model_profiles_budget_check CHECK (
    daily_budget IS NULL OR daily_budget > 0
  ),
  ADD CONSTRAINT ai_model_profiles_budget_order_check CHECK (
    daily_budget IS NULL OR max_cost IS NULL OR daily_budget >= max_cost
  );

DO $$
DECLARE old_unique text;
BEGIN
  SELECT constraint_name INTO old_unique
  FROM information_schema.table_constraints
  WHERE table_schema = 'public'
    AND table_name = 'ai_runs'
    AND constraint_type = 'UNIQUE'
    AND constraint_name <> 'ai_runs_pkey'
  ORDER BY constraint_name
  LIMIT 1;
  IF old_unique IS NULL THEN
    RAISE EXCEPTION 'PLAN-008 expected the PLAN-007 ai_runs uniqueness constraint';
  END IF;
  EXECUTE format('ALTER TABLE ai_runs DROP CONSTRAINT %I', old_unique);
END
$$;

ALTER TABLE ai_runs
  DROP CONSTRAINT IF EXISTS ai_runs_status_check,
  DROP COLUMN error,
  ALTER COLUMN model_profile_id SET NOT NULL,
  ADD COLUMN model_profile_version bigint NOT NULL,
  ADD COLUMN model_version varchar(64) NOT NULL,
  ADD COLUMN parameters_version varchar(64) NOT NULL,
  ADD COLUMN input_schema_version varchar(64) NOT NULL,
  ADD COLUMN evidence_set_hash char(64) NOT NULL,
  ADD COLUMN reuse_key char(64) NOT NULL,
  ADD COLUMN attempt smallint NOT NULL DEFAULT 1 CHECK (attempt BETWEEN 1 AND 3),
  ADD COLUMN max_attempts smallint NOT NULL DEFAULT 1 CHECK (max_attempts BETWEEN 1 AND 3),
  ADD COLUMN repair_attempted boolean NOT NULL DEFAULT false,
  ADD COLUMN retry_after timestamptz,
  ADD COLUMN error_code integer,
  ADD COLUMN budget_day date NOT NULL,
  ADD COLUMN reserved_cost numeric(12,4) NOT NULL DEFAULT 0 CHECK (reserved_cost >= 0),
  ADD CONSTRAINT ai_runs_status_check CHECK (status IN ('queued','running','validating','retry_wait','succeeded','failed','cancelled')),
  ADD CONSTRAINT ai_runs_object_keys_null_check CHECK (request_object_key IS NULL AND response_object_key IS NULL),
  ADD CONSTRAINT ai_runs_attempt_order_check CHECK (attempt <= max_attempts),
  ADD CONSTRAINT ai_runs_cost_check CHECK (cost <= reserved_cost);

CREATE UNIQUE INDEX ai_runs_reuse_succeeded_uq
  ON ai_runs(reuse_key) WHERE status = 'succeeded';
CREATE UNIQUE INDEX ai_runs_reuse_inflight_uq
  ON ai_runs(reuse_key) WHERE status IN ('queued','running','validating','retry_wait');

CREATE TABLE ai_budget_ledgers (
  id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  model_profile_id bigint NOT NULL REFERENCES ai_model_profiles(id) ON DELETE RESTRICT,
  budget_day date NOT NULL,
  reserved_cost numeric(12,4) NOT NULL DEFAULT 0 CHECK (reserved_cost >= 0),
  settled_cost numeric(12,4) NOT NULL DEFAULT 0 CHECK (settled_cost >= 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (model_profile_id, budget_day)
);

ALTER TABLE content_embeddings ADD COLUMN model_profile_version bigint NOT NULL;
ALTER TABLE monitor_embeddings ADD COLUMN model_profile_version bigint NOT NULL;
ALTER TABLE event_embeddings ADD COLUMN model_profile_version bigint NOT NULL;
ALTER TABLE topic_embeddings ADD COLUMN model_profile_version bigint NOT NULL;

CREATE UNIQUE INDEX content_embeddings_one_active_per_profile_uq
  ON content_embeddings(content_id, model_profile_id) WHERE active;
CREATE UNIQUE INDEX monitor_embeddings_one_active_per_profile_uq
  ON monitor_embeddings(monitor_id, model_profile_id) WHERE active;
CREATE UNIQUE INDEX event_embeddings_one_active_per_profile_uq
  ON event_embeddings(event_id, model_profile_id) WHERE active;
CREATE UNIQUE INDEX topic_embeddings_one_active_per_profile_uq
  ON topic_embeddings(topic_id, model_profile_id) WHERE active;

COMMIT;
SQL

go run ./cmd/hotkey db verify
```

`db verify` 必须通过。再重新执行 preflight：五个计数仍为零，并额外确认新 table 存在、两个 ai_runs reuse index 与四个 `one_active_per_profile` index 存在。任何失败都不接受部分修复，直接转入回退。

## 回退

回退只在目标服务仍停止时进行。先在副本上演练以下顺序；**故意不准备**的 `pg_restore --clean --if-exists --no-owner` 应失败，因为新 `ai_budget_ledgers` 外键仍引用 `ai_model_profiles`。该失败是未跳过清理步骤的证据，不是可忽略的警告。

准备恢复只删除 PLAN-008 新增对象；不删除 Schema、不触及 Content/Source/MinIO，也不清空任何业务表：

```bash
psql "$HOTKEY_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;
DROP INDEX IF EXISTS ai_runs_reuse_succeeded_uq;
DROP INDEX IF EXISTS ai_runs_reuse_inflight_uq;
DROP INDEX IF EXISTS content_embeddings_one_active_per_profile_uq;
DROP INDEX IF EXISTS monitor_embeddings_one_active_per_profile_uq;
DROP INDEX IF EXISTS event_embeddings_one_active_per_profile_uq;
DROP INDEX IF EXISTS topic_embeddings_one_active_per_profile_uq;
DROP TABLE IF EXISTS ai_budget_ledgers;
COMMIT;
SQL

pg_restore --clean --if-exists --no-owner \
  --dbname="$HOTKEY_DATABASE_URL" /secure-backups/hotkey-before-plan008.dump
```

`pg_restore` 重建 backup 中的 PLAN-007 ai 表，因而自动移除 PLAN-008 新增 columns/constraints；禁止手工保留其中任一列。最后切换到创建 backup 的 PLAN-007 release 并运行其 verifier：

```bash
go run ./cmd/hotkey db verify
```

该命令必须成功，且 preflight table 不含 `ai_budget_ledgers`、`model_version`、`reuse_key`、`model_profile_version` 和 PLAN-008 index。回退成功前不得恢复服务。

## 演练与证据

PLAN-008 的 database integration test 必须从真实 PLAN-007 release Schema 创建 disposable database，生成 custom dump，执行本手册的 exact upgrade，运行当前 verifier，记录新 catalog/index，先断言未准备 restore 的受控失败，再执行准备步骤与 restore，最后用 historical PLAN-007 verifier 通过。Acceptance-008 只记录命令、fixture、commit 和结果摘要，不记录 DSN、dump 路径或任何凭据。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-17 | 定义空 AI 历史库的 PLAN-007 -> PLAN-008 备份、严格 preflight、add-only physical order、目标 verify、受控 restore 与历史 verifier 演练；待实现和独立复核。 |
