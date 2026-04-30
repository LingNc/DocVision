#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
按线程ID分割日志文件。
默认处理 output 目录中最新的 img2text_*.log，也可通过 --logfile 指定。
输出：在日志文件所在目录创建同名文件夹（去掉 .log），每个线程生成
      Txx_<日志名>.log 文件（T00 为主线程/其它未命名线程）。
"""

import re
import sys
import argparse
from pathlib import Path
from collections import defaultdict

# 匹配日志行开头的线程ID，例如 [17:43:06][T01]
THREAD_PAT = re.compile(r'^\[.*?\]\[(T\d+)\]')


def find_latest_log(output_dir: Path) -> Path:
    """在指定目录查找最新的 img2text_*.log"""
    logs = sorted(output_dir.glob("img2text_*.log"), key=lambda p: p.stat().st_mtime, reverse=True)
    if not logs:
        raise FileNotFoundError(f"No img2text_*.log found in {output_dir}")
    return logs[0]


def split_log(log_path: Path):
    # 读取全部行
    with open(log_path, 'r', encoding='utf-8') as f:
        lines = f.readlines()

    # 按线程分组，同时保持原始顺序（组内追加）
    thread_lines = defaultdict(list)

    for line in lines:
        m = THREAD_PAT.match(line)
        tid = m.group(1) if m else "T00"  # 没有线程ID的行归入 T00
        thread_lines[tid].append(line)

    # 准备输出文件夹：与日志文件同名（去掉 .log 后缀）
    base_name = log_path.stem  # 例如 img2text_20260429_174305
    out_dir = log_path.parent / base_name
    out_dir.mkdir(exist_ok=True)

    # 为每个线程写出文件
    for tid in sorted(thread_lines.keys()):
        out_file = out_dir / f"{tid}_{base_name}.log"
        with open(out_file, 'w', encoding='utf-8') as f:
            f.writelines(thread_lines[tid])
        print(f"  Wrote {len(thread_lines[tid])} lines to {out_file.name}")

    print(f"\nSplit into {len(thread_lines)} thread files in {out_dir}/")


def main():
    parser = argparse.ArgumentParser(description="Split log by thread ID")
    parser.add_argument("--logfile", type=str, help="Path to log file")
    parser.add_argument("--output-dir", type=str, default=None,
                        help="Directory containing logs (default: from config.yaml or current dir)")
    parser.add_argument("--config", type=str, default="config.yaml",
                        help="Config file to read output_dir (used when --logfile not given)")
    args = parser.parse_args()

    if args.logfile:
        log_path = Path(args.logfile)
    else:
        # 尝试从 config 读取 output_dir
        out_dir = Path.cwd() / "finally"   # 默认 fallback
        if Path(args.config).exists():
            import yaml
            try:
                with open(args.config, 'r', encoding='utf-8') as f:
                    config = yaml.safe_load(f)
                out_dir = Path(config["paths"]["output_dir"])
            except Exception:
                pass  # 使用默认

        log_path = find_latest_log(out_dir)
        print(f"Using latest log: {log_path.name}")

    if not log_path.exists():
        print(f"Error: log file not found: {log_path}")
        sys.exit(1)

    print(f"Splitting {log_path}")
    split_log(log_path)
    print("Done.")


if __name__ == "__main__":
    main()