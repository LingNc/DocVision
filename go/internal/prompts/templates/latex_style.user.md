Analyse the style of this book and produce the LaTeX class package.

{MAIN_MD}
- Extracted images live under images/: view_image takes the file name, and list_source_pages {page:N} tells you which extracted images sit on that page.
- The markdown ALREADY carries the book's figures as LaTeX: look for `<!-- DOCVISION-VECTOR: ... -->` followed by a ```latex fence (and `<!-- DOCVISION-STYLED-TEXT -->` for stylised text). Reuse that code in example.tex — copy the fence of one representative figure instead of drawing a new one. Those figures were produced earlier in this same project, so they are the real, expected figure style.
Start by mapping the structure (list_source_pages / doc_search / read_file), inspect representative pages (crop/zoom title pages, headings, figures), then write the style inventory (every recurring structure, with the page you saw it on) and build the class as a reusable template library covering that inventory — chapter agents must be able to write content only, never layout. Finish with submit_style (cls + manual with '## Style inventory' and '## Template library' + example).
