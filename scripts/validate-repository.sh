#!/usr/bin/env bash
# validate-repository.sh — 仓库核心校验入口。
#
# 只校验重要功能：数据库 schema 完整性和架构边界约束。
# 工具链文件存在性、WORKFLOW.md 内容匹配、git 空白等噪音检查已移除。
set -euo pipefail

echo "=== Repository validation ==="

# Step 1: Schema validation
bash "$(dirname "$0")/validate-schema.sh"

# Step 2: Architecture boundary validation
echo ""
bash "$(dirname "$0")/validate-architecture-boundaries.sh"

echo ""
echo "=== All validations passed ==="
