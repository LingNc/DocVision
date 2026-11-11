#!/usr/bin/env python3
"""把一本书里带空格的名字一次性改干净（md 名、插图目录、账本键），保持账本可续。

只动三处"空格"，其余原样：
  A. <proj>/source/<书> .md                → <书>.md
  B. <proj>/source/images/<书> /           → <proj>/source/images/<书>/
  C. <images_dir>/<书> /                   → <images_dir>/<书>/   （旧名留一个软链兜底）

随之同步：
  · md 里 `images/<书> /…` 的引用
  · source/progress_items/ 的目录名、89 个 JSON 里的 key/md/img/equivalent_to/merged_refs
  · source/dedupe/{groups.json,scan.json} 里的 key/ref/image（报告文本顺带改）
  · work/style/pilot/spec.json 里选材用的 md 文件名
  · doc_index/doc_index.json 删掉（下次运行重建，无模型成本）

用法：python3 rename_spaces.py --proj <项目目录> --images-dir <全局图根> [--apply]
不带 --apply 只做只读检查与统计。
"""
import argparse
import json
import os
import sys

OLD_MD = "27闪过-长难句 .md"
NEW_MD = "27闪过-长难句.md"
OLD_DIR = "27闪过-长难句 "          # 末尾带空格
NEW_DIR = "27闪过-长难句"
OLD_REF = "images/" + OLD_DIR + "/"
NEW_REF = "images/" + NEW_DIR + "/"


def go_sanitize(s: str) -> str:
    """逐字节复刻 Go 的 sanitizeName（含 80 字节截断）。"""
    b = bytearray()
    for ch in s:
        o = ord(ch)
        ok = (
            ch in "._-"
            or (ch.isascii() and ch.isalpha())
            or ch.isdigit()
            or 0x4E00 <= o <= 0x9FFF
        )
        b += ch.encode("utf-8") if ok else b"_"
    out = bytes(b).strip(b"._")
    if not out:
        out = b"img"
    return out[:80].decode("utf-8", "surrogateescape")


def fix_str(v: str) -> str:
    return v.replace(OLD_MD, NEW_MD).replace(OLD_REF, NEW_REF)


