#!/usr/bin/env bash
# validate-schema.sh — 数据库 schema 核心校验。
#
# 检查 db/schema.sql 的存在性、表结构完整性和关键约束。
set -euo pipefail

echo "=== Schema validation ==="

if [ ! -f db/schema.sql ]; then
  echo "FAIL: db/schema.sql is required as the schema baseline"
  exit 1
fi

if ! grep -q "create table" db/schema.sql; then
  echo "FAIL: db/schema.sql contains no CREATE TABLE statements"
  exit 1
fi
table_count=$(grep -c "create table" db/schema.sql)
echo "OK: db/schema.sql contains $table_count tables"

if [ -d db/migrations ]; then
  migration_count=$(ls db/migrations/*.sql 2>/dev/null | wc -l)
  echo "OK: db/migrations/ exists with $migration_count migration files (goose)"
fi

if ! grep -q 'topic_id, snapshot_time' db/schema.sql; then
  echo "FAIL: db/schema.sql must include topic snapshot upsert constraint"
  exit 1
fi
echo "OK: topic snapshot upsert constraint present"

if ! grep -q 'monitor_id, snapshot_time' db/schema.sql; then
  echo "FAIL: db/schema.sql must include monitor snapshot upsert constraint"
  exit 1
fi
echo "OK: monitor snapshot upsert constraint present"

if ! grep -q 'notification_id bigint not null references user_notifications(id)' db/schema.sql; then
  echo "FAIL: db/schema.sql must include the current email_deliveries shape"
  exit 1
fi
echo "OK: email_deliveries foreign key present"

echo ""
echo "=== Schema validation passed ==="
