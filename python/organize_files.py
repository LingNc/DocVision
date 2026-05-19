#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
MinerU 文件整理脚本 - 将 MinerU API 返回的 zip 解压文件整理到 output 目录
功能：
1. 从 mineru_output 目录复制 full.md -> output/temp/subject_partN.md
2. 合并同名科目的分片 -> output/subject.md
3. 按 markdown 引用收集图片 -> output/images/（仅复制实际引用的图片）

用法:
    python organize_files.py                    # 使用默认配置
    python organize_files.py --config my.yaml   # 指定配置文件
"""

import os
import re
import sys
import argparse
import shutil
from pathlib import Path
from collections import defaultdict

import yaml


def load_config(config_path: str = "config.yaml") -> dict:
    """加载配置文件"""
    with open(config_path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)


def organize_files(config: dict):
    """整理 MinerU 输出文件"""
    # 获取路径配置
    mineru_output = Path(config["paths"]["mineru_output"])
    output_dir = Path(config["paths"]["output_dir"])
    images_dir = Path(config["paths"]["images_dir"])
    temp_dir = output_dir / "temp"

    # 检查 mineru_output 目录是否存在
    if not mineru_output.exists():
        print(f"[错误] MinerU 输出目录不存在: {mineru_output}")
        return False

    # 清理并创建目录
    if output_dir.exists():
        shutil.rmtree(output_dir)
    temp_dir.mkdir(parents=True, exist_ok=True)
    images_dir.mkdir(parents=True, exist_ok=True)

    groups = defaultdict(list)

    print("=" * 50)
    print("  MinerU 文件整理工具")
    print("=" * 50)
    print()

    # ---- 步骤 1: 复制 md ----
    print(f"[1/4] 复制 full.md -> {temp_dir}/ ...")

    md_count = 0

    # 查找所有 _partN 目录
    part_dirs = [d for d in mineru_output.iterdir() if d.is_dir() and re.search(r'_part\d+', d.name)]

    for d in part_dirs:
        n = d.name
        match = re.match(r'^(.+?)_part(\d+)', n)
        if not match:
            continue

        sub = match.group(1)
        pn = match.group(2)

        print(f"  {sub} (part{pn})")

        if sub not in groups:
            groups[sub] = []
        groups[sub].append(pn)

        # 复制 full.md -> output/temp/subject_partN.md
        src_md = d / "full.md"
        dst_md = temp_dir / f"{sub}_{pn}.md"
        if src_md.exists():
            shutil.copy2(src_md, dst_md)
            md_count += 1
        else:
            print(f"    警告: 没有 full.md")

    print(f"  MD: {md_count}")
    print()

    # ---- 步骤 2: 合并分片 ----
    print(f"[2/4] 合并分片 -> {output_dir}/ ...")

    merge_count = 0

    for sub, parts in groups.items():
        # 按数字排序
        sorted_parts = sorted(parts, key=lambda x: int(x))

        if len(sorted_parts) == 1:
            # 单个分片，复制到 output 根目录（temp 中保留原件）
            src_file = temp_dir / f"{sub}_{sorted_parts[0]}.md"
            dst_file = output_dir / f"{sub}.md"
            if src_file.exists():
                shutil.copy2(str(src_file), str(dst_file))
                print(f"  {sub}_{sorted_parts[0]}.md -> {sub}.md")
            continue

        print(f"  合并: {sub} (parts {sorted_parts})")

        # 合并多个分片
        contents = []
        for p in sorted_parts:
            part_file = temp_dir / f"{sub}_{p}.md"
            if part_file.exists():
                text = part_file.read_text(encoding="utf-8")
                if text:
                    contents.append(text.rstrip())

        # 写入合并后的文件
        merged_file = output_dir / f"{sub}.md"
        merged_file.write_text("\n\n---\n\n".join(contents), encoding="utf-8")
        merge_count += 1

    print(f"  合并: {merge_count}")
    print()

    # ---- 步骤 3: 按 markdown 引用收集图片 ----
    print(f"[3/4] 收集引用的图片 -> {images_dir}/ ...")

    # 目的文件夹已存在且有内容，跳过收集（避免重复扫描和转移）
    if images_dir.exists() and any(images_dir.iterdir()):
        img_total = len(list(images_dir.iterdir()))
        print(f"  目的文件夹已存在 ({img_total} 张图片)，跳过收集")
        print()
    else:
        img_re = re.compile(r'!\[.*?\]\((images/[^)]+)\)')
        img_count = 0
        missing = 0

        # 建立图片源路径索引: image_name -> full_path（从所有 part 目录）
        img_source_map = {}
        for d in part_dirs:
            src_images = d / "images"
            if src_images.exists():
                for img_file in src_images.iterdir():
                    if img_file.is_file() and img_file.name not in img_source_map:
                        img_source_map[img_file.name] = img_file

        for md_file in sorted(output_dir.glob("*.md")):
            content = md_file.read_text(encoding="utf-8")
            refs = set(img_re.findall(content))
            for ref in refs:
                # ref = "images/xxx.jpg"
                img_name = ref.split("/", 1)[1] if "/" in ref else ref
                dst_img = images_dir / img_name
                if dst_img.exists():
                    continue
                src_img = img_source_map.get(img_name)
                if src_img:
                    shutil.copy2(src_img, dst_img)
                    img_count += 1
                else:
                    missing += 1
                    print(f"  警告: 找不到图片源 {ref}")

        print(f"  收集: {img_count} 张")
        if missing:
            print(f"  缺失: {missing} 张")
        print()

    # ---- 步骤 4: 汇总 ----
    print("[4/4] 汇总")
    print("=" * 50)

    print()
    print(f"  {output_dir}/ (合并后):")
    for md_file in sorted(output_dir.glob("*.md")):
        size_kb = md_file.stat().st_size / 1024
        print(f"    {md_file.name}  ({size_kb:.1f} KB)")

    print()
    print(f"  {temp_dir}/ (分片):")
    for md_file in sorted(temp_dir.glob("*.md")):
        size_kb = md_file.stat().st_size / 1024
        print(f"    {md_file.name}  ({size_kb:.1f} KB)")

    img_total = len(list(images_dir.iterdir()))
    print()
    print(f"  {images_dir}/ : {img_total} 张图片")
    print("=" * 50)
    print("完成!")

    return True


def main():
    parser = argparse.ArgumentParser(description="整理 MinerU 输出文件")
    parser.add_argument("--config", type=str, default="config.yaml", help="配置文件路径")
    args = parser.parse_args()

    config = load_config(args.config)
    success = organize_files(config)
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
