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
(cd "$REPO_ROOT" && git rev-parse --short HEAD) > "$STAGE/$PKG/.skill-version"
# 开发向文档（plan/issue 台账、迁移清点）不进包——用户包只带操作/规约类文档
mkdir -p "$STAGE/$PKG/docs"
for f in "$SKILLS_DIR"/docs/*.md; do
	case "$(basename "$f")" in
	plan.md|issue.md|04-migration-inventory.md) continue ;;
	esac
	cp "$f" "$STAGE/$PKG/docs/"
done
cp -r "$SKILLS_DIR/agent" "$SKILLS_DIR/kernel" "$SKILLS_DIR/README.md" "$SKILLS_DIR/PACKAGE-INFO.md" "$SKILLS_DIR/SKILL.md" "$STAGE/$PKG/"
# 用户配置模板随包（激活=cp config.example.yaml config.yaml；真 config.yaml 永不入包）
cp "$SKILLS_DIR/config.example.yaml" "$STAGE/$PKG/config.example.yaml"
# 用户档案模板随包（空值示例；真实 profile.env 永不入包，见 docs/06）
mkdir -p "$STAGE/$PKG/data"
cp "$SKILLS_DIR/data/profile.example" "$STAGE/$PKG/data/profile.example"
# 预编译二进制（linux/amd64）随包——使用者无需装 Go
if ! command -v go >/dev/null 2>&1; then
  echo "  (无 go：包内不含预编译二进制，使用者自行 cd kernel && go build -o bin/dvagent ./cmd/dvagent)"
elif (cd "$SKILLS_DIR/kernel" && GOFLAGS=-mod=mod GOCACHE=/tmp/gocache go build -buildvcs=false -o bin/dvagent ./cmd/dvagent 2>&1); then
  mkdir -p "$STAGE/$PKG/kernel/bin"
  cp "$SKILLS_DIR/kernel/bin/dvagent" "$STAGE/$PKG/kernel/bin/dvagent"
  echo "  (含预编译二进制 kernel/bin/dvagent)"
else
  echo "  (编译失败：包内不含二进制，使用者自行编译)"
fi

# 打包（GNU tar 处理 UTF-8 文件名无碍）
TARBALL="$OUT_DIR/$PKG.tar.gz"
tar -C "$STAGE" -czf "$TARBALL" "$PKG"

echo "导出完成: $TARBALL"
echo "内容: docs/（设计+验收标准）agent/（工作手册）kernel/（dvagent CLI，go build ./cmd/dvagent）"
echo "作者: 绫袅（LingNc） | 仓库: https://github.com/LingNc/DocVision"
