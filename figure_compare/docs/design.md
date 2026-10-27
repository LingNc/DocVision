# Design notes

## Metrics

`compare.py` preserves the legacy soft-ink overlap workflow and its PDF rasterization/evidence outputs. `compare_vnext.py` adds cached pairing, progressive alignment, channel evidence, heatmap/region views, and optional judge input. `robust_metrics.py` supplies tolerant and coarse structural proxies. These scores are visual evidence, not semantic truth.

## Session review

The Go runner exposes `view_pdf` for inspecting the compiled candidate independently, followed by `compare_figure` for registered channel evidence and similarity. The comparison response includes `worst_region` as a percentage crop box; callers can pass that box with a larger `zoom` to inspect the weakest area. This restores review evidence and navigation only: the existing similarity metric and submission threshold are unchanged.

## Transcript recovery

The viewer can roll the transcript back to any model tool call. Each tool row offers a cut control: truncating keeps every record before the assistant message that issued the call (including its sibling calls) and removes that call and all later records. The kept history stays wire-valid, and replay sanitization synthesizes receipts for any dangling call, so restarting the runner with `-resume` on the same transcript continues directly without a new user turn. The viewer requires read/write permission, asks for explicit confirmation, refuses if the transcript changed since it was loaded, and creates a unique sibling backup before rewriting. Stop the runner before truncating, and recompile/recompare before submitting because runtime gates are process-local.

## Reusable comparator Skill

`compare_figure_skill/` is a self-contained, exportable copy of the current `algorithms/compare.py` comparator plus its optional `adaptive-v1` threshold policy. The Skill directory contains the full implementation, model-facing instructions, and a static local viewer; it does not import from the parent repository. The copy preserves rasterization, alignment, soft-ink score, channel overlays, and worst-region behavior, with only the adaptive threshold entry point added and two stray unreachable fragments omitted so it compiles. Keep the repository implementation and exported copy behaviorally aligned with synthetic regression tests. The threshold uses explicit normalized difficulty and confidence, clamped to caller-provided bounds; high difficulty can lower the gate within bounds, while low confidence raises it conservatively. It changes only the pass decision and is recorded in metrics. This is a candidate policy for later calibration, not a replacement for visual review. The default remains the legacy fixed 50% threshold, preserving existing score values and behavior. The standalone viewer loads metrics and user-selected PNG outputs locally without a server or upload.

The repository contains code and synthetic tests only. Real images, PDFs, JSONL transcripts, metrics, and generated overlays remain external and are ignored by this project.
