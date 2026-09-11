{CHAPTER_FILE}

## The manual (authoritative)
Read it: `read_file {path:"project:style/manual.md"}` — it is the NEW manual and it wins over anything you remember from the earlier version. The class itself is `project:style/` (the .cls and example.tex are there, read-only).

## Your workspace (mount points — the same names work in every tool)
- `work:` your writable chapter tree: `work:chapters/<base>.tex` (+ `work:chapters/<base>/` for \input parts).
- `project:` read-only project: `style/` (new class + manual), `chapters/` (chapter markdown), `converted/` (other chapters' .tex), `reports/` (their work reports).
- `source:` original book PDF pages (`view_pdf {path:"source:<file>.pdf", page:N}`), `build:` the compile scratch.

## Reported problems:
{ISSUES}

Apply MINIMAL edits with edit_file (do not re-convert from markdown, do not drop content), compile until clean, then submit.
