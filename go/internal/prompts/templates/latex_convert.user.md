Convert this chapter to LaTeX:

{CHAPTER_FILE}

## Your workspace (mount points — the same names work in read_file/grep/write_file/edit_file/bash)
- `work:` YOUR writable chapter tree: `work:chapters/<base>.tex` (your main file) and `work:chapters/<base>/` for extra `\input` parts. Nothing else is writable.
- `project:` the project, read-only: `project:style/` (class .cls + manual.md + example.tex), `project:chapters/` (your chapter markdown + sibling chapters), `project:source/` (the whole processed book markdown + images/ + figures/), `project:converted/` (other chapters' submitted .tex), `project:reports/` (their work reports).
- `source:` the ORIGINAL book PDF pages, read with `view_pdf {path:"source:<file>.pdf", page:N}`; `list_source_pages`/`doc_search` tell you which page holds what.
- `build:` the compile scratch (where `compile` runs). `/tmp` in bash is scratch for this session and survives between calls.

## Read these first (they are the authoritative inputs)
- `read_file {path:"project:style/manual.md"}` — the class usage manual. Follow it exactly.
- `read_file {path:"project:chapters/<base>.md"}` — the chapter TEXT you must convert. It is already high-quality markdown with the DOCVISION machine comments; convert from it. Never re-read text off page images: use the original PDF only to check LAYOUT (side-by-side figures, tables, captions) that markdown cannot express.

{TEX_PATH}

Chapter markdown preview (first 2000 characters — read the file itself for the rest):
{CHAPTER_PREVIEW}
