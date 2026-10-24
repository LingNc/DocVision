#!/usr/bin/env python3
"""Experimental read-only evidence API built on compare.py's existing metrics."""
import argparse
import hashlib
import json
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw

from compare import content_ink, overlap_scores, progressive_overlap, raw_overlay, to_grid, translated, worst_region
from robust_metrics import balanced_score, ink_field
from judge_contract import combine_decision, validate_judgment


def _pair(original_path, candidate_path, grid="0.95S", shift=0.10):
    original = Image.open(original_path).convert("RGB")
    candidate = Image.open(candidate_path).convert("RGB")
    ow, oh = original.size
    if candidate.width != ow:
        candidate_h = max(1, round(candidate.height * ow / candidate.width))
        candidate = candidate.resize((ow, candidate_h), Image.Resampling.LANCZOS)
    height = max(oh, candidate.height)
    src = Image.new("RGB", (ow, height), "white")
    dst = Image.new("RGB", (ow, height), "white")
    src.paste(original, (0, (height - oh) // 2))
    dst.paste(candidate, (0, (height - candidate.height) // 2))
    result = progressive_overlap(content_ink(np.asarray(src)), content_ink(np.asarray(dst)), grid, shift)
    aligned = translated(np.asarray(dst), result["shift_px"]["x"], result["shift_px"]["y"])
    return src, Image.fromarray(aligned), result, (ow, oh), candidate.size


def _file_sha256(path):
    digest = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _pair_cached(original_path, candidate_path, cache_dir, grid="0.95S", shift=0.10):
    """Cache normalized/aligned pair once per input bytes and alignment settings."""
    key_data = {"original": _file_sha256(original_path), "candidate": _file_sha256(candidate_path),
                "grid": str(grid), "shift": float(shift)}
    key = hashlib.sha256(json.dumps(key_data, sort_keys=True).encode()).hexdigest()
    cache_dir = Path(cache_dir) / key
    src_path, dst_path, meta_path = cache_dir / "source.png", cache_dir / "aligned.png", cache_dir / "pair.json"
    if src_path.is_file() and dst_path.is_file() and meta_path.is_file():
        meta = json.loads(meta_path.read_text(encoding="utf-8"))
        src, dst = Image.open(src_path).convert("RGB"), Image.open(dst_path).convert("RGB")
        return src, dst, meta["result"], tuple(meta["source_size"]), tuple(meta["candidate_size"]), True, key
    src, dst, result, source_size, candidate_size = _pair(original_path, candidate_path, grid, shift)
    cache_dir.mkdir(parents=True, exist_ok=True)
    src.save(src_path)
    dst.save(dst_path)
    meta_path.write_text(json.dumps({**key_data, "result": result, "source_size": source_size,
                                     "candidate_size": candidate_size}, ensure_ascii=False, indent=2), encoding="utf-8")
    return src, dst, result, source_size, candidate_size, False, key


def pixel_heatmap(src, dst, out_path):
    """Render deterministic absolute grayscale difference, not a semantic heatmap."""
    a = ink_field(src)
    b = ink_field(dst)
    diff = np.clip(np.abs(a - b) * 255 * 2.0, 0, 255).astype(np.uint8)
    # Blue-to-red palette makes larger ink differences more visible.
    rgb = np.zeros((*diff.shape, 3), dtype=np.uint8)
    rgb[..., 0] = diff
    rgb[..., 1] = np.maximum(0, 180 - diff // 2)
    rgb[..., 2] = 255 - diff
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    Image.fromarray(rgb).save(out_path)
    return {"image": str(out_path), "kind": "pixel_absolute_difference", "semantic": False}


def _gray_score(src, dst, result):
    def gray_ink(image):
        gray = np.asarray(image.convert("L"))
        return content_ink(np.repeat(gray[:, :, None], 3, axis=2))

    canvas, grid = result["canvas"], result["grid"]
    a, b = to_grid(gray_ink(src), canvas, grid), to_grid(gray_ink(dst), canvas, grid)
    return overlap_scores(a, b, result["shift_cells"]["y"], result["shift_cells"]["x"])[0]


def evaluate(original_path, candidate_path, out_dir, grid="0.95S", shift=0.10, judge=None, cache_dir=None):
    out_dir = Path(out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    cache_dir = Path(cache_dir) if cache_dir is not None else out_dir / ".pair-cache"
    src, dst, result, source_size, candidate_size, cache_hit, cache_key = _pair_cached(
        original_path, candidate_path, cache_dir, grid, shift)
    channels = {}
    for channel in ("gray", "r", "g", "b"):
        overlay_path = out_dir / f"overlay-{channel}.png"
        raw_overlay(np.asarray(src), np.asarray(dst), channel).save(overlay_path)
        channels[channel] = str(overlay_path)
    channel_scores = {**result["channel_scores"], "gray": _gray_score(src, dst, result)}
    robust = balanced_score(ink_field(src), ink_field(dst), radius=2, coarse_side=64, tolerant_weight=0.65)
    decision = combine_decision(robust["balanced_visual_score"], judge)
    heatmap_path = out_dir / "pixel-heatmap.png"
    pixel_heatmap(src, dst, heatmap_path)
    judge_request_path = out_dir / "judge-request.json"
    judge_request_path.write_text(json.dumps({
        "schema": "figure-judge-request/v1",
        "instruction": "Compare semantic content and relations. Treat numeric metrics as fallible evidence, not ground truth. Inspect the original and redraw before verdict.",
        "images": {"original": str(Path(original_path).resolve()), "redraw": str(Path(candidate_path).resolve()),
                   "pixel_heatmap": str(heatmap_path.resolve())},
        "quantitative_evidence": {"soft_ink_jaccard": result["score"],
                                   "robust_visual": robust,
                                   "pixel_worst_region_percent": worst_region(np.asarray(src), np.asarray(dst))},
        "signals_unavailable": ["dinov2_dense_features", "siglip_cosine", "ocr_diff"],
        "required_output_schema": {"element_check": [{"element": "string", "status": "match|shifted|distorted|wrong|missing", "note": "string"}],
                                   "missing_in_redraw": ["string"], "semantic_score": "integer 1-10",
                                   "verdict": "pass|revise|reject|uncertain", "confidence": "number 0-1",
                                   "rationale": "string", "evidence_boxes": ["percentage bbox objects"]},
    }, ensure_ascii=False, indent=2) + "\\n", encoding="utf-8")
    metrics = {
        "schema": "figure-evaluation/v1",
        "method": "soft-ink Jaccard, RGB mean; grayscale reported separately",
        "soft_ink_jaccard": result["score"],
        "soft_ink_jaccard_percent": round(result["score"] * 100, 2),
        "channel_scores": channel_scores,
        "robust_visual": robust,
        "judge": validate_judgment(judge) if judge is not None else {"status": "not_configured"},
        "decision": decision,
        "translation_px": result["shift_px"],
        "worst_missing_ink_region_percent": worst_region(np.asarray(src), np.asarray(dst)),
        "source_size": list(source_size),
        "candidate_size_after_width_normalization": list(candidate_size),
        "cache": {"hit": cache_hit, "key": cache_key},
        "alignment": {k: result[k] for k in ("canvas", "grid", "levels", "tested_offsets", "shift_cells")},
        "signals": {
            "dinov2": {"status": "not_configured"},
            "siglip": {"status": "not_configured"},
            "ocr": {"status": "not_configured"},
        },
        "evidence": {
            "overlays": channels,
            "pixel_heatmap": {"image": str(heatmap_path), "kind": "pixel_absolute_difference", "semantic": False},
            "judge_request": str(judge_request_path),
            "pair_cache_key": cache_key,
        },
    }
    (out_dir / "metrics.json").write_text(json.dumps(metrics, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return metrics


def view_region(original_path, candidate_path, box, out_path, grid="0.95S", shift=0.10, zoom=3, cache_dir=None):
    left, top, right, bottom = map(float, box)
    if not (0 <= left < right <= 100 and 0 <= top < bottom <= 100):
        raise ValueError("box must satisfy 0 <= left < right <= 100 and 0 <= top < bottom <= 100")
    if zoom < 1:
        raise ValueError("zoom must be >= 1")
    cache_dir = Path(cache_dir) if cache_dir is not None else Path(out_path).parent / ".pair-cache"
    src, dst, _, _, _, cache_hit, cache_key = _pair_cached(
        original_path, candidate_path, cache_dir, grid, shift)
    w, h = src.size
    bounds = (round(left * w / 100), round(top * h / 100), round(right * w / 100), round(bottom * h / 100))
    a, b = src.crop(bounds), dst.crop(bounds)
    source_crop_size = list(a.size)
    candidate_crop_size = list(b.size)
    if zoom != 1:
        a = a.resize((a.width * zoom, a.height * zoom), Image.Resampling.NEAREST)
        b = b.resize((b.width * zoom, b.height * zoom), Image.Resampling.NEAREST)
    gap = 12
    board = Image.new("RGB", (a.width + b.width + gap, max(a.height, b.height) + 28), "white")
    draw = ImageDraw.Draw(board)
    draw.text((0, 4), "Original", fill="black")
    draw.text((a.width + gap, 4), "Aligned redraw", fill="black")
    board.paste(a, (0, 28))
    board.paste(b, (a.width + gap, 28))
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    board.save(out_path)
    return {
        "box_percent": [left, top, right, bottom],
        "source_crop_size": source_crop_size,
        "candidate_crop_size": candidate_crop_size,
        "zoom": zoom,
        "cache_hit": cache_hit,
        "cache_key": cache_key,
        "image": str(out_path),
    }


def main():
    ap = argparse.ArgumentParser(description="Experimental local figure evaluation evidence API")
    sub = ap.add_subparsers(dest="command", required=True)
    for name in ("evaluate", "view-region", "overlay", "heatmap"):
        parser = sub.add_parser(name)
        parser.add_argument("--original", required=True, type=Path)
        parser.add_argument("--candidate", required=True, type=Path)
        parser.add_argument("--out", required=True, type=Path)
        parser.add_argument("--grid", default="0.95S")
        parser.add_argument("--shift", type=float, default=0.10)
        parser.add_argument("--cache-dir", type=Path, help="shared cache directory; defaults to <out parent>/.pair-cache")
    sub.choices["view-region"].add_argument("--box", required=True, nargs=4, type=float, metavar=("LEFT", "TOP", "RIGHT", "BOTTOM"))
    sub.choices["view-region"].add_argument("--zoom", type=int, default=3)
    sub.choices["evaluate"].add_argument("--judge-json", type=Path, help="validated structured judgment JSON; no VLM API is called")
    sub.choices["overlay"].add_argument("--channel", choices=("gray", "r", "g", "b"), default="gray")
    args = ap.parse_args()
    if args.command == "evaluate":
        judge = json.loads(args.judge_json.read_text(encoding="utf-8")) if args.judge_json else None
        cache_dir = args.cache_dir or args.out / ".pair-cache"
        result = evaluate(args.original, args.candidate, args.out, args.grid, args.shift, judge, cache_dir)
    elif args.command == "view-region":
        cache_dir = args.cache_dir or args.out.parent / ".pair-cache"
        result = view_region(args.original, args.candidate, args.box, args.out, args.grid, args.shift, args.zoom, cache_dir)
    else:
        cache_dir = args.cache_dir or args.out.parent / ".pair-cache"
        src, dst, _, _, _, cache_hit, cache_key = _pair_cached(
            args.original, args.candidate, cache_dir, args.grid, args.shift)
        if args.command == "overlay":
            args.out.parent.mkdir(parents=True, exist_ok=True)
            raw_overlay(np.asarray(src), np.asarray(dst), args.channel).save(args.out)
            result = {"channel": args.channel, "image": str(args.out), "cache_hit": cache_hit, "cache_key": cache_key}
        else:
            result = pixel_heatmap(src, dst, args.out)
            result.update({"cache_hit": cache_hit, "cache_key": cache_key})
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
