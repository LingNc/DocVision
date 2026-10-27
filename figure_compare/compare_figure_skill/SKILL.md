# compare_figure Skill

This directory is self-contained and can be copied or archived into another agent's Skill directory. It does not import code from the parent repository.

Use this Skill to compare a rendered candidate PDF against a reference image.

## Requirements

- Python 3.10 or newer with NumPy and Pillow.
- Poppler command-line tools `pdfinfo` and `pdftocairo` available on PATH.

## Inputs

- `original`: reference PNG/JPEG path.
- `pdf`: candidate PDF; page 1 is rasterized to the reference width and cached by PDF SHA-256.
- `out`: writable result directory.
- Optional `detail`, percentage crop (`left`, `top`, `right`, `bottom`), `width`, `grid`, and `shift`.

## Run

From this directory:

```bash
python3 compare.py --original <reference.png> --pdf <candidate.pdf> --out <result-dir>
```

The default gate remains the legacy fixed 50%. To explicitly use the candidate adaptive policy:

```bash
python3 compare.py --original <reference.png> --pdf <candidate.pdf> --out <result-dir> \\
  --threshold-policy adaptive-v1 --difficulty 0.65 --confidence 0.8
```

Before using the adaptive policy, the model should estimate difficulty and confidence from the comparison context. Values are normalized to `[0, 1]`; low confidence raises the gate conservatively, while high intrinsic difficulty may lower it within the configured bounds. The formula is `clamp(base + 10*(0.5-confidence) - 12*(difficulty-0.5), min, max)`. This is a candidate heuristic, not a calibrated acceptance standard.

The threshold changes only the `pass` decision. It does not change the similarity score or alignment algorithm. Treat `pass` as a review hint, not semantic proof; inspect layout, labels, geometry, colors, selected overlay, and `worst_region` before submitting.

## Outputs

- `metrics.json`: similarity, channel scores, translation, crop, output paths, worst region, threshold and policy.
- `candidate.png`: page-1 raster.
- `gray.png`, `r.png`, `g.png`, `b.png`, `montage.png`, `selected.png`: review images.
- stdout: JSON with metrics and selected image path.

Open `viewer.html` and select `metrics.json` plus the output PNGs to inspect results locally. No server or upload is used.
