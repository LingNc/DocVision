#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
一次性迁移脚本：将旧的 _progress.json 转换为独立文件存储（按 MD 文件分文件夹）
使用前请备份原有的 _progress.json 文件。
"""

import os
import json
import shutil
from pathlib import Path

def migrate_progress(old_progress_path: Path, new_progress_root: Path):
    """
    将 old_progress_path (_progress.json) 中的条目迁移到 new_progress_root 目录下，
    每个图片保存为一个 JSON 文件，按原 MD 文件名字（去掉 .md）创建子目录。
    """
    if not old_progress_path.exists():
        print(f"错误：找不到旧的进度文件 {old_progress_path}")
        return False

    # 加载旧进度
    with open(old_progress_path, "r", encoding="utf-8") as f:
        old_progress = json.load(f)

    if not old_progress:
        print("旧进度文件为空，无需迁移")
        return True

    # 创建新进度根目录
    new_progress_root.mkdir(parents=True, exist_ok=True)

    migrated_count = 0
    error_count = 0

    for key, info in old_progress.items():
        # key 格式: "2027操作系统.md::images/xxx.jpg"
        try:
            parts = key.split("::")
            if len(parts) != 2:
                print(f"跳过无效的 key: {key}")
                error_count += 1
                continue
            md_filename = parts[0]          # "2027操作系统.md"
            img_rel_path = parts[1]         # "images/xxx.jpg"

            # 子目录名：去掉 .md 后缀
            subdir_name = md_filename.replace(".md", "")
            subdir = new_progress_root / subdir_name
            subdir.mkdir(exist_ok=True)

            # 安全的文件名：将图片路径中的 '/' 替换为 '_'
            safe_img_name = img_rel_path.replace("/", "_").replace("\\", "_")
            target_file = subdir / f"{safe_img_name}.json"

            # 准备保存的数据（与主程序中的格式一致）
            item_data = {
                "key": key,
                "result": info.get("result", ""),
                "start": info.get("start", 0),
                "end": info.get("end", 0),
                "img_path": info.get("img_path", "")
            }

            # 原子写入：先写临时文件，再替换
            tmp_file = target_file.with_suffix(".tmp")
            with open(tmp_file, "w", encoding="utf-8") as f:
                json.dump(item_data, f, ensure_ascii=False, indent=2)
            os.replace(tmp_file, target_file)

            migrated_count += 1
            if migrated_count % 1000 == 0:
                print(f"已迁移 {migrated_count} 条...")
        except Exception as e:
            print(f"迁移 key '{key}' 时出错: {e}")
            error_count += 1
            continue

    print(f"迁移完成。成功：{migrated_count} 条，失败：{error_count} 条。")
    # 迁移成功后，可选择备份或删除旧文件
    backup_path = old_progress_path.with_suffix(".json.bak")
    shutil.copy2(old_progress_path, backup_path)
    print(f"旧进度文件已备份到 {backup_path}")
    # 如果想直接删除旧文件，取消下面一行的注释
    # old_progress_path.unlink()

    return error_count == 0

if __name__ == "__main__":
    # 请根据你的实际路径修改下面的值，或者通过命令行参数传入
    # 通常 outdir 是主程序配置中的输出目录，例如 "output"
    OUTDIR = Path("finally_old")          # 替换为你的实际输出目录
    OLD_PROGRESS = OUTDIR / "_progress.json"
    NEW_PROGRESS_ROOT = OUTDIR / "progress_items"

    if not OUTDIR.exists():
        print(f"错误：输出目录 {OUTDIR} 不存在")
        exit(1)

    print(f"旧进度文件: {OLD_PROGRESS}")
    print(f"新进度根目录: {NEW_PROGRESS_ROOT}")
    confirm = input("确认开始迁移？(yes/no): ")
    if confirm.lower() != "yes":
        print("退出")
        exit(0)

    migrate_progress(OLD_PROGRESS, NEW_PROGRESS_ROOT)
