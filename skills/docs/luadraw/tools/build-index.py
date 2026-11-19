#!/usr/bin/env python3
# luadraw 文档索引生成器：解析 doc-src-en 的 tex 源（章节树 + luadraw 示例），
# 用 pdftotext 从 luadraw-doc-en.pdf 的目录提取"标题→印刷页"映射（点线引导行，
# 点与点之间带空格，须用 (?:\.\s*){2,} 匹配；双栏折行拼接；条目前缀 I/1) 剥离），
# 目录外条目（subsubsection/示例）在锚点窗口内搜索，剩余继承锚点页。
# 产物：luadraw-index.json / luadraw-index.md。依赖 poppler 的 pdftotext/pdfinfo。
import json, os, re, subprocess, sys, unicodedata

def norm(s):
    s = unicodedata.normalize("NFKD", s)
    return re.sub(r"[^a-z0-9]+", "", s.lower())

def tex_to_text(s):
    # \cmd{arg}→arg；裸命令 \cmd→cmd 名（保住 luadrawShadedForms 这类标识，匹配目录靠它）
    s = re.sub(r"\\[a-zA-Z]+\*?(?:\[[^\]]*\])?\{([^{}]*)\}", r"\1", s)
    s = re.sub(r"\\([a-zA-Z]+)\*?", r"\1", s)
    return s

def main(src_dir, pdf, out_prefix):
    n_pages = int(subprocess.run(["pdfinfo", pdf], capture_output=True, text=True).stdout
                  .split("Pages:")[1].split()[0])
    pages = []
    for p in range(1, n_pages + 1):
        t = subprocess.run(["pdftotext", "-f", str(p), "-l", str(p), pdf, "-"],
                           capture_output=True, text=True).stdout
        pages.append(norm(t))

    # 解析 tex：章节树 + 命名示例，按文档序（2d → 3d → 扩展 → 附录，文件内按行号）
    entries = []
    files = sorted(f for f in os.listdir(src_dir) if f.endswith(".tex"))
    def group_key(fn):
        if "doc2d" in fn: g = 0
        elif "doc3d" in fn: g = 1
        elif "Ext" in fn: g = 2
        elif "Appendices" in fn or "History" in fn: g = 3
        else: g = 4
        sec = re.search(r"section-(\d+)", fn)
        return (g, int(sec.group(1)) if sec else 0, fn)
    for fn in files:
        with open(os.path.join(src_dir, fn), encoding="utf-8", errors="replace") as f:
            lines = f.readlines()
        for i, l in enumerate(lines):
            m = re.search(r"\\(section|subsection|subsubsection)\*?\{(.+)\}", l)
            if m:
                entries.append({"kind": m.group(1), "title": m.group(2).strip(),
                                "tex": fn, "line": i + 1})
            ex = re.search(r"\\begin\{luadraw\}\{[^}]*name=([^,}\s]+)", l)
            if ex:
                entries.append({"kind": "example", "title": ex.group(1), "tex": fn, "line": i + 1})
    entries.sort(key=lambda e: (*group_key(e["tex"]), e["line"]))

    # 目录解析（前 20 页内扫描带点线引导+尾页码的行；折行标题暂存拼接）
    toc_map = {}
    def add_toc(title, page):
        t = tex_to_text(title).strip()
        cands = {t, re.sub(r"^[IVX]+\s+", "", t), re.sub(r"^\d+\)\s*", "", t),
                 re.sub(r"^[IVX]+\s+|\d+\)\s*", "", t)}
        for key in cands:
            k = norm(key)
            if k:
                toc_map.setdefault(k, page)
    dotted_tail = re.compile(r"(?:\.\s*){2,}\d{1,3}\s*$")
    dotted_full = re.compile(r"^\s*(.+?)\s*(?:\.\s*){2,}(\d{1,3})\s*$")
    for p in range(1, min(20, n_pages) + 1):
        t = subprocess.run(["pdftotext", "-layout", "-f", str(p), "-l", str(p), pdf, "-"],
                           capture_output=True, text=True).stdout
        buf = ""
        for ln in t.splitlines():
            if buf:
                ln = buf + " " + ln.strip()
                buf = ""
            if dotted_tail.search(ln):
                m = dotted_full.match(ln)
                if m:
                    add_toc(m.group(1), int(m.group(2)))
            elif re.search(r"[A-Za-zÀ-ÿ]$", ln.strip()) and "Contents" not in ln:
                buf = ln.strip()

    # 页码定位：目录锚点优先（印刷页≈物理页，前两个锚点用正文搜索校准偏移）；
    # 窗口搜索；最后继承锚点页。单调性由构造保证。
    PAGE_OFFSET = [0]
    calibrated = [False]
    cursor = [0]
    miss = 0
    for e in entries:
        title_text = tex_to_text(e["title"])
        needle = norm(title_text)
        found = None
        if needle in toc_map:
            printed = toc_map[needle]
            if not calibrated[0]:
                for p in range(6, n_pages):
                    if needle in pages[p]:
                        PAGE_OFFSET[0] = (p + 1) - printed
                        calibrated[0] = True
                        break
            pg = printed + PAGE_OFFSET[0]
            if pg >= cursor[0]:
                found = pg
        if found is None and needle:
            for p in range(cursor[0], min(cursor[0] + 3, n_pages)):
                if needle in pages[p]:
                    found = p + 1
                    break
        if found is None:
            miss += 1
            e["pdf_page"] = cursor[0] if cursor[0] else None
        else:
            e["pdf_page"] = found
            cursor[0] = found
        e["title"] = title_text

    data = {"pdf": os.path.basename(pdf), "pages": n_pages,
            "generated": "tools/build-index.py", "entries": entries, "missing": miss}
    with open(out_prefix + ".json", "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=1)
    with open(out_prefix + ".md", "w", encoding="utf-8") as f:
        f.write(f"# luadraw 文档索引（PDF 共 {n_pages} 页；未单独定位 {miss} 条，已继承锚点页）\n\n")
        f.write("检索用 luadraw-index.json；本表供人浏览。PDF 页码=物理页（pdftotext -f/-l 直接可用）。\n\n")
        f.write("| 层级 | 标题 | 源码 | PDF页 |\n|---|---|---|---|\n")
        for e in entries:
            src = f"{e['tex']}:{e['line']}"
            pg = f"p.{e['pdf_page']}" if e["pdf_page"] else "—"
            f.write(f"| {e['kind']} | {e['title']} | {src} | {pg} |\n")
    print(f"entries={len(entries)} missing={miss} pages={n_pages}")

if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2], sys.argv[3])
