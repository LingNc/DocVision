#!/usr/bin/env bash
# clean_convert.sh —— 把档位1 的运行时项目清回「章节划分刚做完、还没逐章转换」的状态。
#
# 用途：章节转换目前是最贵的一段（每章一个会话、失败重试还换新会话、递归细分最多
# 三层），改完转换逻辑要靠重跑验证时，分类结果、样式包、章节划分这些前置成果都
# 不该重烧。这个脚本**删掉章节转换及其之后的一切**（转换成品、每章报告、逐章与
# checker 转录、临时工作区，以及风格修复大循环、分块修复、汇编、终审的产物），只
# 保留 `chapters/*.md` 的划分成果与 `style/` 样式包，重跑直接从逐章转换开始。
#
# 用法：
#   ./clean_convert.sh [-n] [--no-backup] [--keep-convert] [-f] [-y] <项目目录>
#
#   -n, --dry-run     只打印计划，不动任何文件
#       --no-backup   跳过清理前的整目录备份（默认备份）
#       --keep-convert
#                     反口径：**保留**章节转换的成果（work/chapters、work/reports、
#                     转换与 checker 转录、work/temp），只清它之后的阶段，重跑从
#                     风格修复大循环接上
#   -f, --force       检测到 docvision latex 正在运行时也继续（默认拒绝）
#   -y, --yes         不交互确认
#   -h, --help        显示帮助
#
# 默认保留（= 章节转换之前的一切）：
#   chapters/<章>.md（划分成果）、work/sessions/chapters.jsonl（划分会话）、
#   style/、work/style/、work/style_session.jsonl（样式包）、
#   source/（含 progress_items 分类结果、images 软链、dedupe 结论）、doc_index/、
#   progress.json、.docvision_project.json、work/mem.log
# 默认删除（= 章节转换及其之后的一切）：
#   work/chapters/（转换成品）、work/reports/（每章报告）、work/temp/（章节工作区、
#   递归子树、.gate、issues store）、work/sessions/convert_*|checker_*（逐章与
#   checker 转录）、build/、out/、<项目名>.pdf（汇编交付副本）、
#   work/style_fix/、work/stylefix_assess/、work/stylefix_integrate/、work/final_review/、
#   work/views/、work/media/、work/pdfview/、work/sessions/book_fix.jsonl、
#   work/sessions/final_review.jsonl、work/sessions/style_fix_*.jsonl
# 默认清理（阶段标记）：progress.json 的 convert、assemble 归空（style/images/chapters
#   保持 done）——重跑时这三段跳过，直接从逐章转换开始。
#
set -euo pipefail

prog="$(basename "$0")"

dry_run=0
do_backup=1
keep_convert=0
force=0
assume_yes=0
proj=""

usage() { sed -n '2,44p' "$0" | sed 's/^# \{0,1\}//'; }

say()  { printf '%s\n' "$*"; }
step() { printf '\n\033[1m%s\033[0m\n' "$*"; }
warn() { printf '\033[33m%s\033[0m\n' "$*" >&2; }
die()  { printf '\033[31m%s\033[0m\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    -n|--dry-run)     dry_run=1 ;;
    --no-backup)      do_backup=0 ;;
    --keep-convert)   keep_convert=1 ;;
    --drop-convert)   keep_convert=0 ;;   # 旧稿的写法，等价于默认
    -f|--force)       force=1 ;;
    -y|--yes)         assume_yes=1 ;;
    -h|--help)        usage; exit 0 ;;
    -*)               die "未知选项：$1（用 -h 看用法）" ;;
    *)                [ -z "$proj" ] || die "只接受一个项目目录（多给了：$1）"; proj="$1" ;;
  esac
  shift
done

