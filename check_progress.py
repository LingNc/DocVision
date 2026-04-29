#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
检查脚本：分析日志中的 ERROR/WARNING，核对 progress_items 中的进度。
Usage:
    python check_progress.py           # 默认分析最近一次运行的日志
    python check_progress.py --all     # 分析所有历史日志（汇总）
"""

import os
import re
import json
import argparse
from pathlib import Path
from datetime import datetime
from collections import defaultdict

import yaml

# 尝试导入 glob 用于查找文件
import glob

def load_config(config_path="config.yaml"):
    with open(config_path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)

def count_images_in_md_files(input_dir: Path):
    """计算所有 md 文件中的图片总数（与主程序一致的正则）"""
    IMAGE_RE = re.compile(r'!\[.*?\]\((images/[^)]+\.(?:jpg|jpeg|png|gif|webp))\)')
    total = 0
    for md_file in input_dir.glob("*.md"):
        content = md_file.read_text(encoding="utf-8")
        matches = IMAGE_RE.findall(content)
        total += len(matches)
    return total

def check_progress_items(progress_root: Path):
    """检查进度文件夹，返回 (completed_count, invalid_count, invalid_files, completed_images)"""
    completed = 0
    invalid = 0
    invalid_files = []
    completed_images = set()   # 新增：存储有效图片的 img_path
    # 遍历所有子目录下的 .json 文件
    for json_file in progress_root.glob("**/*.json"):
        try:
            with open(json_file, "r", encoding="utf-8") as f:
                data = json.load(f)
            result = data.get("result", "")
            # 检查是否有效：包含 "[IMG_TYPE:" 且不包含 "__INVALID_RESPONSE__"
            if "[IMG_TYPE:" in result and result != "__INVALID_RESPONSE__":
                completed += 1
                completed_images.add(data.get("img_path", ""))
            else:
                invalid += 1
                invalid_files.append(str(json_file))
        except Exception:
            invalid += 1
            invalid_files.append(str(json_file))
    return completed, invalid, invalid_files, completed_images

def get_problematic_images_from_log(log_path: Path) -> set:
    """从日志中提取出现过 ERROR 或 WARNING 的图片路径集合"""
    problematic = set()
    if not log_path.exists():
        return problematic
    with open(log_path, "r", encoding="utf-8") as f:
        lines = f.readlines()
    i = 0
    while i < len(lines):
        line = lines[i]
        # 匹配 "[1/10] images/xxx.jpg" 或类似格式
        m = re.search(r'\[\d+/\d+\]\s+(\S+\.(?:jpg|jpeg|png|gif|webp))', line, re.IGNORECASE)
        if m:
            current_img = m.group(1)
            has_error = False
            j = i + 1
            # 检查直到下一个图片处理行或文件结束
            while j < len(lines) and not re.search(r'\[\d+/\d+\]\s+\S+\.(?:jpg|jpeg|png|gif|webp)', lines[j], re.IGNORECASE):
                if "[ERROR]" in lines[j] or "[WARNING]" in lines[j]:
                    has_error = True
                j += 1
            if has_error:
                problematic.add(current_img)
            i = j
        else:
            i += 1
    return problematic

def find_log_files(log_dir: Path, only_latest=False):
    """查找所有 img2text_*.log 文件（排除 errors 日志）"""
    pattern = "img2text_*.log"
    all_logs = list(log_dir.glob(pattern))
    # 按修改时间排序，最新的在前
    all_logs.sort(key=lambda p: p.stat().st_mtime, reverse=True)
    if only_latest:
        return all_logs[:1] if all_logs else []
    return all_logs

def analyze_log_file(log_path: Path):
    """分析单个日志文件，返回 (total_lines, error_count, warning_count)"""
    error_cnt = 0
    warning_cnt = 0
    total = 0
    with open(log_path, "r", encoding="utf-8") as f:
        for line in f:
            total += 1
            if "[ERROR]" in line:
                error_cnt += 1
            elif "[WARNING]" in line:
                warning_cnt += 1
    return total, error_cnt, warning_cnt

def main():
    parser = argparse.ArgumentParser(description="Check progress and logs")
    parser.add_argument("--all", action="store_true", help="Analyze all historical logs (not only the latest)")
    parser.add_argument("--config", default="config.yaml", help="Path to config.yaml")
    args = parser.parse_args()

    config = load_config(args.config)
    input_dir = Path(config["paths"]["input_dir"])
    output_dir = Path(config["paths"]["output_dir"])
    progress_root = output_dir / "progress_items"

    # 1. 计算总图片数
    print("正在计算总图片数...")
    total_images = count_images_in_md_files(input_dir)
    print(f"总图片数: {total_images}")

    # 2. 检查进度文件夹
    if progress_root.exists():
        completed, invalid, invalid_files, completed_images = check_progress_items(progress_root)
        print(f"已完成有效图片数: {completed}")
        print(f"无效进度条目: {invalid}")
        if invalid > 0 and len(invalid_files) <= 10:
            for f in invalid_files:
                print(f"  - {f}")
        elif invalid > 10:
            print(f"  显示前10个无效文件: {invalid_files[:10]} ...")
        remaining = total_images - completed - invalid
        print(f"未完成图片数: {remaining} (包括未处理 + 无效)")
        if total_images > 0:
            pct = completed / total_images * 100
            print(f"完成率: {pct:.2f}%")
    else:
        print("进度文件夹不存在，尚未开始处理。")
        completed = 0
        invalid = 0
        completed_images = set()

    # 3. 分析日志并计算良品率
    log_files = find_log_files(output_dir, only_latest=not args.all)
    if not log_files:
        print("\n未找到日志文件。")
        return

    print("\n日志分析:")
    if args.all:
        print(f"分析所有 {len(log_files)} 个日志文件")
        total_lines = 0
        total_errors = 0
        total_warnings = 0
        problematic_images = set()
        for lf in log_files:
            lines, err, warn = analyze_log_file(lf)
            total_lines += lines
            total_errors += err
            total_warnings += warn
            problematic_images.update(get_problematic_images_from_log(lf))
        normal = total_lines - total_errors - total_warnings
        print(f"总日志行数: {total_lines}")
        print(f"ERROR 数量: {total_errors} ({total_errors/total_lines*100:.2f}%)" if total_lines>0 else "0")
        print(f"WARNING 数量: {total_warnings} ({total_warnings/total_lines*100:.2f}%)" if total_lines>0 else "0")
        print(f"正常行: {normal} ({normal/total_lines*100:.2f}%)" if total_lines>0 else "0")
        # 显示良品率
        if completed > 0:
            problematic_in_completed = completed_images.intersection(problematic_images)
            good_images = completed - len(problematic_in_completed)
            good_rate = good_images / completed * 100
            print(f"\n已完成图片中无错误/警告的良品率: {good_rate:.2f}% ({good_images}/{completed})")
    else:
        latest = log_files[0]
        lines, err, warn = analyze_log_file(latest)
        problematic_images = get_problematic_images_from_log(latest)
        normal = lines - err - warn
        print(f"最近日志: {latest.name}")
        print(f"总行数: {lines}")
        print(f"ERROR: {err} ({err/lines*100:.2f}%)" if lines>0 else "0")
        print(f"WARNING: {warn} ({warn/lines*100:.2f}%)" if lines>0 else "0")
        print(f"正常: {normal} ({normal/lines*100:.2f}%)" if lines>0 else "0")
        # 显示良品率
        if completed > 0:
            problematic_in_completed = completed_images.intersection(problematic_images)
            good_images = completed - len(problematic_in_completed)
            good_rate = good_images / completed * 100
            print(f"\n本次运行良品率: {good_rate:.2f}% ({good_images}/{completed})")

    # 4. 额外建议
    if invalid > 0:
        print("\n⚠ 发现无效进度条目，建议重新运行处理这些图片（脚本会自动忽略无效条目并重试）")
    if remaining > 0:
        print(f"\n剩余 {remaining} 张图片未完成，可继续运行主程序。")

if __name__ == "__main__":
    main()