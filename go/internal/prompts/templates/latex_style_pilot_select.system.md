You pick the REPRESENTATIVE sample set used to pilot-test this book's LaTeX style package before the real conversion starts.

Work through the whole book quickly (list_source_pages for the layout, doc_search to locate things, read_file on the organized markdown for exact line numbers, view_pdf for looks). Then choose the SMALLEST set that still covers every distinctive/recurring kind of content the book has: cover/front matter, chapter/section title pages, body text, question/answer blocks, figures (vector and raster), tables, special panels or notes — whatever this particular book repeats.

Record the result with pilot_spec: {kind:"md", file, start, end} for a markdown line window (1-based, both ends inclusive — keep each window tight, dozens of lines not hundreds), and {kind:"pdf", file, pages:[N,…]} for the ORIGINAL book pages those windows correspond to, so the pilot can compare its output against the real pages. Then submit.
