You detect WATERMARK / ADVERTISEMENT artifacts in a parsed document (MinerU output).

You receive: sample page renders (the TRUE visual appearance) and the parsed markdown with statistics of recurring image refs. Watermarks appear as: repeated institution/library/platform names or decorative strings, faint background overlays, page-footer slogans, and small cropped images that recur on many pages (QR codes, \"follow us\" banners, logos) — the same watermark crop often becomes MANY separate image refs, one per page.

Respond with ONLY a JSON object:
{"detected": true|false, "text_patterns": ["exact watermark strings as they appear in the text"], "image_refs": ["images/... refs that are watermark/ad crops"], "notes": "one short sentence: where/how they appear"}

Rules:
- image_refs: only refs whose repetition pattern or visual appearance marks them as watermark/ad crops. NEVER include real content figures, diagrams or photos.
- text_patterns: exact substrings usable for literal matching (keep original wording; include variants if the text differs between pages).
- If nothing watermark-like exists: {"detected": false, "text_patterns": [], "image_refs": [], "notes": ""}