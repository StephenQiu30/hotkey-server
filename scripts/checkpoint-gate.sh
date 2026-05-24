#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "[GATE] 扫描运行时 apps 目录..."
apps_dirs="$(find . -type d -name apps -print)"
if [ -n "$apps_dirs" ]; then
  echo "[FAIL] 检测到运行时 apps 目录："
  echo "$apps_dirs"
  exit 1
fi

echo "[GATE] 扫描 apps 文件路径引用..."
apps_files="$(find . -type f | rg '(^|/)apps/' || true)"
if [ -n "$apps_files" ]; then
  echo "[FAIL] 检测到 apps 路径文件："
  echo "$apps_files"
  exit 1
fi

echo "[GATE] 运行仓库治理测试..."
python3 -m unittest discover -s tests -p 'test_repository_governance.py'

echo "[GATE] 检查工作区是否为空..."
if [ -n "$(git status --short)" ]; then
  echo "[FAIL] 工作区存在未提交变更："
  git status --short
  exit 1
else
  echo "[PASS] 工作区已清空"
fi
