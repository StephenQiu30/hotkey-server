# db/tables/ — 按表名拆分的独立 Schema 文件

每个 `.sql` 文件对应一张数据库表，包含该表的 `CREATE TABLE`、相关 `ALTER TABLE` 和 `CREATE INDEX`。

## 文件约定

- `00_header.sql` — 文件头部注释 + `CREATE EXTENSION`
- `01_*.sql`–`30_*.sql` — 每张表的完整定义
- `build.sh` — 组装脚本：将所有 `*.sql` 拼接回 `../schema.sql`

## 工作流

**修改表结构** → 编辑对应 `db/tables/01_<table_name>.sql`
**新增表** → 在 `db/tables/` 创建 `$(next)_<table_name>.sql`
**重建聚合文件** → `make schema-rebuild`（或 `bash db/tables/build.sh`）
**应用到数据库** → `make schema`（或 `bash scripts/apply-schema.sh`）

> `make validate-schema`（CI 的一部分）会校验聚合后的 `db/schema.sql`。
