# Design notes

## Metrics

`compare.py` preserves the legacy soft-ink overlap workflow and its PDF rasterization/evidence outputs. `compare_vnext.py` adds cached pairing, progressive alignment, channel evidence, heatmap/region views, and optional judge input. `robust_metrics.py` supplies tolerant and coarse structural proxies. These scores are visual evidence, not semantic truth.

## Decision boundary

`judge_contract.py` validates an optional structured semantic judgment. Missing or low-confidence judgment never becomes an automatic semantic pass. Calibration data is synthetic and is not a substitute for validation on real documents.

## Data handling

The repository contains code and synthetic tests only. Real images, PDFs, JSONL transcripts, metrics, and generated overlays remain external and are ignored by this project.
