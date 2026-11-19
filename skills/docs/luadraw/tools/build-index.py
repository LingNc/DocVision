#!/usr/bin/env python3
# luadraw 文档索引生成器：解析 doc-src-en 的 tex 源（章节树 + luadraw 示例），
# 用 pdftotext 在 luadraw-doc-en.pdf 中逐页定位标题，产出 luadraw-index.json / .md。
# 用法：python3 build-index.py <doc-src-en目录> <pdf路径> <输出前缀>
# 依赖：poppler 的 pdftotext（PDF 按页拆文本）。
import json, os, re, subprocess, sys, unicodedata

def norm(s):
    s = unicodedata.normalize("NFKD", s)
    return re.sub(r"[^a-z0-9]+", "", s.lower())

def tex_to_text(s):
    # 去 tex 标记，近似 PDF 中的显示文本：
    # \cmd{arg}→arg；裸命令 \cmd→cmd 名（保住 luadrawShadedForms 这类标识，匹配目录靠它）
    s = re.sub(r"\\[a-zA-Z]+\*?(?:\[[^\]]*\])?\{([^{}]*)\}", r"\1", s)
    s = re.sub(r"\\([a-zA-Z]+)\*?", r"\1", s)
    return s

def main(src_dir, pdf, out_prefix):
    # 1) PDF 逐页文本
    n_pages = int(subprocess.run(["pdfinfo", pdf], capture_output=True, text=True).stdout
                  .split("Pages:")[1].split()[0])
    pages = []
    for p in range(1, n_pages + 1):
        t = subprocess.run(["pdftotext", "-f", str(p), "-l", str(p), pdf, "-"],
                           capture_output=True, text=True).stdout
        pages.append(norm(t))

    # 2) 解析 tex：章节树 + 示例（按文档顺序）
    entries = []
    files = sorted(f for f in os.listdir(src_dir) if f.endswith(".tex"))
    order = {"section": 0, "subsection": 1, "subsubsection": 2}

    def file_seq(fn):
        m = re.match(r"luadraw-doc(2d|3d|)-en(?:-section-(\d+))?|luadraw-doc-en-(Ext|Appendices|History)", fn)
        return fn

    for fn in files:
        path = os.path.join(src_dir, fn)
        with open(path, encoding="utf-8", errors="replace") as f:
            lines = f.readlines()
        # 示例名（按出现顺序，挂到最近的后面的章节）
        ex_names = [(i + 1, m.group(1)) for i, l in enumerate(lines)
                    if (m := re.search(r"\\begin\{luadraw\}\{[^}]*name=([^,}\s]+)", l))]
        for i, l in enumerate(lines):
            m = re.search(r"\\(section|subsection|subsubsection)\*?\{(.+)\}", l)
            if m:
                entries.append({"kind": m.group(1), "title": m.group(2).strip(),
                                "tex": fn, "line": i + 1})
        for ln, name in ex_names:
            entries.append({"kind": "example", "title": name, "tex": fn, "line": ln})
    # 稳定排序：按文件名文档序 + 行号（前缀分组：2d < 3d < Ext < 附录）
    def group_key(fn):
        if "doc2d" in fn: g = 0
        elif "doc3d" in fn: g = 1
        elif "Ext" in fn: g = 2
        elif "Appendices" in fn or "History" in fn: g = 3
        else: g = 4
        sec = re.search(r"section-(\d+)", fn)
        return (g, int(sec.group(1)) if sec else 0, fn)
    entries.sort(key=lambda e: (*group_key(e["tex"]), e["line"]))

    # 3) 页码定位：优先解析 PDF 目录（标题+点线+页码），印刷页码→物理页码用前两个
    #    已定位章节校准偏移；目录外的条目（subsubsection/示例）在同文件锚点的
    #    [锚点页, 锚点页+3] 窗口内搜标题，搜不到就继承锚点页。单调性由构造保证。
    toc_map = {}  # norm(title) -> printed_page

    def add_toc(title, page):
        t = tex_to_text(title).strip()
        for key in {t, re.sub(r"^[IVX]+\s+", "", t), re.sub(r"^\d+\)\s*", "", t),
                    re.sub(r"^[IVX]+\s+|\d+\)\s*", "", t)}:
            k = norm(key)
            if k:
                toc_map.setdefault(k, page)

    # TOC 解析：双栏 -layout，条目可跨行折行（无尾页码的行与下一行拼接）；带点线引导+尾页码即一条
    for p in range(1, min(20, n_pages) + 1):
        t = subprocess.run(["pdftotext", "-layout", "-f", str(p), "-l", str(p), pdf, "-"],
                           capture_output=True, text=True).stdout
        buf = ""
        for ln in t.splitlines():
            if buf:
                ln = buf + " " + ln.strip()
                buf = ""
            if re.search(r"\.{2,}\s*\d{1,3}\s*$", ln):
                m = re.match(r"^\s*(.+?)\s*\.{2,}\s*(\d{1,3})\s*$", ln)
                if m:
                    add_toc(m.group(1), int(m.group(2)))
            elif re.search(r"[\u4e00-\u9fffA-Za-zÀ-ÿ]$", ln.strip()) and not ln.strip().startswith("Contents"):
                # 可能是折行标题的头部（无点线无页码），暂存拼下一行
                buf = ln.strip()

    def physical(printed, hint_start):
        # 印刷页码→物理页码：默认偏移 0，用前两个锚点校准
        return printed + PAGE_OFFSET[0]

    PAGE_OFFSET = [0]
    calibrated = [False]
    cursor = [0]
    miss = 0
    for e in entries:
        title_text = tex_to_text(e["title"])
        needle = norm(title_text)
        found = None
        # ① 目录锚点
        if needle in toc_map:
            printed = toc_map[needle]
            if not calibrated[0]:
                # 用标题全文搜定偏移（限制正文区，避开目录页自身）
                body_start = 6
                for p in range(body_start, n_pages):
                    if needle in pages[p]:
                        PAGE_OFFSET[0] = (p + 1) - printed
                        calibrated[0] = True
                        break
            pg = printed + PAGE_OFFSET[0]
            if pg >= cursor[0]:
                found = pg
        # ② 窗口搜索（示例/subsubsection）
        if found is None and needle:
            for p in range(cursor[0], min(cursor[0] + 3, n_pages)):
                if needle in pages[p]:
                    found = p + 1
                    break
        # ③ 继承锚点页
        if found is None:
            miss += 1
            e["pdf_page"] = cursor[0] if cursor[0] else None
        else:
            e["pdf_page"] = found
            cursor[0] = found
        e["title"] = title_text

    # 4) 产物
    data = {"pdf": os.path.basename(pdf), "pages": n_pages,
            "generated": "build-index.py", "entries": entries,
            "missing": miss}
    with open(out_prefix + ".json", "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=1)
    with open(out_prefix + ".md", "w", encoding="utf-8") as f:
        f.write(f"# luadraw 文档索引（PDF 共 {n_pages} 页；未定位 {miss} 条）\n\n")
        f.write("| 层级 | 标题 | 源码 | PDF页 |\n|---|---|---|---|\n")
        for e in entries:
            src = f"{e['tex']}:{e['line']}"
            pg = f"p.{e['pdf_page']}" if e["pdf_page"] else "—"
            f.write(f"| {e['kind']} | {e['title']} | {src} | {pg} |\n")
    print(f"entries={len(entries)} missing={miss} pages={n_pages}")

if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2], sys.argv[3])
