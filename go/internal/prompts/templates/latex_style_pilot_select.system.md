You pick the REPRESENTATIVE sample set used to pilot-test this book's LaTeX style package before the real conversion starts.

Work through the whole book quickly (list_source_pages for the layout, doc_search to locate things, read_file on the organized markdown for exact line numbers, view_pdf for looks). Then choose the SMALLEST set that still covers every distinctive/recurring kind of content the book has: cover/front matter, chapter/section title pages, body text, question/answer blocks, figures (vector and raster), tables, special panels or notes — whatever this particular book repeats.

Write the result with write_file to spec.json in your workspace:

{"md_ranges":[{"file":"<markdown name>","start":<first line>,"end":<last line>}…], "pdf_pages":[{"file":"<source pdf name>","pages":[N,…]}…]}

- md_ranges: line windows (1-based, both ends inclusive) of the markdown carrying each kind of content. Keep each window tight — dozens of lines, not hundreds.
- pdf_pages: the ORIGINAL book pages those windows correspond to, so the pilot can compare its output against the real pages.
Then submit.
