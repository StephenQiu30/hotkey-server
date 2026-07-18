---
layer: Operations
doc_no: "007"
audience: [Dev, QA, Ops]
feature_area: AI Provider升级与连接
purpose: 升级既有模型档案约束并验证 DeepSeek、Ollama 与 Qwen Embedding 连接
canonical_path: docs/operations/007-LangChainGo多模型升级与连接.md
status: accepted
version: v1.0
owner: HotKey Server Team
inputs:
  - db/schema.sql
  - docs/design/archive/015-LangChainGo多Provider与本地模型设计.md
  - docs/plans/archive/018-LangChainGo多模型接入计划.md
outputs:
  - PLAN-018 约束升级、连接探测与安全回退流程
triggers:
  - 既有数据库部署 PLAN-018
  - DeepSeek 或 Ollama 连接诊断
downstream:
  - docs/acceptance/archive/018-LangChainGo多模型接入验收.md
---

# PLAN-018 LangChainGo 多模型升级与连接

## 适用范围与停止条件

新库只执行目标版本的 `hotkey db init --empty-only --confirm-empty`。本手册仅用于已有 PLAN-017 数据库；先备份并停止 API/worker 的模型档案写入和 AI 任务。任一 preflight、DDL、`db verify` 或连接探测失败都停止，不删除 profile、run 或 embedding。

## 备份与升级前检查

```bash
pg_dump "$HOTKEY_DATABASE_URL" --format=custom --file=/secure-backups/hotkey-before-plan018.dump
pg_restore --list /secure-backups/hotkey-before-plan018.dump
psql "$HOTKEY_DATABASE_URL" -v ON_ERROR_STOP=1 -c \
  "SELECT provider, count(*) FROM ai_model_profiles GROUP BY provider ORDER BY provider"
```

升级前只允许既有 `openai/onnx`；出现其他 provider 即停止。随后执行一个事务替换 provider、credential 和 provider/task 约束，并新增 Ollama 模型约束：

```sql
BEGIN;
ALTER TABLE ai_model_profiles DROP CONSTRAINT ai_model_profiles_provider_check;

DO $$
DECLARE item record;
BEGIN
  FOR item IN
    SELECT conname FROM pg_constraint
    WHERE conrelid = 'ai_model_profiles'::regclass AND contype = 'c'
      AND pg_get_constraintdef(oid) ILIKE '%credential_ref%provider%'
  LOOP EXECUTE format('ALTER TABLE ai_model_profiles DROP CONSTRAINT %I', item.conname); END LOOP;
  FOR item IN
    SELECT conname FROM pg_constraint
    WHERE conrelid = 'ai_model_profiles'::regclass AND contype = 'c'
      AND pg_get_constraintdef(oid) ILIKE '%provider%task_type%'
  LOOP EXECUTE format('ALTER TABLE ai_model_profiles DROP CONSTRAINT %I', item.conname); END LOOP;
END $$;

ALTER TABLE ai_model_profiles
  ADD CONSTRAINT ai_model_profiles_provider_check
    CHECK (provider IN ('openai','deepseek','ollama','onnx')),
  ADD CONSTRAINT ai_model_profiles_credential_check CHECK (
    (provider = 'openai' AND credential_ref = 'env:OPENAI_API_KEY')
    OR (provider = 'deepseek' AND credential_ref = 'env:DEEPSEEK_API_KEY')
    OR (provider IN ('ollama','onnx') AND credential_ref IS NULL)
  ),
  ADD CONSTRAINT ai_model_profiles_provider_task_check CHECK (
    (provider = 'onnx' AND task_type = 'embedding')
    OR (provider = 'deepseek' AND task_type <> 'embedding')
    OR provider IN ('openai','ollama')
  ),
  ADD CONSTRAINT ai_model_profiles_ollama_model_check CHECK (
    provider <> 'ollama' OR (
      model_version ~ '^[0-9a-f]{64}$'
      AND (task_type <> 'embedding' OR model_name = 'qwen3-embedding:0.6b')
    )
  );
COMMIT;
```

执行 `go run ./cmd/hotkey db verify`；失败时保持服务停止并从备份恢复。

## 配置与连接探测

DeepSeek 使用 `HOTKEY_DEEPSEEK_API_KEY`，数据库只保存 `env:DEEPSEEK_API_KEY`。Ollama 使用原生根地址，不带 `/v1`：

```dotenv
HOTKEY_DEEPSEEK_API_KEY=
HOTKEY_OLLAMA_ENABLED=true
HOTKEY_OLLAMA_BASE_URL=http://127.0.0.1:11434
```

先由操作者在受信任终端确认本地模型和 digest；不要把输出或 key 写入仓库：

```bash
curl --fail --silent --show-error "$HOTKEY_OLLAMA_BASE_URL/api/tags"
ollama pull qwen3-embedding:0.6b
ollama list
```

用 `/api/tags` 返回 digest 去掉 `sha256:` 前缀后的 64 位小写 hex 创建 Ollama profile。服务调用前会再次校验 digest；tag 漂移时不会调用 chat/embed。DeepSeek 与 Ollama 的业务探测通过管理员创建临时 profile 后运行最小结构化任务/Embedding，用完软删除 profile；不得在命令历史中展开 key。

## 回退

任何代码或约束回退之前，在仍运行 PLAN-018 的数据库执行：

```sql
SELECT count(*) AS plan018_profiles
FROM ai_model_profiles
WHERE provider IN ('deepseek','ollama');
```

结果非零即停止，不删除或改写这些 profile。仅为零时才停服、备份并整体执行下列事务；事务内再次检查以关闭 preflight 与停服之间的竞态，并恢复 PLAN-017 的三个旧约束：

```sql
BEGIN;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM ai_model_profiles WHERE provider IN ('deepseek','ollama')
  ) THEN
    RAISE EXCEPTION 'PLAN-018 rollback refused: deepseek/ollama profiles still exist';
  END IF;
END $$;

ALTER TABLE ai_model_profiles
  DROP CONSTRAINT ai_model_profiles_provider_check,
  DROP CONSTRAINT ai_model_profiles_credential_check,
  DROP CONSTRAINT ai_model_profiles_provider_task_check,
  DROP CONSTRAINT ai_model_profiles_ollama_model_check,
  ADD CONSTRAINT ai_model_profiles_provider_check
    CHECK (provider IN ('openai','onnx')),
  ADD CONSTRAINT ai_model_profiles_credential_check CHECK (
    (provider = 'openai' AND credential_ref = 'env:OPENAI_API_KEY')
    OR (provider = 'onnx' AND credential_ref IS NULL)
  ),
  ADD CONSTRAINT ai_model_profiles_provider_task_check CHECK (
    (provider = 'onnx' AND task_type = 'embedding')
    OR provider = 'openai'
  );

COMMIT;
```

任一语句失败时 PostgreSQL 会保持事务未提交；执行 `ROLLBACK;`，保持服务停止，并从升级前 custom backup 在隔离库验证后整体恢复。事务成功后部署旧代码并运行该版本 `hotkey db verify`。禁止删除 profile、`DROP SCHEMA` 或清空业务表。

## 最近演练

2026-07-18 已在 disposable PostgreSQL 18.4 数据库完成旧约束升级、新矩阵拒绝、回退 preflight 和 `db verify`，fingerprint 为 `fbf72249003644104c60cd6d469f8888a38288f7ee6e37dace873268569d3f50`。当前 DeepSeek probe 返回 401，本机 Ollama 未安装且 11434 不可达；外部连接补测要求与风险记录在 Acceptance-018。
