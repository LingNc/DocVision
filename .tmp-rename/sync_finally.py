#!/usr/bin/env python3
"""档位2（finally）的两个账本目录改名 + 键同步。

finally/progress_items/ 下这本书有两个目录：
  · "<书名> "        —— 89 条逐图结果（JSON 里的 key / img_path 带旧名）
  · "<书名> .md"     —— 原 md 的快照（original.md，正文里的插图引用带旧名）

LoadProgress 按 JSON 里的 key 索引、不认目录名，所以键必须跟着改，否则档位2
重跑会把这一整批图重做。原 md 快照只影响"这张图对应哪一段"的定位，也一起改。

用法：python3 sync_finally.py --run-dir <运行目录> [--apply]
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


def fix(v: str) -> str:
    return v.replace(OLD_MD, NEW_MD).replace(OLD_REF, NEW_REF)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--run-dir", required=True)
    ap.add_argument("--apply", action="store_true")
    a = ap.parse_args()
    R = os.path.abspath(a.run_dir)
    root = os.path.join(R, "finally", "progress_items")
    apply = a.apply

    jobs = []  # (old_path, new_path, new_bytes_or_None, kind)
    for name in (OLD_DIR, OLD_MD):
        old = os.path.join(root, name)
        new = os.path.join(root, name.replace(OLD_DIR, NEW_DIR, 1).rstrip())
        if not os.path.isdir(old):
            continue
        for fn in sorted(os.listdir(old)):
            p = os.path.join(old, fn)
            if not os.path.isfile(p):
                continue
            raw = open(p, encoding="utf-8").read()
            if fn.endswith(".json"):
                d = json.loads(raw)
                nl = {k: (fix(v) if isinstance(v, str) else v) for k, v in d.items()}
                out = json.dumps(nl, ensure_ascii=False, indent=2) + "\n"
                kind = "json"
            else:
                out = fix(raw)
                kind = "text"
            jobs.append((old, new, fn, out, kind))
    print(f"待处理 {len(jobs)} 个文件，涉及目录：" +
          str(sorted({os.path.basename(j[0]) for j in jobs})))

    if not apply:
        print("[dry-run] 没写任何文件；确认无误后加 --apply")
        return 0

    touched_dirs = set()
    for old, new, fn, out, _ in jobs:
        os.makedirs(new, exist_ok=True)
        with open(os.path.join(new, fn), "w", encoding="utf-8") as f:
            f.write(out)
        os.remove(os.path.join(old, fn))
        touched_dirs.add(old)
    for d in touched_dirs:
        rest = os.listdir(d)
        if rest:
            print("!! 旧目录还有:", d, rest)
        else:
            os.rmdir(d)
    for d in sorted(os.listdir(root)):
        pass
    print(f"完成：{len(jobs)} 个文件已搬到新目录并改写。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
