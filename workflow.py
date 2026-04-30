#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
MinerU 工作流主脚本 - 串联整个 PDF 处理流程
工作流程：
1. 分割 PDF 文件（如果需要）
2. 调用 MinerU API 解析 PDF
3. 整理 MinerU 输出文件
4. 使用 AI 将图片转换为文本
5. 分析处理日志

用法:
    python workflow.py                      # 运行完整工作流
    python workflow.py --step split         # 仅运行分割步骤
    python workflow.py --step mineru        # 仅运行 MinerU API 步骤
    python workflow.py --step organize      # 仅运行文件整理步骤
    python workflow.py --step img2text      # 仅运行图片转文本步骤
    python workflow.py --step analyze       # 仅运行日志分析步骤
    python workflow.py --config my.yaml     # 指定配置文件
"""

import os
import sys
import argparse
import subprocess
from pathlib import Path
from typing import List, Optional

import yaml


def load_config(config_path: str = "config.yaml") -> dict:
    """加载配置文件"""
    with open(config_path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)


def run_command(cmd: List[str], cwd: Optional[str] = None) -> bool:
    """
    运行命令

    Args:
        cmd: 命令列表
        cwd: 工作目录

    Returns:
        是否成功
    """
    try:
        print(f"运行: {' '.join(cmd)}")
        env = os.environ.copy()
        env["PYTHONUNBUFFERED"] = "1"
        result = subprocess.run(cmd, cwd=cwd, capture_output=False, text=True, env=env)
        return result.returncode == 0
    except Exception as e:
        print(f"命令执行失败: {e}")
        return False


def step_split(config: dict) -> bool:
    """
    步骤 1: 分割 PDF 文件

    Args:
        config: 配置字典

    Returns:
        是否成功
    """
    print("\n" + "=" * 50)
    print("步骤 1: 分割 PDF 文件")
    print("=" * 50)

    input_dir = Path(config["paths"]["input_dir"])
    split_dir = Path(config["paths"]["split_dir"])
    max_pages = config["mineru"]["max_pages_per_part"]

    # 检查输入目录
    if not input_dir.exists():
        print(f"[错误] 输入目录不存在: {input_dir}")
        return False

    pdf_files = list(input_dir.glob("*.pdf"))
    if not pdf_files:
        print(f"[警告] 没有找到 PDF 文件: {input_dir}")
        return True

    print(f"找到 {len(pdf_files)} 个 PDF 文件")

    # 逐个处理 PDF 文件
    all_success = True
    for pdf_file in pdf_files:
        cmd = [
            sys.executable, "split_pdfs.py",
            str(pdf_file),
            "--max-pages", str(max_pages),
            "--output-dir", str(split_dir)
        ]
        if not run_command(cmd):
            all_success = False

    return all_success


def step_mineru(config: dict) -> bool:
    """
    步骤 2: 调用 MinerU API 解析 PDF

    Args:
        config: 配置字典

    Returns:
        是否成功
    """
    print("\n" + "=" * 50)
    print("步骤 2: 调用 MinerU API 解析 PDF")
    print("=" * 50)

    # 运行 MinerU API 脚本
    cmd = [sys.executable, "mineru_api.py"]
    return run_command(cmd)


def step_organize(config: dict) -> bool:
    """
    步骤 3: 整理 MinerU 输出文件

    Args:
        config: 配置字典

    Returns:
        是否成功
    """
    print("\n" + "=" * 50)
    print("步骤 3: 整理 MinerU 输出文件")
    print("=" * 50)

    # 运行文件整理脚本
    cmd = [sys.executable, "organize_files.py"]
    return run_command(cmd)


def step_img2text(config: dict) -> bool:
    """
    步骤 4: 使用 AI 将图片转换为文本

    Args:
        config: 配置字典

    Returns:
        是否成功
    """
    print("\n" + "=" * 50)
    print("步骤 4: 使用 AI 将图片转换为文本")
    print("=" * 50)

    # 运行图片转文本脚本
    cmd = [sys.executable, "img2text.py"]
    return run_command(cmd)


def step_analyze(config: dict) -> bool:
    """
    步骤 5: 分析处理日志

    Args:
        config: 配置字典

    Returns:
        是否成功
    """
    print("\n" + "=" * 50)
    print("步骤 5: 分析处理日志")
    print("=" * 50)

    # 运行日志分析脚本
    cmd = [sys.executable, "analyze.py", "--progress"]
    return run_command(cmd)


def run_full_workflow(config: dict) -> bool:
    """
    运行完整工作流

    Args:
        config: 配置字典

    Returns:
        是否成功
    """
    print("=" * 50)
    print("MinerU 完整工作流")
    print("=" * 50)

    steps = [
        ("分割 PDF 文件", step_split),
        ("调用 MinerU API", step_mineru),
        ("整理输出文件", step_organize),
        ("图片转文本", step_img2text),
        ("分析日志", step_analyze),
    ]

    for step_name, step_func in steps:
        print(f"\n>>> 开始: {step_name}")
        success = step_func(config)
        if not success:
            print(f"\n[错误] 步骤失败: {step_name}")
            return False
        print(f">>> 完成: {step_name}")

    print("\n" + "=" * 50)
    print("工作流完成!")
    print("=" * 50)
    return True


def main():
    parser = argparse.ArgumentParser(description="MinerU 工作流主脚本")
    parser.add_argument("--config", type=str, default="config.yaml", help="配置文件路径")
    parser.add_argument("--step", type=str, choices=["split", "mineru", "organize", "img2text", "analyze"],
                        help="仅运行指定步骤")
    args = parser.parse_args()

    # 加载配置
    config = load_config(args.config)

    # 确保在正确的目录中运行
    script_dir = Path(__file__).parent
    os.chdir(script_dir)

    if args.step:
        # 运行单个步骤
        step_map = {
            "split": step_split,
            "mineru": step_mineru,
            "organize": step_organize,
            "img2text": step_img2text,
            "analyze": step_analyze,
        }
        step_func = step_map[args.step]
        success = step_func(config)
    else:
        # 运行完整工作流
        success = run_full_workflow(config)

    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
