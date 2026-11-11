#!/usr/bin/env python3
"""收尾：把上一步漏掉的两处旧名改干净。

上一版脚本只改 JSON 的「值」没改「键」，`scan.json` 的 `pairs` 恰好是
"<哈希>\\u0000<md>::<图片路径>" 这样的键，于是旧名留在键上。这里补上：

  1. source/dedupe/scan.json  —— 键和值一起改（含 NUL 分隔的对键）
  2. source/dedupe/report.md / review.html —— 顺带把已经安全化过的旧键
     （`27闪过-长难句_.md`、`images_27闪过-长难句__…`）也统一
  3. work/style/pilot/sample.md —— 试点样本里的插图引用

历史转录（source/sessions/、work/sessions/）保持不变：它们是记录，不是账本。

用法：python3 finish_leftovers.py --proj <项目目录> [--apply]
"""
import argparse
import json
import os
import sys

OLD_MD = "27闪过-长难句 .md"
NEW_MD = "27闪过-长难句.md"
OLD_DIR = "27闪过-长难句 "
NEW_DIR = "27闪过-长难句"
OLD_REF = "images/" + OLD_DIR + "/"
NEW_REF = "images/" + NEW_DIR + "/"
# 已经过 sanitizeName 的旧形态（空格变下划线）
OLD_SAN_MD = "27闪过-长难句_.md"
NEW_SAN_MD = "27闪过-长难句.md"
OLD_SAN_IMG = "images_27闪过-长难句__"
NEW_SAN_IMG = "images_27闪过-长难句_"


def fix_str(v: str) -> str:
    return (
        v.replace(OLD_MD, NEW_MD)
        .replace(OLD_REF, NEW_REF)
        .replace(OLD_SAN_MD, NEW_SAN_MD)
        .replace(OLD_SAN_IMG, NEW_SAN_IMG)
    )


def fix_obj(obj):
    """键和值都改；改名撞车直接报错，不静默丢数据。"""
    if isinstance(obj, str):
        return fix_str(obj)
    if isinstance(obj, list):
        return [fix_obj(x) for x in obj]
    if isinstance(obj, dict):
        out = {}
        for k, v in obj.items():
            nk = fix_str(k) if isinstance(k, str) else k
            if nk in out:
                raise SystemExit(f"键改名撞车: {k!r} -> {nk!r}")
            out[nk] = fix_obj(v)
        return out
    return obj


def count_old(text: str) -> int:
    return (
        text.count(OLD_MD)
        + text.count(OLD_REF)
        + text.count(OLD_SAN_MD)
        + text.count(OLD_SAN_IMG)
    )


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--proj", required=True)
    ap.add_argument("--apply", action="store_true")
    a = ap.parse_args()
    P = os.path.abspath(a.proj)
    src = os.path.join(P, "source")
    dd = os.path.join(src, "dedupe")

    jobs = []  # (path, kind, new_text, before)
    for name in ("scan.json", "groups.json"):
        path = os.path.join(dd, name)
        if not os.path.exists(path):
            continue
        raw = open(path, encoding="utf-8").read()
        obj = json.loads(raw)
        fixed = json.dumps(fix_obj(obj), ensure_ascii=False, indent=2) + "\n"
        jobs.append((path, "json", fixed, count_old(raw)))
        print(f"[json] {name}: 旧名 {count_old(raw)} 处 -> {count_old(fixed)} 处")

    for name in ("report.md", "review.html"):
        path = os.path.join(dd, name)
        if not os.path.exists(path):
            continue
        raw = open(path, encoding="utf-8").read()
        fixed = fix_str(raw)
        jobs.append((path, "text", fixed, count_old(raw)))
        print(f"[text] {name}: 旧名 {count_old(raw)} 处 -> {count_old(fixed)} 处")

    sample = os.path.join(P, "work", "style", "pilot", "sample.md")
    if os.path.exists(sample):
        raw = open(sample, encoding="utf-8").read()
        fixed = fix_str(raw)
        jobs.append((sample, "text", fixed, count_old(raw)))
        print(f"[试点样本] sample.md: 旧名 {count_old(raw)} 处 -> {count_old(fixed)} 处")

    if not a.apply:
        print("\n[dry-run] 没写任何文件；确认无误后加 --apply")
        return 0

    for path, _, text, _ in jobs:
        with open(path, "w", encoding="utf-8") as f:
            f.write(text)
        print("已写", path)

    print("\n完成。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
