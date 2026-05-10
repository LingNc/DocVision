#!/usr/bin/env python3
"""
PDF 分割脚本 —— 将大 PDF 按不超过 MAX_PAGES 页分割为多个部分。

用法:
    # 分割单个 PDF
    python split_pdfs.py 2027数据结构_高清带书签版.pdf

    # 分割目录下所有 PDF（默认从 config.yaml 的 paths.input_dir 读取）
    python split_pdfs.py --all

    # 指定最大页数（默认 200）
    python split_pdfs.py 2027数据结构_高清带书签版.pdf --max-pages 150

输出目录: 从 config.yaml 的 paths.split_dir 读取，默认 ./split_parts/
"""

import argparse
import os
import sys
from pathlib import Path

import pymupdf
import yaml

MAX_PAGES = 200
MAX_SIZE_MB = 200  # MinerU API 文件大小限制 (MB)
OUTPUT_DIR = "split_parts"
INPUT_DIR = "."  # --all 模式的默认输入目录


def load_config(config_path: str = "config.yaml") -> dict:
    """加载配置文件，如果不存在则返回空字典"""
    try:
        with open(config_path, "r", encoding="utf-8") as f:
            return yaml.safe_load(f)
    except FileNotFoundError:
        return {}
    except Exception as e:
        print(f"[警告] 读取配置文件失败: {e}")
        return {}


def get_default_dirs() -> tuple:
    """
    从配置文件获取默认目录
    Returns: (input_dir, split_dir)
    """
    config = load_config()
    paths = config.get("paths", {})
    input_dir = paths.get("input_dir", INPUT_DIR)
    split_dir = paths.get("split_dir", OUTPUT_DIR)
    return input_dir, split_dir


def _estimate_part_size_mb(doc, from_page: int, to_page: int) -> float:
    """估算 doc 中 from_page~to_page 范围写入 PDF 后的大致大小 (MB)。"""
    part_doc = pymupdf.open()
    part_doc.insert_pdf(doc, from_page=from_page, to_page=to_page)
    # 用 incremental=False 写到内存来获取真实大小
    pdf_bytes = part_doc.tobytes()
    part_doc.close()
    return len(pdf_bytes) / (1024 * 1024)


def _split_by_size_and_pages(doc, start: int, total: int, max_pages: int, max_size_mb: float):
    """
    从 start 开始，找出不超过 max_pages 且不超过 max_size_mb 的结束页。
    采用二分法在 [start+1, start+max_pages] 范围内查找。
    Returns: end page (exclusive)
    """
    end = min(start + max_pages, total)

    if max_size_mb <= 0:
        return end

    # 先检查单页是否就超限（极端情况）
    single_page_mb = _estimate_part_size_mb(doc, start, start)
    if single_page_mb > max_size_mb:
        print(f"  [警告] 第 {start + 1} 页单页大小 {single_page_mb:.1f}MB 已超过限制 {max_size_mb}MB，仍将单独输出")
        return start + 1

    # 检查整个范围是否不超限
    full_mb = _estimate_part_size_mb(doc, start, end - 1)
    if full_mb <= max_size_mb:
        return end

    # 二分查找最大不超限的 end
    lo, hi = start + 1, end
    best = start + 1
    while lo <= hi:
        mid = (lo + hi) // 2
        size_mb = _estimate_part_size_mb(doc, start, mid - 1)
        if size_mb <= max_size_mb:
            best = mid
            lo = mid + 1
        else:
            hi = mid - 1

    return best


