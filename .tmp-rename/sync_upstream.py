#!/usr/bin/env python3
"""源 PDF 改名后，把上游派生出来的名字一起对齐，保住"不再重切、不再重解析"。

链路上带这本书名字的有五处（都以源文件名的主干为前缀）：

  files/<书>.pdf                          ← 用户已改名（本次的起因）
  files/done/<书> .pdf                     ← 切分成功后归档的原件（归档，不动）
  split_files/<书> _partN.pdf + _split_manifest.json
  mineru_output/<书> _partN/               ← MinerU 解析产物（重解析要钱）
  output/<书> .md + .md.fp                 ← 合并出来的 md 与指纹
  finally/<书> .md                         ← 档位2 终稿

判据（internal/split/manifest.go 的 Matches）：source_path / source_size /
source_mtime_ns / max_pages / max_size_mb 全对上才会跳过切分。所以清单里
三样都要跟着新文件名走，否则下次会重切 → part 名一变 → MinerU 目录对不上
→ 整本重解析。

用法：python3 sync_upstream.py --run-dir <运行目录> [--apply]
"""
import argparse
import json
import os
import sys

OLD = "27闪过-长难句 "      # 末尾带空格（旧主干）
NEW = "27闪过-长难句"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--run-dir", required=True)
    ap.add_argument("--apply", action="store_true")
    a = ap.parse_args()
    R = os.path.abspath(a.run_dir)
    apply = a.apply

    src_new = os.path.join(R, "files", NEW + ".pdf")
    if not os.path.exists(src_new):
        print("源文件不在:", src_new, file=sys.stderr)
        return 1
    st = os.stat(src_new)

    split_dir = os.path.join(R, "split_files")
    parts_old = [os.path.join(split_dir, f"{OLD}_part{i}.pdf") for i in (1, 2)]
    manif_old = os.path.join(split_dir, f"{OLD}_split_manifest.json")
    for p in parts_old + [manif_old]:
        if not os.path.exists(p):
            print("缺:", p, file=sys.stderr)
            return 1

    manif = json.load(open(manif_old, encoding="utf-8"))
    print(f"[清单] 旧 source_path={manif['source_path']!r} size={manif['source_size']}")
    print(f"[清单] 新 source_path=files/{NEW}.pdf  size={st.st_size}  mtime_ns={st.st_mtime_ns}")
    if manif["source_size"] != st.st_size:
        print("!! 大小不一致，先停下：新源文件与切分时的不是同一份", file=sys.stderr)
        return 1
    print("[清单] 大小一致；mtime 与记录值差异 = 改名/拷贝带来的，本次按新文件刷新")

    # 新清单
    manif["source_path"] = f"files/{NEW}.pdf"
    manif["source_mtime_ns"] = st.st_mtime_ns
    for i, part in enumerate(manif["parts"]):
        print(f"[清单] part{i+1}: {part['filename']!r} -> {NEW}_part{i+1}.pdf")
        part["filename"] = f"{NEW}_part{i+1}.pdf"

    mineru_old = [os.path.join(R, "mineru_output", f"{OLD}_part{i}") for i in (1, 2)]
    mineru_new = [os.path.join(R, "mineru_output", f"{NEW}_part{i}") for i in (1, 2)]
    for o, n in zip(mineru_old, mineru_new):
        print(f"[MinerU] {os.path.basename(o)!r} -> {os.path.basename(n)!r}  存在={os.path.isdir(o)}")

    out_old = [os.path.join(R, "output", OLD + ".md"), os.path.join(R, "output", OLD + ".md.fp")]
    out_new = [os.path.join(R, "output", NEW + ".md"), os.path.join(R, "output", NEW + ".md.fp")]
    fin_old = os.path.join(R, "finally", OLD + ".md")
    fin_new = os.path.join(R, "finally", NEW + ".md")
    for o, n in list(zip(out_old, out_new)) + [(fin_old, fin_new)]:
        print(f"[产物] {os.path.basename(o)!r} -> {os.path.basename(n)!r}  存在={os.path.exists(o)}")

    if not apply:
        print("\n[dry-run] 没写任何文件；确认无误后加 --apply")
        return 0

    # 1) 切分产物
    for i, p in enumerate(parts_old, 1):
        os.rename(p, os.path.join(split_dir, f"{NEW}_part{i}.pdf"))
    with open(manif_old, "w", encoding="utf-8") as f:
        json.dump(manif, f, ensure_ascii=False, indent=2)
        f.write("\n")
    os.rename(manif_old, os.path.join(split_dir, f"{NEW}_split_manifest.json"))
    print("[切分] part1/part2 + 清单 已改名，清单来源已指向新文件名")

    # 2) MinerU 产物
    for o, n in zip(mineru_old, mineru_new):
        if os.path.isdir(o) and not os.path.exists(n):
            os.rename(o, n)
    print("[MinerU] 两个 part 目录已改名")

    # 3) 合并 md / 指纹 / 终稿
    for o, n in list(zip(out_old, out_new)) + [(fin_old, fin_new)]:
        if os.path.exists(o) and not os.path.exists(n):
            os.rename(o, n)
    print("[产物] output md/fp 与 finally 终稿已改名")

    print("\n完成。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
