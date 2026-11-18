#!/usr/bin/env bash
# export.sh —— 导出 skills 安装压缩包。
# 包名规范：DocVision-Skills-绫袅LingNc-<版本>.tar.gz（作者+项目标识进文件名）。
# 用法：bash skills/export.sh [输出目录]   （默认输出到仓库上级目录）
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SKILLS_DIR="$REPO_ROOT/skills"
OUT_DIR="${1:-$REPO_ROOT/..}"
VERSION="$(cd "$REPO_ROOT" && git describe --tags --match 'skills-v*' 2>/dev/null | sed 's/^skills-//' || echo 'dev')"

PKG="DocVision-Skills-绫袅LingNc-$VERSION"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

mkdir -p "$STAGE/$PKG"
cp -r "$SKILLS_DIR/docs" "$SKILLS_DIR/agent" "$SKILLS_DIR/kernel" "$SKILLS_DIR/README.md" "$SKILLS_DIR/PACKAGE-INFO.md" "$SKILLS_DIR/SKILL.md" "$STAGE/$PKG/"
# 预编译二进制（linux/amd64）随包——使用者无需装 Go
if (cd "$SKILLS_DIR/kernel" && go build -buildvcs=false -o bin/dvagent ./cmd/dvagent 2>/dev/null); then
  mkdir -p "$STAGE/$PKG/kernel/bin"
  cp "$SKILLS_DIR/kernel/bin/dvagent" "$STAGE/$PKG/kernel/bin/dvagent"
  echo "  (含预编译二进制 kernel/bin/dvagent)"
fi

# 打包（GNU tar 处理 UTF-8 文件名无碍）
TARBALL="$OUT_DIR/$PKG.tar.gz"
tar -C "$STAGE" -czf "$TARBALL" "$PKG"

echo "导出完成: $TARBALL"
echo "内容: docs/（设计+验收标准）agent/（工作手册）kernel/（dvagent CLI，go build ./cmd/dvagent）"
echo "作者: 绫袅（LingNc） | 仓库: https://github.com/LingNc/DocVision"