def split_pdf(pdf_path: str, max_pages: int = MAX_PAGES, max_size_mb: float = MAX_SIZE_MB,
              output_dir: str = OUTPUT_DIR, force: bool = False):
    """将单个 PDF 按 max_pages 页和 max_size_mb 大小分割。

    Args:
        pdf_path: PDF 文件路径
        max_pages: 每部分最大页数
        max_size_mb: 每部分最大大小 (MB)，0 表示不限制
        output_dir: 输出目录
        force: 是否强制重新分割（忽略已有文件）
    """
    if not os.path.exists(pdf_path):
        print(f"[错误] 文件不存在: {pdf_path}")
        return

    os.makedirs(output_dir, exist_ok=True)

    doc = pymupdf.open(pdf_path)
    total = doc.page_count
    base_name = os.path.splitext(os.path.basename(pdf_path))[0]

    # ── 检测是否已经分割过 ──
    if not force:
        # 用页数估算预期的 part 数，然后检查文件是否存在
        existing_parts = sorted(
            Path(output_dir).glob(f"{base_name}_part*.pdf")
        )
        if existing_parts:
            # 验证已有 part 的页数总和是否等于总页数
            existing_total_pages = 0
            for p in existing_parts:
                try:
                    d = pymupdf.open(str(p))
                    existing_total_pages += d.page_count
                    d.close()
                except Exception:
                    existing_total_pages = -1
                    break
            if existing_total_pages == total:
                print(f"[跳过] {os.path.basename(pdf_path)}: 已分割为 {len(existing_parts)} 个部分，跳过")
                doc.close()
                return
            else:
                print(f"[信息] {os.path.basename(pdf_path)}: 已有 {len(existing_parts)} 个部分但页数不匹配({existing_total_pages}≠{total})，重新分割")

    print(f"[信息] {os.path.basename(pdf_path)}: 共 {total} 页")

    part = 1
    start = 0

    while start < total:
        end = _split_by_size_and_pages(doc, start, total, max_pages, max_size_mb)
        part_doc = pymupdf.open()  # 新建空 PDF
        part_doc.insert_pdf(doc, from_page=start, to_page=end - 1)

        part_name = f"{base_name}_part{part}.pdf"
        part_path = os.path.join(output_dir, part_name)
        part_doc.save(part_path)
        part_size_mb = os.path.getsize(part_path) / (1024 * 1024)
        part_doc.close()

        size_info = f", {part_size_mb:.1f}MB" if max_size_mb > 0 else ""
        print(f"  → {part_name}  (页 {start + 1}–{end}, 共 {end - start} 页{size_info})")
        start = end
        part += 1

    doc.close()
    print(f"[完成] 共分割为 {part - 1} 个部分，输出到 {output_dir}/\n")


def main():
    # 先获取配置默认值
    default_input_dir, default_output_dir = get_default_dirs()

    parser = argparse.ArgumentParser(description="将大 PDF 按页数分割")
    parser.add_argument(
        "pdf", nargs="?", help="要分割的 PDF 文件路径（使用 --all 时可选）"
    )
    parser.add_argument(
        "--all", action="store_true", help=f"分割目录下所有 PDF（默认从 config.yaml 的 paths.input_dir: {default_input_dir}）"
    )
    # 从配置读取默认值
    mineru_cfg = load_config().get("mineru", {})
    default_max_size = mineru_cfg.get("max_size_mb", MAX_SIZE_MB)

    parser.add_argument(
        "--max-pages", type=int, default=MAX_PAGES,
        help=f"每部分最大页数（默认 {MAX_PAGES}）"
    )
    parser.add_argument(
        "--max-size-mb", type=float, default=default_max_size,
        help=f"每部分最大大小 MB（默认 {default_max_size}，0=不限制）"
    )
    parser.add_argument(
        "--output-dir", default=default_output_dir,
        help=f"输出目录（默认从 config.yaml 的 paths.split_dir: {default_output_dir}）"
    )
    parser.add_argument(
        "--input-dir", default=default_input_dir,
        help=f"--all 模式下的输入目录（默认从 config.yaml 的 paths.input_dir: {default_input_dir}）"
    )
    parser.add_argument(
        "--force", action="store_true", help="强制重新分割（忽略已有文件）"
    )
    args = parser.parse_args()

    cwd = os.getcwd()

    if args.all:
        # 使用指定的输入目录
        input_dir = args.input_dir
        if not os.path.isabs(input_dir):
            input_dir = os.path.join(cwd, input_dir)

        if not os.path.exists(input_dir):
            print(f"[错误] 输入目录不存在: {input_dir}")
            sys.exit(1)

        pdf_files = sorted(
            f for f in os.listdir(input_dir)
            if f.lower().endswith(".pdf") and not f.startswith(".")
        )
        if not pdf_files:
            print(f"[错误] 目录 {input_dir} 下没有找到 PDF 文件")
            sys.exit(1)
        print(f"找到 {len(pdf_files)} 个 PDF 文件\n")
        for pdf_file in pdf_files:
            split_pdf(
                os.path.join(input_dir, pdf_file),
                args.max_pages,
                args.max_size_mb,
                args.output_dir,
                force=args.force,
            )
    elif args.pdf:
        split_pdf(
            args.pdf if os.path.isabs(args.pdf) else os.path.join(cwd, args.pdf),
            args.max_pages,
            args.max_size_mb,
            args.output_dir,
            force=args.force,
        )
    else:
        parser.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
