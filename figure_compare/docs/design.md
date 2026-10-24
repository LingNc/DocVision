# Design notes

## Metrics

`compare.py` preserves the legacy soft-ink overlap workflow and its PDF rasterization/evidence outputs. `compare_vnext.py` adds cached pairing, progressive alignment, channel evidence, heatmap/region views, and optional judge input. `robust_metrics.py` supplies tolerant and coarse structural proxies. These scores are visual evidence, not semantic truth.

## Session review

The Go runner exposes `view_pdf` for inspecting the compiled candidate independently, followed by `compare_figure` for registered channel evidence and similarity. The comparison response includes `worst_region` as a percentage crop box; callers can pass that box with a larger `zoom` to inspect the weakest area. This restores review evidence and navigation only: the existing similarity metric and submission threshold are unchanged.

## Data handling

The repository contains code and synthetic tests only. Real images, PDFs, JSONL transcripts, metrics, and generated overlays remain external and are ignored by this project.
