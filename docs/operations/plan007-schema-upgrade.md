---
layer: Operations
doc_no: "002"
audience: [Dev, QA, Ops]
feature_area: 数据库升级
purpose: 在不引入 migration 或运行时 DDL 的前提下，将既有 PLAN-006 数据库受控升级到 PLAN-007 Schema
canonical_path: docs/operations/plan007-schema-upgrade.md
status: accepted
version: v1.0
owner: HotKey Server Team
inputs:
  - db/schema.sql
  - docs/design/archive/003-数据库与数据生命周期设计.md
  - docs/design/archive/006-内容标准化去重与证据设计.md
  - docs/plans/archive/007-内容标准化去重与MinIO证据计划.md
outputs:
  - 可恢复的 PLAN-007 结构升级流程
triggers:
  - 已有 PLAN-006 collection_run_items 或旧 Content 数据库需要进入 PLAN-007
downstream:
  - docs/acceptance/archive/007-内容标准化去重与MinIO证据验收.md
---

# PLAN-007 既有数据库受控升级

## 适用范围与停止条件

本手册仅升级已经由 PLAN-006 或更早版本初始化的 PostgreSQL 库。新环境仍必须使用 `go run ./cmd/hotkey db init --empty-only --confirm-empty` 从 [db/schema.sql](../../db/schema.sql) 建立，不能执行本手册。

只在已批准维护窗口、停止会写入 `contents`、`collection_runs` 与 `collection_run_items` 的进程后执行。运行手册中的 SQL 是从 `db/schema.sql` 导出的单次操作，不是第二份 Schema；服务启动绝不自动执行它。任一 preflight、转换或验证失败时立即停止，不启动服务，并进入“回退”。

## 前置条件与备份

操作者必须把 `HOTKEY_DATABASE_URL` 指向目标库，并有 `pg_dump`、`pg_restore` 与变更该库结构的权限。备份路径必须位于受保护、可恢复的位置，不能提交到仓库。

```bash
pg_dump "$HOTKEY_DATABASE_URL" --format=custom --file=/secure-backups/hotkey-before-plan007.dump
pg_restore --list /secure-backups/hotkey-before-plan007.dump
```

上述两条命令均成功，且备份创建时间、目标库名和 `pg_restore --list` 输出已记录，才可继续。

在写入前执行以下只读检查。`duplicate_rows` 必须为 `0`；旧 duplicate 没有足够事实自动补齐原因/版本，必须先由拥有者另行分类，不能猜测。

```sql
SELECT
  (SELECT count(*) FROM collection_run_items AS item
   JOIN collection_runs AS run ON run.id = item.run_id
   WHERE item.run_id IS NULL OR run.source_connection_id IS NULL) AS invalid_runs,
  (SELECT count(*) FROM contents WHERE content_status = 'duplicate' OR duplicate_of_id IS NOT NULL) AS duplicate_rows;
```

## 受控转换

先在从上述 custom backup 恢复的可丢弃副本完整演练；通过后才可在维护窗口的目标库执行。以下 block 必须整体以 `ON_ERROR_STOP` 运行，不能拆分或手工跳过。已有旧 `0` 没有“未知/显式零”的存在性事实，因此 `NULLIF(value, 0)` 是唯一允许的保守转换：正值保持，零改为 unknown；不会重新 Fetch 来源。

```bash
psql "$HOTKEY_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;

ALTER TABLE contents
  ADD COLUMN dedupe_reason varchar(32),
  ADD COLUMN dedupe_version varchar(64),
  ALTER COLUMN view_count DROP NOT NULL,
  ALTER COLUMN view_count DROP DEFAULT,
  ALTER COLUMN like_count DROP NOT NULL,
  ALTER COLUMN like_count DROP DEFAULT,
  ALTER COLUMN comment_count DROP NOT NULL,
  ALTER COLUMN comment_count DROP DEFAULT,
  ALTER COLUMN share_count DROP NOT NULL,
  ALTER COLUMN share_count DROP DEFAULT;
UPDATE contents
SET view_count = NULLIF(view_count, 0),
    like_count = NULLIF(like_count, 0),
    comment_count = NULLIF(comment_count, 0),
    share_count = NULLIF(share_count, 0);
ALTER TABLE contents
  ADD CONSTRAINT contents_id_source_connection_key UNIQUE (id, source_connection_id),
  ADD CONSTRAINT contents_duplicate_evidence_check CHECK (
    (content_status = 'duplicate' AND duplicate_of_id IS NOT NULL AND dedupe_reason IS NOT NULL AND dedupe_version IS NOT NULL)
    OR (content_status <> 'duplicate' AND duplicate_of_id IS NULL AND dedupe_reason IS NULL AND dedupe_version IS NULL)
  );

ALTER TABLE content_metric_snapshots
  ALTER COLUMN view_count DROP NOT NULL,
  ALTER COLUMN view_count DROP DEFAULT,
  ALTER COLUMN like_count DROP NOT NULL,
  ALTER COLUMN like_count DROP DEFAULT,
  ALTER COLUMN comment_count DROP NOT NULL,
  ALTER COLUMN comment_count DROP DEFAULT,
  ALTER COLUMN share_count DROP NOT NULL,
  ALTER COLUMN share_count DROP DEFAULT;
UPDATE content_metric_snapshots
SET view_count = NULLIF(view_count, 0),
    like_count = NULLIF(like_count, 0),
    comment_count = NULLIF(comment_count, 0),
    share_count = NULLIF(share_count, 0);

ALTER TABLE collection_runs
  ADD CONSTRAINT collection_runs_id_source_connection_key UNIQUE (id, source_connection_id);
ALTER TABLE collection_run_items
  ADD COLUMN source_connection_id bigint,
  ADD COLUMN ingestion_status varchar(16) NOT NULL DEFAULT 'pending'
    CHECK (ingestion_status IN ('pending', 'succeeded', 'failed')),
  ADD COLUMN ingestion_error_code varchar(64);
UPDATE collection_run_items AS item
SET source_connection_id = run.source_connection_id
FROM collection_runs AS run
WHERE run.id = item.run_id;
UPDATE collection_run_items
SET ingestion_status = 'succeeded',
    ingestion_error_code = NULL
WHERE content_id IS NOT NULL;
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM collection_run_items AS item
    LEFT JOIN collection_runs AS run ON run.id = item.run_id
    WHERE item.source_connection_id IS NULL OR item.source_connection_id <> run.source_connection_id
  ) THEN
    RAISE EXCEPTION 'PLAN-007 source_connection_id backfill failed';
  END IF;
END
$$;
ALTER TABLE collection_run_items
  ALTER COLUMN source_connection_id SET NOT NULL,
  DROP CONSTRAINT IF EXISTS collection_run_items_content_id_fkey,
  ADD CONSTRAINT collection_run_items_run_source_connection_fkey
    FOREIGN KEY (run_id, source_connection_id)
    REFERENCES collection_runs(id, source_connection_id) ON DELETE CASCADE,
  ADD CONSTRAINT collection_run_items_content_source_connection_fkey
    FOREIGN KEY (content_id, source_connection_id)
    REFERENCES contents(id, source_connection_id) ON DELETE SET NULL (content_id),
  ADD CONSTRAINT collection_run_items_ingestion_state_check CHECK (
    (outcome = 'captured' AND (
      (content_id IS NULL AND ingestion_status = 'pending' AND ingestion_error_code IS NULL)
      OR (content_id IS NULL AND ingestion_status = 'failed' AND ingestion_error_code IS NOT NULL)
      OR (content_id IS NOT NULL AND ingestion_status = 'succeeded' AND ingestion_error_code IS NULL)
    ))
    OR (outcome IN ('skipped', 'failed') AND content_id IS NULL AND ingestion_status = 'pending' AND ingestion_error_code IS NULL)
  );

COMMIT;
SQL
```