# ---- 定位项目目录 --------------------------------------------------------
if [ -z "$proj" ]; then
  say "用法：$prog [-n] [--no-backup] [--keep-convert] [-f] [-y] <项目目录>"
  cand=()
  for d in ./latex_project/*/; do
    [ -d "$d" ] || continue
    case "$(basename "$d")" in
      *_记录_*) continue ;;
    esac
    cand+=("$d")
  done
  if [ ${#cand[@]} -gt 0 ]; then
    say ""
    say "候选项目（$PWD/latex_project）："
    for d in "${cand[@]}"; do say "  $d"; done
  fi
  exit 1
fi
[ -d "$proj" ] || die "项目目录不存在：$proj"
proj="$(cd "$proj" && pwd)"
book="$(basename "$proj")"

# ---- 确认这是跑过档位1 的项目 --------------------------------------------
[ -d "$proj/source" ] && [ -d "$proj/work" ] || die "看起来不是 docvision 项目（缺 source/ 或 work/）：$proj"
[ -d "$proj/work/chapters" ] || die "没有 work/chapters（章节转换还没跑过？）：$proj
前置阶段没跑过就没有可清的东西。"
nchap=$(find "$proj/work/chapters" -maxdepth 1 -name 'chapter_*.tex' 2>/dev/null | wc -l)
if [ "$nchap" -eq 0 ]; then
  warn "  work/chapters 里没有 chapter_*.tex——章节转换还没产出成品，可能上次就中断在这里"
fi
# 划分成果必须还在，否则清了转换就等于从零重头跑（那不是这个脚本的用途）。
if ! find "$proj/chapters" -maxdepth 1 -name 'chapter_*.md' 2>/dev/null | grep -q .; then
  warn "  注意：chapters/ 里没有 chapter_*.md（章节划分成果），清理后重新跑会从划分阶段开始"
fi

# ---- 正在运行就不要动 ----------------------------------------------------
if [ "$force" -eq 0 ] && pgrep -f 'docvision[[:space:]]+latex' >/dev/null 2>&1; then
  die "检测到 docvision latex 正在运行，先停下再清（确认要动请加 -f）：
$(pgrep -af 'docvision[[:space:]]+latex' | sed 's/^/  /')"
fi

# ---- 清理计划 ------------------------------------------------------------
# 转换之后的阶段产物（两种口径都删）。
to_delete_after=(
  "build"
  "out"
  "work/style_fix"
  "work/stylefix_assess"
  "work/stylefix_integrate"
  "work/final_review"
  "work/views"
  "work/media"
  "work/pdfview"
)
to_delete_after_files=(
  "work/sessions/book_fix.jsonl"
  "work/sessions/final_review.jsonl"
)
# 转换本身（默认删；--keep-convert 时保留）。
to_delete_convert=(
  "work/chapters"
  "work/reports"
  "work/temp"
)
to_delete=("${to_delete_after[@]}")
[ "$keep_convert" -eq 1 ] || to_delete+=("${to_delete_convert[@]}")

# 旧二进制转出的章节可能把 `_subdivision/…` 交进书里（T166 修掉的断链）：
# 默认口径下它们会被删掉，不用管；--keep-convert 时要点出来（汇编会失败）。
broken=$(grep -rl '_subdivision/' "$proj/work/chapters" --include='*.tex' 2>/dev/null | head -5 || true)

step "清理计划：$proj"
say "  章节转换成品：$nchap 章（$proj/work/chapters）— $([ "$keep_convert" -eq 1 ] && echo '保留，重跑从风格修复接上' || echo '删除，重跑从逐章转换开始')"
for d in "${to_delete[@]}"; do
  if [ -e "$proj/$d" ]; then
    if [ -d "$proj/$d" ] && [ -z "$(ls -A "$proj/$d" 2>/dev/null)" ]; then
      say "  删除：$d  (空目录)"
    else
      say "  删除：$d  ($(du -sh "$proj/$d" 2>/dev/null | cut -f1))"
    fi
  fi
done
for f in "${to_delete_after_files[@]}"; do
  [ -e "$proj/$f" ] && say "  删除：$f  ($(du -h "$proj/$f" 2>/dev/null | cut -f1))"
done
if [ "$keep_convert" -eq 0 ]; then
  for f in "$proj"/work/sessions/convert_*.jsonl; do
    [ -e "$f" ] && say "  删除：work/sessions/$(basename "$f")  ($(du -h "$f" 2>/dev/null | cut -f1))"
  done
  for f in "$proj"/work/sessions/checker_*.jsonl; do
    [ -e "$f" ] && say "  删除：work/sessions/$(basename "$f")  ($(du -h "$f" 2>/dev/null | cut -f1))"
  done
fi
for f in "$proj"/work/sessions/style_fix_*.jsonl; do
  [ -e "$f" ] && say "  删除：work/sessions/$(basename "$f")  ($(du -h "$f" 2>/dev/null | cut -f1))"
done
sibling="$(dirname "$proj")/$book.pdf"
[ -e "$sibling" ] && say "  删除：$sibling  （汇编交付副本，$(du -h "$sibling" 2>/dev/null | cut -f1)）"

if [ "$keep_convert" -eq 1 ]; then
  say "  保留：work/chapters/、work/reports/、转换与 checker 转录、work/temp/"
  say "  阶段标记：progress.json 的 convert、assemble 归空 —— 章节「已有产物，跳过」，从风格修复重跑"
  if [ -n "$broken" ]; then
    warn ""
    warn "  ⚠ 转换成品里有指向 _subdivision/ 的断链路径（旧版递归父章节的交法）："
    printf '%s\n' "$broken" | sed 's/^/    /' >&2
    warn "    --keep-convert 会保留它们，汇编时多半编译失败；要按新代码重转就别加这个参数"
  fi
else
  say "  保留：chapters/*.md、work/sessions/chapters.jsonl、style/、work/style/、work/style_session.jsonl、source/、doc_index/"
  say "  阶段标记：progress.json 的 convert、assemble 归空 —— 分类/样式/划分跳过，从逐章转换重跑"
  if [ -n "$broken" ]; then
    say "  顺带说一句：检测到 $(printf '%s\n' "$broken" | wc -l) 个 .tex 引用了 _subdivision/（旧二进制产物），本次会一并删除"
  fi
fi
if [ "$do_backup" -eq 1 ]; then
  stamp="$(date +%Y%m%d_%H%M)"
  backup="$(dirname "$proj")/${book}_记录_$stamp"
  n=2
  while [ -e "$backup" ]; do backup="$(dirname "$proj")/${book}_记录_${stamp}_$n"; n=$((n+1)); done
  say "  备份：$backup（cp -a 整目录，$(du -sh "$proj" 2>/dev/null | cut -f1)）"
else
  warn "  --no-backup：不备份，删掉就没了"
fi

if [ "$dry_run" -eq 1 ]; then
  say ""
  say "（--dry-run：什么都没动）"
  exit 0
fi

if [ "$assume_yes" -eq 0 ]; then
  printf '\n继续？[y/N] '
  read -r ans
  case "$ans" in
    y|Y|yes|YES) ;;
    *) say "已取消"; exit 0 ;;
  esac
fi

# ---- 备份 ----------------------------------------------------------------
size_before=$(du -sh "$proj" 2>/dev/null | cut -f1)
if [ "$do_backup" -eq 1 ]; then
  step "备份 → $backup"
  cp -a "$proj" "$backup"
  say "  ok（$(du -sh "$backup" 2>/dev/null | cut -f1)）"
fi

# ---- 删除 ----------------------------------------------------------------
if [ "$keep_convert" -eq 1 ]; then
  step "删除章节转换之后的产物"
else
  step "删除章节转换及其之后的产物"
fi
for d in "${to_delete[@]}"; do
  if [ -e "$proj/$d" ]; then
    rm -rf -- "$proj/$d"
    say "  - $d"
  fi
done
for f in "${to_delete_after_files[@]}"; do
  if [ -e "$proj/$f" ]; then
    rm -f -- "$proj/$f"
    say "  - $f"
  fi
done
remove_glob() {
  local pat="$1"
  for f in "$proj"/$pat; do
    [ -e "$f" ] || continue
    rm -f -- "$f"
    say "  - work/sessions/$(basename "$f")"
  done
}
if [ "$keep_convert" -eq 0 ]; then
  remove_glob 'work/sessions/convert_*.jsonl'
  remove_glob 'work/sessions/checker_*.jsonl'
fi
remove_glob 'work/sessions/style_fix_*.jsonl'
if [ -e "$sibling" ]; then
  rm -f -- "$sibling"
  say "  - $sibling"
fi

# ---- 重置阶段标记 --------------------------------------------------------
step "重置 progress.json 的阶段标记"
python3 - "$proj/progress.json" <<'PY'
import json, os, sys

path = sys.argv[1]
try:
    with open(path, encoding="utf-8") as fh:
        prog = json.load(fh)
except (OSError, ValueError):
    prog = {}
if not isinstance(prog, dict):
    prog = {}
changed = []
for k in ("convert", "assemble"):
    if prog.get(k) != "":
        prog[k] = ""
        changed.append(k)
os.makedirs(os.path.dirname(path), exist_ok=True)
tmp = path + ".tmp"
with open(tmp, "w", encoding="utf-8") as fh:
    json.dump(prog, fh, ensure_ascii=False, indent=2)
os.replace(tmp, path)
print("  已归空：" + ("、".join(changed) if changed else "（本来就是空的）"))
print("  style/images/chapters 保持：%s" % {k: prog.get(k) for k in ("style", "images", "chapters")})
PY

# ---- 结果 ----------------------------------------------------------------
size_after=$(du -sh "$proj" 2>/dev/null | cut -f1)
step "完成"
say "  $size_before → $size_after"
say "  剩余的目录：$(cd "$proj" && ls -A | tr '\n' ' ')"
if [ "$keep_convert" -eq 1 ]; then
  say "  剩余章节：$(find "$proj/work/chapters" -maxdepth 1 -name 'chapter_*.tex' 2>/dev/null | wc -l) 章（保留）"
else
  say "  剩余章节：0 章（work/chapters 已清空）"
fi
say ""
if [ "$keep_convert" -eq 1 ]; then
  say "重新测试（分类/样式/划分/章节转换全部跳过；从风格修复大循环重跑）："
else
  say "重新测试（分类/样式/划分全部跳过；从逐章转换重跑，每章会重新开会话）："
fi
say "  cd /home/lingnc/workspace/kaoyan/docvision && ./docvision latex ./files/$book.pdf"
[ "$do_backup" -eq 1 ] && say "回退（清理后想还原）：rm -rf '$proj' && cp -a '$backup' '$proj'"
