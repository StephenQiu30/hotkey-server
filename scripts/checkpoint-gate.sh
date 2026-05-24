#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "[GATE] 扫描运行时 apps 目录..."
find . -type d -name apps -print

echo "[GATE] 扫描 apps 文件路径引用..."
find . -type f | rg '(^|/)apps/' || true

echo "[GATE] 运行仓库治理测试..."
python3 -m unittest discover -s tests -p 'test_repository_governance.py'

echo "[GATE] 检查工作区是否为空..."
if [ -n "$(git status --short)" ]; then
  echo "[WARN] 工作区存在未提交变更："
  git status --short
else
  echo "[PASS] 工作区已清空"
fi
