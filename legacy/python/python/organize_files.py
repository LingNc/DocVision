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

    print("=" * 50)
    print("  MinerU 文件整理工具")
    print("=" * 50)
    print()

    # ---- 步骤 1: 发现目录并复制 md ----
    print(f"[1/4] 复制 full.md -> {temp_dir}/ ...")

    md_count = 0
    groups = defaultdict(list)
    all_dirs = []  # 所有含 full.md 的目录（用于图片索引）

    for d in sorted(mineru_output.iterdir()):
        if not d.is_dir():
            continue
        if not (d / "full.md").exists():
            continue

        all_dirs.append(d)

        match = re.match(r'^(.+?)_part(\d+)$', d.name)
        if match:
            # 分片目录: subject_partN
            sub, pn = match.group(1), match.group(2)
            groups[sub].append(pn)
            print(f"  {sub} (part{pn})")
            src_md = d / "full.md"
            dst_md = temp_dir / f"{sub}_{pn}.md"
            shutil.copy2(src_md, dst_md)
            md_count += 1
        else:
            # 单文件目录: 直接作为 subject（无分片）
            sub = d.name
            groups[sub] = []
            print(f"  {sub} (单文件)")
            src_md = d / "full.md"
            dst_md = temp_dir / f"{sub}.md"
            shutil.copy2(src_md, dst_md)
            md_count += 1

    print(f"  MD: {md_count}")
    print()

    # ---- 步骤 2: 合并分片 ----
    print(f"[2/4] 合并分片 -> {output_dir}/ ...")

    merge_count = 0
    skip_count = 0

    for sub, parts in groups.items():
        dst_file = output_dir / f"{sub}.md"

        # 最终文件已存在则跳过（想重新生成请删除 output/ 下对应的 .md 文件）
        if dst_file.exists():
            skip_count += 1
            continue

        if not parts:
            # 单文件（非分片），temp 中已有 sub.md，直接复制到 output
            src_file = temp_dir / f"{sub}.md"
            if src_file.exists():
                shutil.copy2(str(src_file), str(dst_file))
                print(f"  {sub}.md (单文件)")
            continue

        # 按数字排序
        sorted_parts = sorted(parts, key=lambda x: int(x))

        if len(sorted_parts) == 1:
            # 单个分片，复制到 output 根目录（temp 中保留原件）
            src_file = temp_dir / f"{sub}_{sorted_parts[0]}.md"
            if src_file.exists():
                shutil.copy2(str(src_file), str(dst_file))
                print(f"  {sub}.md (单分片)")
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

    print(f"  合并: {merge_count}, 跳过: {skip_count}")
    print()

    # ---- 步骤 3: 按文件收集图片到独立子目录 ----
    print(f"[3/4] 收集引用的图片 -> {images_dir}/ ...")

    img_re = re.compile(r'!\[.*?\]\((images/.+?\.(?:jpg|jpeg|png|gif|webp))\)')
    img_count = 0
    skip_subject = 0
    missing = 0

    # 建立图片源路径索引: image_name -> full_path（从所有输出目录）
    img_source_map = {}
    for d in all_dirs:
        src_images = d / "images"
        if src_images.exists():
            for img_file in src_images.iterdir():
                if img_file.is_file() and img_file.name not in img_source_map:
                    img_source_map[img_file.name] = img_file

    for md_file in sorted(output_dir.glob("*.md")):
        subject = md_file.stem
        subject_img_dir = images_dir / subject

        # 该文件的图片目录已存在且有内容，跳过
        if subject_img_dir.exists() and any(subject_img_dir.iterdir()):
            skip_subject += 1
            continue

        content = md_file.read_text(encoding="utf-8")
        refs = set(img_re.findall(content))
        if not refs:
            continue

        subject_img_dir.mkdir(parents=True, exist_ok=True)
        collected = 0

        for ref in refs:
            img_name = ref.split("/", 1)[1] if "/" in ref else ref
            dst_img = subject_img_dir / img_name
            if dst_img.exists():
                collected += 1
                continue
            src_img = img_source_map.get(img_name)
            if src_img:
                shutil.copy2(src_img, dst_img)
                img_count += 1
                collected += 1
            else:
                missing += 1
                print(f"  警告: [{subject}] 找不到图片源 {ref}")

        # 更新 md 中的图片引用路径: images/xxx.jpg -> images/subject/xxx.jpg
        new_content = content.replace(
            "images/", f"images/{subject}/"
        )
        if new_content != content:
            md_file.write_text(new_content, encoding="utf-8")

        print(f"  {subject}: {collected} 张图片")

    print(f"  收集: {img_count} 张, 跳过: {skip_subject} 个文件")
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

    img_total = 0
    print()
    print(f"  {images_dir}/ :")
    if images_dir.exists():
        for sub_dir in sorted(images_dir.iterdir()):
            if sub_dir.is_dir():
                count = len(list(sub_dir.iterdir()))
                img_total += count
                print(f"    {sub_dir.name}/ ({count} 张)")
    print(f"    共 {img_total} 张图片")
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
