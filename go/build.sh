#!/bin/bash
# PDF 分割工具构建脚本
# 用法: ./build.sh [all|linux|windows|clean]

set -e

# 确保 Go 能自动下载所需工具链版本
export GOTOOLCHAIN=auto

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/release"

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

build() {
    local os=$1
    local arch=$2
    local ext=""
    [ "$os" = "windows" ] && ext=".exe"

    log "构建 split_pdfs ($os/$arch)..."
    cd "$SCRIPT_DIR"
    GOOS=$os GOARCH=$arch go build -o "$OUTPUT_DIR/split_pdfs_${os}_${arch}${ext}" .
    log "构建完成: split_pdfs_${os}_${arch}${ext}"
}

clean() {
    log "清理构建产物..."
    rm -rf "$OUTPUT_DIR"
    log "清理完成"
}

case "${1:-all}" in
    linux)
        mkdir -p "$OUTPUT_DIR"
        build "linux" "amd64"
        ;;
    windows)
        mkdir -p "$OUTPUT_DIR"
        build "windows" "amd64"
        ;;
    all)
        mkdir -p "$OUTPUT_DIR"
        log "=== 开始构建所有版本 ==="

        # Linux
        build "linux" "amd64"
        # Windows
        build "windows" "amd64"

        log "=== 构建完成 ==="
        log "输出目录: $OUTPUT_DIR/"
        ls -la "$OUTPUT_DIR/"
        ;;
    clean)
        clean
        ;;
    *)
        echo "用法: $0 [all|linux|windows|clean]"
        echo ""
        echo "  all     - 构建所有版本 (默认)"
        echo "  linux   - 仅构建 Linux 版本"
        echo "  windows - 仅构建 Windows 版本"
        echo "  clean   - 清理构建产物"
        exit 1
        ;;
esac