def fix_obj(obj):
    if isinstance(obj, str):
        return fix_str(obj)
    if isinstance(obj, list):
        return [fix_obj(x) for x in obj]
    if isinstance(obj, dict):
        return {k: fix_obj(v) for k, v in obj.items()}
    return obj


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--proj", required=True)
    ap.add_argument("--images-dir", required=True)
    ap.add_argument("--apply", action="store_true")
    a = ap.parse_args()
    P = os.path.abspath(a.proj)
    IMG = os.path.abspath(a.images_dir)
    apply = a.apply

    src = os.path.join(P, "source")
    old_md = os.path.join(src, OLD_MD)
    new_md = os.path.join(src, NEW_MD)
    old_view = os.path.join(src, "images", OLD_DIR)
    new_view = os.path.join(src, "images", NEW_DIR)
    old_img = os.path.join(IMG, OLD_DIR)
    new_img = os.path.join(IMG, NEW_DIR)
    prog = os.path.join(src, "progress_items")

    for p in (old_md, old_view, old_img):
        if not os.path.exists(p):
            print("缺失，先停下来:", p, file=sys.stderr)
            return 1

    # 1) md 内容：改写引用
    with open(old_md, encoding="utf-8") as f:
        body = f.read()
    n_ref = body.count(OLD_REF)
    new_body = body.replace(OLD_REF, NEW_REF)
    print(f"[md] 引用改写 {n_ref} 处；剩余带空格引用 {new_body.count(OLD_REF)} 处")
    print(f"[md] 新文件里 `](images/{NEW_DIR}/` 共 {new_body.count('](images/' + NEW_DIR + '/')} 处")

    # 2) 账本：逐条改写内容，并按新名字重排文件
    prog_files = sorted(
        os.path.join(dp, f)
        for dp, _, fs in os.walk(prog)
        for f in fs
        if f.endswith(".json")
    )
    plan = []
    bad = 0
    for path in prog_files:
        with open(path, encoding="utf-8") as f:
            d = json.load(f)
        nd = fix_obj(d)
        expect_key = nd.get("md", "") + "::" + nd.get("img", "")
        if nd.get("key") != expect_key:
            bad += 1
        want = os.path.join(
            prog, go_sanitize(nd.get("md", "")), go_sanitize(nd.get("img", "")) + ".json"
        )
        plan.append((path, nd, want))
    print(f"[账本] {len(plan)} 条；改写后 key 不自洽 {bad} 条")
    left = sum(1 for _, d, _ in plan for s in json.dumps(d, ensure_ascii=False).split('"') if OLD_MD in s or OLD_DIR in s)
    print(f"[账本] 改写后仍有旧名的字段片段 {left} 个")
    print(f"[账本] 目录 {os.path.join(prog, go_sanitize(NEW_MD))}")
    print(f"[账本] 文件名样例 {os.path.basename(plan[0][2]) if plan else '(无)'}")

    # 3) dedupe 产物
    dd = os.path.join(src, "dedupe")
    dd_plan = []
    for name in ("groups.json", "scan.json"):
        path = os.path.join(dd, name)
        if not os.path.exists(path):
            continue
        with open(path, encoding="utf-8") as f:
            obj = json.load(f)
        n0 = json.dumps(obj, ensure_ascii=False).count(OLD_DIR)
        dd_plan.append((path, fix_obj(obj), n0))
        print(f"[dedupe] {name}: 旧名出现 {n0} 处")
    text_plan = []
    for name in ("report.md", "review.html"):
        path = os.path.join(dd, name)
        if not os.path.exists(path):
            continue
        with open(path, encoding="utf-8") as f:
            t = f.read()
        text_plan.append((path, fix_str(t), t.count(OLD_DIR)))
        print(f"[dedupe] {name}: 旧名出现 {t.count(OLD_DIR)} 处（人看的报告，顺带改）")

    # 4) 试点选材里记的 md 名
    spec = os.path.join(P, "work", "style", "pilot", "spec.json")
    spec_plan = None
    if os.path.exists(spec):
        with open(spec, encoding="utf-8") as f:
            sp = json.load(f)
        before = json.dumps(sp, ensure_ascii=False).count(OLD_MD)
        spec_plan = (spec, fix_obj(sp), before)
        print(f"[spec] 选材里的旧 md 名 {before} 处")

    if not apply:
        print("\n[dry-run] 没写任何文件；确认无误后加 --apply")
        return 0

    # ---- 落盘 ----
    # C：全局图目录改干净，旧名留软链（历史软链/副本还能解析）
    if not os.path.exists(new_img):
        os.rename(old_img, new_img)
    if not os.path.lexists(old_img):
        os.symlink(new_img, old_img)
    print("[图根] 已改名为", new_img, "并留软链", old_img)

    # B：视图目录改名，里面 89 个链接重新指向新路径
    if not os.path.exists(new_view):
        os.rename(old_view, new_view)
    for name in os.listdir(new_view):
        dst = os.path.join(new_view, name)
        target = os.path.join(new_img, name)
        if os.path.islink(dst):
            os.remove(dst)
        os.symlink(target, dst)
    print("[视图] 已改名为", new_view, "并重指链接")

    # A：md 写回 + 改名
    with open(old_md, "w", encoding="utf-8") as f:
        f.write(new_body)
    if not os.path.exists(new_md):
        os.rename(old_md, new_md)
    print("[md] 已改动引用并改名为", new_md)

    # 账本
    newdir = os.path.join(prog, go_sanitize(NEW_MD))
    os.makedirs(newdir, exist_ok=True)
    for path, nd, want in plan:
        with open(want, "w", encoding="utf-8") as f:
            json.dump(nd, f, ensure_ascii=False, indent=2)
            f.write("\n")
    for path in prog_files:
        if os.path.exists(path) and os.path.dirname(path) != newdir:
            os.remove(path)
    for d in os.listdir(prog):
        sub = os.path.join(prog, d)
        if os.path.isdir(sub) and not os.listdir(sub):
            os.rmdir(sub)
    print(f"[账本] 写入 {len(plan)} 条到 {newdir}")

    for path, obj, _ in dd_plan:
        with open(path, "w", encoding="utf-8") as f:
            json.dump(obj, f, ensure_ascii=False, indent=2)
            f.write("\n")
    for path, t, _ in text_plan:
        with open(path, "w", encoding="utf-8") as f:
            f.write(t)
    print("[dedupe] groups.json / scan.json / report.md / review.html 已同步")

    if spec_plan:
        with open(spec_plan[0], "w", encoding="utf-8") as f:
            json.dump(spec_plan[1], f, ensure_ascii=False, indent=2)
            f.write("\n")
        print("[spec] 选材里的 md 名已同步")

    di = os.path.join(P, "doc_index", "doc_index.json")
    if os.path.exists(di):
        os.remove(di)
        print("[索引] 删除 doc_index.json（下次运行重建）")

    print("\n完成。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
