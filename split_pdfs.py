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


def split_pdf(pdf_path: str, max_pages: int = MAX_PAGES, output_dir: str = OUTPUT_DIR):
    """将单个 PDF 按 max_pages 页分割。"""
    if not os.path.exists(pdf_path):
        print(f"[错误] 文件不存在: {pdf_path}")
        return

    os.makedirs(output_dir, exist_ok=True)

    doc = pymupdf.open(pdf_path)
    total = doc.page_count
    base_name = os.path.splitext(os.path.basename(pdf_path))[0]

    print(f"[信息] {os.path.basename(pdf_path)}: 共 {total} 页")

    part = 1
    start = 0

    while start < total:
        end = min(start + max_pages, total)
        part_doc = pymupdf.open()  # 新建空 PDF
        part_doc.insert_pdf(doc, from_page=start, to_page=end - 1)

        part_name = f"{base_name}_part{part}.pdf"
        part_path = os.path.join(output_dir, part_name)
        part_doc.save(part_path)
        part_doc.close()

        print(f"  → {part_name}  (页 {start + 1}–{end}, 共 {end - start} 页)")
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
    parser.add_argument(
        "--max-pages", type=int, default=MAX_PAGES,
        help=f"每部分最大页数（默认 {MAX_PAGES}）"
    )
    parser.add_argument(
        "--output-dir", default=default_output_dir,
        help=f"输出目录（默认从 config.yaml 的 paths.split_dir: {default_output_dir}）"
    )
    parser.add_argument(
        "--input-dir", default=default_input_dir,
        help=f"--all 模式下的输入目录（默认从 config.yaml 的 paths.input_dir: {default_input_dir}）"
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
                args.output_dir,
            )
    elif args.pdf:
        split_pdf(
            args.pdf if os.path.isabs(args.pdf) else os.path.join(cwd, args.pdf),
            args.max_pages,
            args.output_dir,
        )
    else:
        parser.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
