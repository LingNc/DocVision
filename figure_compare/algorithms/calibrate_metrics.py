#!/usr/bin/env python3
"""Synthetic calibration sweep; outputs are diagnostic, not semantic ground truth."""
import argparse
import csv
import json
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw, ImageFilter

from compare import content_ink, progressive_overlap
from robust_metrics import balanced_score, ink_field


def base_diagram(size=256):
    image = Image.new("RGB", (size, size), "white")
    d = ImageDraw.Draw(image)
    d.line((32, 218, 224, 218), fill="black", width=4)
    d.line((46, 228, 46, 28), fill="black", width=4)
    d.line((60, 190, 96, 144, 135, 158, 190, 48), fill="black", width=4)
    d.ellipse((182, 38, 198, 54), fill="black")
    d.line((118, 158, 118, 90), fill="black", width=3)
    return image


def cases():
    src = base_diagram()
    yield "exact", "nuisance", src.copy()

    # Rasterization/antialias changes from downsample then upsample.
    small = src.resize((128, 128), Image.Resampling.LANCZOS)
    yield "resample_2x", "nuisance", small.resize(src.size, Image.Resampling.BILINEAR)

    shifted = Image.new("RGB", src.size, "white")
    shifted.paste(src, (1, 1))
    yield "translation_1px", "nuisance", shifted

    for width in (3, 5):
        image = Image.new("RGB", src.size, "white")
        d = ImageDraw.Draw(image)
        d.line((32, 218, 224, 218), fill="black", width=width)
        d.line((46, 228, 46, 28), fill="black", width=width)
        d.line((60, 190, 96, 144, 135, 158, 190, 48), fill="black", width=width)
        d.ellipse((182, 38, 198, 54), fill="black")
        d.line((118, 158, 118, 90), fill="black", width=max(1, width - 1))
        yield f"line_width_{width}", "nuisance", image

    rng = np.random.default_rng(71023)
    array = np.asarray(src).copy()
    ys = rng.integers(0, src.height, size=220)
    xs = rng.integers(0, src.width, size=220)
    array[ys, xs] = rng.choice([0, 128, 220], size=(220, 3))
    yield "sparse_speckles", "nuisance", Image.fromarray(array)

    for label, box in (
        ("remove_vertical_axis", (38, 32, 54, 226)),
        ("remove_diagonal_segment", (118, 130, 142, 168)),
        ("remove_node", (179, 35, 202, 58)),
    ):
        image = src.copy()
        ImageDraw.Draw(image).rectangle(box, fill="white")
        yield label, "structural_change", image

    reversed_edge = Image.new("RGB", src.size, "white")
    d = ImageDraw.Draw(reversed_edge)
    d.line((32, 218, 224, 218), fill="black", width=4)
    d.line((46, 228, 46, 28), fill="black", width=4)
    d.line((60, 190, 96, 144, 135, 158, 190, 48), fill="black", width=4)
    d.ellipse((182, 38, 198, 54), fill="black")
    d.line((118, 90, 118, 158), fill="black", width=3)
    yield "reverse_edge_direction", "structural_change", reversed_edge


def score_pair(reference, candidate):
    src = np.asarray(reference)
    dst = np.asarray(candidate)
    strict_result = progressive_overlap(content_ink(src), content_ink(dst), "0.95S", 0.10)
    robust = balanced_score(ink_field(reference), ink_field(candidate))
    return {
        "legacy_soft_ink_jaccard": strict_result["score"],
        "tolerant_ink_coverage": robust["tolerant_ink_coverage"],
        "coarse_structure_overlap": robust["coarse_structure_overlap"],
        "balanced_visual_score": robust["balanced_visual_score"],
    }


def threshold_sweep(rows, score_key):
    positives = [r for r in rows if r["expected"] == "nuisance"]
    negatives = [r for r in rows if r["expected"] == "structural_change"]
    results = []
    for threshold in np.linspace(0, 1, 101):
        tpr = sum(r[score_key] >= threshold for r in positives) / max(1, len(positives))
        tnr = sum(r[score_key] < threshold for r in negatives) / max(1, len(negatives))
        results.append({"threshold": round(float(threshold), 2), "balanced_accuracy": (tpr + tnr) / 2,
                        "nuisance_accept_rate": tpr, "structure_reject_rate": tnr})
    return max(results, key=lambda r: (r["balanced_accuracy"], r["structure_reject_rate"], r["threshold"]))


def run(out_dir):
    out_dir = Path(out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    reference = base_diagram()
    rows = []
    for name, expected, candidate in cases():
        rows.append({"case": name, "expected": expected, **score_pair(reference, candidate)})
    for row in rows:
        row["expected_pass"] = row["expected"] == "nuisance"
    calibration = {
        "schema": "synthetic-calibration/v1",
        "warning": "Synthetic perturbation labels do not establish semantic correctness or production thresholds.",
        "cases": rows,
        "thresholds": {
            key: threshold_sweep(rows, key)
            for key in ("legacy_soft_ink_jaccard", "balanced_visual_score")
        },
    }
    (out_dir / "calibration.json").write_text(json.dumps(calibration, indent=2) + "\n", encoding="utf-8")
    with (out_dir / "calibration.csv").open("w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=list(rows[0]))
        writer.writeheader()
        writer.writerows(rows)
    return calibration


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, default=Path("work/vnext-eval/calibration"))
    args = parser.parse_args()
    result = run(args.out)
    print(json.dumps({"thresholds": result["thresholds"], "cases": len(result["cases"]),
                      "output": str(args.out)}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
