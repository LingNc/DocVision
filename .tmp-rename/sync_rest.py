#!/usr/bin/env python3
"""对齐剩下两处派生名字：MinerU 的任务状态 sidecar、档位2 的图片账本。

1. mineru_output/status/<part>_task.json —— process.go 用 `<part 文件名主干>_task.json`
   记录任务 id；不改名的话以后真重解析会当"没提交过"重新上传。
2. finally/progress_items/<书名>.md/<键>.json —— LoadProgress 按 JSON 里的
   key 索引（不认目录名），所以键必须跟着 md 名一起改，否则档位2 重跑会把
   这批图全部重做。

output/temp/ 下的东西每次 organize 都会重建，不动。

用法：python3 sync_rest.py --run-dir <运行目录> [--apply]
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
    apply = a.apply

    # 1) MinerU 任务状态
    status = os.path.join(R, "mineru_output", "status")
    st_plan = []
    if os.path.isdir(status):
        for n in sorted(os.listdir(status)):
            if n.startswith(OLD_DIR) and n.endswith("_task.json"):
                st_plan.append((os.path.join(status, n),
                                os.path.join(status, NEW_DIR + n[len(OLD_DIR):])))
    for o, n in st_plan:
        print(f"[状态] {os.path.basename(o)!r} -> {os.path.basename(n)!r}")

    # 2) 档位2 图片账本
    root = os.path.join(R, "finally", "progress_items")
    old_dir = os.path.join(root, OLD_DIR + ".md")
    new_dir = os.path.join(root, NEW_DIR + ".md")
    lg_plan = []
    changed_fields = 0
    if os.path.isdir(old_dir):
        for n in sorted(os.listdir(old_dir)):
            p = os.path.join(old_dir, n)
            if not n.endswith(".json"):
                continue
            d = json.load(open(p, encoding="utf-8"))
            nl = {}
            for k, v in d.items():
                if isinstance(v, str):
                    nv = fix(v)
                    if nv != v:
                        changed_fields += 1
                    nl[k] = nv
                else:
                    nl[k] = v
            lg_plan.append((p, nl))
        print(f"[账本] {os.path.basename(old_dir)!r} -> {os.path.basename(new_dir)!r}，"
              f"{len(lg_plan)} 条，改写字段 {changed_fields} 个")
    else:
        print("[账本] 没有这本书的档位2 账本目录（跳过）")

    if not apply:
        print("\n[dry-run] 没写任何文件；确认无误后加 --apply")
        return 0

    for o, n in st_plan:
        if os.path.exists(o) and not os.path.exists(n):
            os.rename(o, n)
    print(f"[状态] 改名 {len(st_plan)} 个")

    if lg_plan:
        os.makedirs(new_dir, exist_ok=True)
        for p, d in lg_plan:
            dst = os.path.join(new_dir, os.path.basename(p))
            with open(dst, "w", encoding="utf-8") as f:
                json.dump(d, f, ensure_ascii=False, indent=2)
                f.write("\n")
            if os.path.abspath(p) != os.path.abspath(dst):
                os.remove(p)
        for n in os.listdir(old_dir):
            print("!! 旧目录还有文件:", n)
        if not os.listdir(old_dir):
            os.rmdir(old_dir)
        print(f"[账本] 写入 {len(lg_plan)} 条到 {new_dir}")

    print("\n完成。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