CapturedItem v1 JSON is not rewritten: the Source reader maps its zero metrics to unknown at read time. All captures written after this upgrade use v2 nullable metric fields, so an explicit zero remains zero.

## 验证

执行升级后，以下查询的 `wrong_source_bindings`、`null_source_connection` 和 `invalid_ingestion_states` 必须都是 `0`；保存 upgrade 前后行数与每个统一指标的正值聚合，确认正值未被改变。

```sql
SELECT
  (SELECT count(*)
   FROM collection_run_items AS item
   JOIN collection_runs AS run ON run.id = item.run_id
   WHERE item.source_connection_id <> run.source_connection_id) AS wrong_source_bindings,
  (SELECT count(*) FROM collection_run_items WHERE source_connection_id IS NULL) AS null_source_connection,
  (SELECT count(*)
   FROM collection_run_items
   WHERE NOT (
     (outcome = 'captured' AND (
       (content_id IS NULL AND ingestion_status = 'pending' AND ingestion_error_code IS NULL)
       OR (content_id IS NULL AND ingestion_status = 'failed' AND ingestion_error_code IS NOT NULL)
       OR (content_id IS NOT NULL AND ingestion_status = 'succeeded' AND ingestion_error_code IS NULL)
     ))
     OR (outcome IN ('skipped', 'failed') AND content_id IS NULL AND ingestion_status = 'pending' AND ingestion_error_code IS NULL)
   )) AS invalid_ingestion_states,
  (SELECT count(*) FROM contents WHERE view_count = 0 OR like_count = 0 OR comment_count = 0 OR share_count = 0) AS legacy_zero_values;
```

```bash
HOTKEY_DATABASE_URL="$HOTKEY_DATABASE_URL" go run ./cmd/hotkey db verify
HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' \
  go test ./internal/platform/database -run TestPlan007SchemaUpgradeBackfillsCaptureSourceAndPreservesMetricPolicy -count=1
```

`legacy_zero_values` 必须为 `0`；升级后新写入的显式零由 PLAN-007 repository integration 测试验证，不用此旧数据检查推断。

## 回退

若转换未提交，`psql` 会退出并回滚当前 transaction；不要继续重试或手工修正。若转换已提交但验证失败，保持服务停止并从已验证 backup 恢复。`collection_run_items` 的以下两个复合外键只在 PLAN-007 转换后存在，旧 backup 不包含它们；先仅删除这两个新外键，避免 `pg_restore --clean` 因无法删除 `contents` 或 `collection_runs` 而留下混合 Schema。不得使用 `DROP SCHEMA ... CASCADE` 或其他宽泛重置。

```bash
psql "$HOTKEY_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
ALTER TABLE collection_run_items
  DROP CONSTRAINT IF EXISTS collection_run_items_run_source_connection_fkey,
  DROP CONSTRAINT IF EXISTS collection_run_items_content_source_connection_fkey;
SQL

pg_restore --clean --if-exists --no-owner --dbname="$HOTKEY_DATABASE_URL" /secure-backups/hotkey-before-plan007.dump
```

恢复后必须同时回退到创建该 backup 的 release，并使用该 release 的 `hotkey db verify` 校验其 legacy Schema；当前 PLAN-007 二进制的 verifier 故意要求新 catalog，不能拿它验证已恢复的旧库。确认恢复校验通过后，记录失败原因并创建新的前向修复计划；不得在未恢复的目标库上继续写入。
