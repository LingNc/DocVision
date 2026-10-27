#!/usr/bin/env python3
import argparse
import json
import hashlib
import math
import re
import subprocess
import tempfile
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw


def render_pdf(pdf: Path, out: Path, target_size):
    w, h = target_size
    info = subprocess.run(["pdfinfo", str(pdf)], check=True, text=True, capture_output=True).stdout
    page = re.search(r"^Page size:\s*([0-9.]+)\s+x\s+([0-9.]+)\s+pts", info, re.M)
    if not page:
        raise RuntimeError("pdfinfo did not report a page size")
    page_w, page_h = float(page.group(1)), float(page.group(2))
    dpi = 72.0 * w / page_w
    with tempfile.TemporaryDirectory(prefix="figure-render-") as td:
        prefix = Path(td) / "page"
        subprocess.run([
            "pdftocairo", "-f", "1", "-l", "1", "-singlefile", "-png",
            "-r", f"{dpi:.8f}", str(pdf), str(prefix),
        ], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        page_img = Image.open(str(prefix) + ".png").convert("RGB")
        rw, rh = page_img.size
        if rw != w:
            # Cairo dimensions round from points to pixels; normalize width only.
            rh = max(1, go_round(rh * w / rw))
            page_img = page_img.resize((w, rh), Image.Resampling.LANCZOS)
        canvas_h = max(h, rh)
        canvas = Image.new("RGB", (w, canvas_h), "white")
        page_y = (canvas_h - rh) // 2
        canvas.paste(page_img, (0, page_y))
        canvas.save(out)
        return {"rendered_size": [w, rh], "canvas_size": [w, canvas_h], "source_size": [w, h], "render_dpi": round(dpi, 6)}


def pdf_cache_key(pdf: Path):
    h = hashlib.sha256()
    with pdf.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def render_cached(pdf: Path, out: Path, target_size):
    key_path = out.with_suffix(".pdf.sha256")
    key = pdf_cache_key(pdf)
    if out.exists() and key_path.exists() and key_path.read_text(encoding="utf-8").strip() == key:
        return {"cache_hit": True, "pdf_sha256": key}
    info = render_pdf(pdf, out, target_size)
    key_path.write_text(key + "\n", encoding="utf-8")
    info.update({"cache_hit": False, "pdf_sha256": key})
    return info


def parse_finest(spec, canvas):
    value = str(spec).strip().lower()
    if value.endswith("s"):
        try:
            finest = math.floor(float(value[:-1]) * canvas + 0.5)
        except ValueError:
            finest = round(0.95 * canvas)
    else:
        try:
            finest = int(value)
        except ValueError:
            finest = round(0.95 * canvas)
    return max(8, min(1024, finest))


def levels_for(canvas, finest):
    final = min(finest, canvas)
    levels = []
    grid = 32
    while grid <= final:
        levels.append(grid)
        grid *= 2
    if not levels:
        return [32]
    if levels[-1] < final:
        levels.append(final)
    return levels


def go_round(value):
    return math.floor(value + 0.5) if value >= 0 else math.ceil(value - 0.5)


def to_grid(ink, canvas, grid):
    h, w, channels = ink.shape
    off_x, off_y = (canvas - w) // 2, (canvas - h) // 2
    integral = np.pad(ink.astype(np.float64), ((1, 0), (1, 0), (0, 0))).cumsum(0).cumsum(1)
    edges = np.arange(grid + 1) * canvas // grid
    x0 = np.clip(edges[:-1] - off_x, 0, w)
    x1 = np.clip(edges[1:] - off_x, 0, w)
    y0 = np.clip(edges[:-1] - off_y, 0, h)
    y1 = np.clip(edges[1:] - off_y, 0, h)
    total = (integral[y1[:, None], x1[None, :]]
             - integral[y0[:, None], x1[None, :]]
             - integral[y1[:, None], x0[None, :]]
             + integral[y0[:, None], x0[None, :]])
    area = np.diff(edges)[:, None] * np.diff(edges)[None, :]
    return (total / area[:, :, None]).astype(np.float32)


def overlap_scores(a, b, dy, dx):
    g = a.shape[0]
    # Same displacement convention as motifCanonOverlap: B is sampled at
    # (y-dy, x-dx); out-of-canvas cells contribute A to the union only.
    ay0, ay1 = max(0, dy), min(g, g + dy)
    ax0, ax1 = max(0, dx), min(g, g + dx)
    if ay1 <= ay0 or ax1 <= ax0:
        return [0.0] * a.shape[2]
    av = a[ay0:ay1, ax0:ax1]
    bv = b[ay0 - dy:ay1 - dy, ax0 - dx:ax1 - dx]
    inter = np.minimum(av, bv).sum(axis=(0, 1), dtype=np.float64)
    union = np.maximum(av, bv).sum(axis=(0, 1), dtype=np.float64)
    # Cells outside B's translated bounds still carry A's ink.
    total_a = a.sum(axis=(0, 1), dtype=np.float64)
    union += total_a - av.sum(axis=(0, 1), dtype=np.float64)
    return [float(inter[c] / union[c]) if union[c] > 1e-9 else 0.0
            for c in range(a.shape[2])]


def ink_centroid(ink):
    """Return (y, x) centroid of ink mass in a cropped frame."""
    mass = ink.max(axis=2)
    total = float(mass.sum())
    if total <= 0:
        return 0.0, 0.0
    ys, xs = np.nonzero(mass)
    return float((ys * mass[ys, xs]).sum() / total), float((xs * mass[ys, xs]).sum() / total)


def progressive_overlap(src_ink, dst_ink, grid_spec="0.95S", shift_pct=0.10, starts=None):
    """Coarse-to-fine soft-ink alignment with multiple starting anchors."""
    canvas = max(src_ink.shape[0], src_ink.shape[1], dst_ink.shape[0], dst_ink.shape[1])
    canvas = max(24, min(512, canvas))
    finest = parse_finest(grid_spec, canvas)
    levels = levels_for(canvas, finest)
    shift_cells = min(16, max(0, math.floor(shift_pct * 32 + 0.5)))
    shift_pct = shift_cells / 32.0
    coarse_window = max(1, round(shift_pct * levels[0]))
    grids = [(g, to_grid(src_ink, canvas, g), to_grid(dst_ink, canvas, g)) for g in levels]

    def run_pyramid(start_y, start_x):
        shift_y, shift_x = start_y, start_x
        best_scores = [0.0, 0.0, 0.0]
        tested = 0
        for level_index, (grid, a, b) in enumerate(grids):
            if level_index == 0:
                win = coarse_window
                cx, cy = start_x, start_y
            else:
                win = min(2, grid)
                previous = levels[level_index - 1]
                cx = go_round(shift_x * grid / previous)
                cy = go_round(shift_y * grid / previous)
            best = -1.0
            best_y, best_x = cy, cx
            for dy in range(cy - win, cy + win + 1):
                for dx in range(cx - win, cx + win + 1):
                    scores = overlap_scores(a, b, dy, dx)
                    score = sum(scores) / len(scores)
                    tested += 1
                    if score > best:
                        best, best_y, best_x, best_scores = score, dy, dx, scores
            shift_y, shift_x = best_y, best_x
        return {"score": max(0.0, best), "channel_scores": best_scores,
                "shift_y": shift_y, "shift_x": shift_x, "tested": tested}

    candidates = [(0, 0)]
    for start in starts or []:
        if len(start) == 2:
            candidates.append((int(start[0]), int(start[1])))
    seen = set()
    candidates = [c for c in candidates if not (c in seen or seen.add(c))]
    runs = [run_pyramid(y, x) for y, x in candidates]
    # Prefer the smallest displacement only for exact score ties.
    best_run = max(runs, key=lambda run: (run["score"], -(abs(run["shift_y"]) + abs(run["shift_x"]))))
    shift_y, shift_x = best_run["shift_y"], best_run["shift_x"]
    return {
        "score": best_run["score"],
        "channel_scores": dict(zip(("r", "g", "b"), best_run["channel_scores"])),
        "canvas": canvas,
        "grid": levels[-1],
        "levels": levels,
        "shift_cells": {"x": shift_x, "y": shift_y},
        "shift_px": {"x": go_round(shift_x * canvas / levels[-1]), "y": go_round(shift_y * canvas / levels[-1])},
        "tested_offsets": sum(run["tested"] for run in runs),
        "coarse_shift_cells": coarse_window,
        "shift_budget_percent": round(shift_pct * 100, 3),
        "alignment_starts": [{"y": y, "x": x} for y, x in candidates],
        "alignment_runs": [
            {"start": {"y": y, "x": x}, "score": round(run["score"], 6),
             "shift_cells": {"x": run["shift_x"], "y": run["shift_y"]}}
            for (y, x), run in zip(candidates, runs)
        ],
    }


def content_ink(rgb):
    """Return cropped soft ink and its exact origin in the full canvas."""
    ink = 1.0 - np.asarray(rgb, dtype=np.float32) / 255.0
    mask = np.max(ink, axis=2) > 0.2
    ys, xs = np.where(mask)
    if not len(xs):
        return np.zeros((1, 1, 3), dtype=np.float32), (0, 0)
    return ink[ys.min():ys.max() + 1, xs.min():xs.max() + 1], (int(xs.min()), int(ys.min()))


def select_channel(rgb, channel):
    channel = str(channel or "all").lower()
    if channel in ("all", "rgb"):
        return rgb
    if channel == "gray":
        return np.asarray(Image.fromarray(rgb).convert("L")).astype(np.uint8)
    indices = {"r": 0, "g": 1, "b": 2}
    if all(c in indices for c in channel) and 1 <= len(channel) <= 3:
        idx = [indices[c] for c in channel]
        if len(idx) == 1:
            return rgb[:, :, idx[0]]
        return rgb[:, :, idx]
    raise ValueError(f"unsupported channel selection: {channel}")


def crop_percent_image(img, left=0, top=0, right=100, bottom=100):
    w, h = img.size
    x0, y0 = round(clamp_pct(left) * w / 100), round(clamp_pct(top) * h / 100)
    x1, y1 = round(clamp_pct(right) * w / 100), round(clamp_pct(bottom) * h / 100)
    if x1 <= x0 or y1 <= y0:
        raise ValueError("crop must have positive area")
    return img.crop((x0, y0, x1, y1))


def clamp_pct(v):
    return max(0.0, min(100.0, float(v)))


def raw_overlay(src, dst, channel="all"):
    """Native-pixel additive composite: no montage resizing or resampling."""
    src_sel, dst_sel = select_channel(src, channel), select_channel(dst, channel)
    if src_sel.ndim == 2:
        src_sel = src_sel[:, :, None]
        dst_sel = dst_sel[:, :, None]
    src_ink = 255 - src_sel.astype(np.uint16)
    dst_ink = 255 - dst_sel.astype(np.uint16)
    shared = np.minimum(src_ink, dst_ink).max(axis=2) > 51
    src_only = src_ink.max(axis=2) > 51
    dst_only = dst_ink.max(axis=2) > 51
    out = np.zeros((*src.shape[:2], 3), dtype=np.uint8)
    out[shared] = 255
    out[src_only & ~dst_only, 0] = 255
    out[src_only & ~dst_only, 2] = 255
    out[dst_only & ~src_only, 1] = 255
    return Image.fromarray(out)


def translated(rgb, dx, dy):
    h, w = rgb.shape[:2]
    out = np.full_like(rgb, 255)
    sx0, sx1 = max(0, -dx), min(w, w - dx)
    sy0, sy1 = max(0, -dy), min(h, h - dy)
    if sx0 < sx1 and sy0 < sy1:
        out[sy0 + dy:sy1 + dy, sx0 + dx:sx1 + dx] = rgb[sy0:sy1, sx0:sx1]
    return out


def tint_overlay(src, dst, channel):
    return raw_overlay(src, dst, channel)



BOARD_GAP = 16
BOARD_MARGIN = 16
BOARD_TITLE_H = 26
BOARD_BG = "#e9edee"


def compose_board(panels, labels, gap=BOARD_GAP, margin=BOARD_MARGIN, title_h=BOARD_TITLE_H):
    """Lay panels out 2x2 with title strips, gutters and an outer margin.

    Panel pixels are pasted 1:1 — only the gutters/titles are presentation.
    """
    pw, ph = panels[0].size
    cols, rows = 2, 2
    board = Image.new("RGB", (margin * 2 + pw * cols + gap * (cols - 1),
                              margin * 2 + (ph + title_h) * rows + gap * (rows - 1)), BOARD_BG)
    draw = ImageDraw.Draw(board)
    for i, panel in enumerate(panels):
        x = margin + (i % cols) * (pw + gap)
        y = margin + (i // cols) * (ph + title_h + gap)
        draw.rectangle((x, y, x + pw - 1, y + title_h - 2), fill="#ffffff", outline="#c9d2d4")
        label = labels[i] if i < len(labels) else ""
        draw.text((x + 8, y + 7), label, fill="#16222a")
        board.paste(panel, (x, y + title_h))
        draw.rectangle((x, y + title_h, x + pw - 1, y + title_h + ph - 1), outline="#c9d2d4")
    return board


def worst_region(src, dst, cells=16):
    """Where does the redraw miss original ink? Returns percent box + share."""
    a = 1.0 - np.asarray(src, dtype=np.float32) / 255.0
    b = 1.0 - np.asarray(dst, dtype=np.float32) / 255.0
    missing = np.clip(a.max(axis=2) - b.max(axis=2), 0.0, None)
    total = float(missing.sum())
    if total <= 0:
        return None
    h, w = missing.shape
    cy, cx = max(1, cells), max(1, cells)
    acc = np.zeros((cy, cx), dtype=np.float64)
    for y in range(cy):
        for x in range(cx):
            acc[y, x] = missing[y * h // cy:(y + 1) * h // cy, x * w // cx:(x + 1) * w // cx].sum()
    peak_y, peak_x = np.unravel_index(int(acc.argmax()), acc.shape)
    y0, y1 = max(0, peak_y - 1), min(cy, peak_y + 2)
    x0, x1 = max(0, peak_x - 1), min(cx, peak_x + 2)
    share = float(acc[y0:y1, x0:x1].sum()) / total
    return {
        "left": round(x0 * 100 / cx, 1),
        "top": round(y0 * 100 / cy, 1),
        "right": round(x1 * 100 / cx, 1),
        "bottom": round(y1 * 100 / cy, 1),
        "share_percent": round(share * 100, 1),
    }


def threshold_for(difficulty=0.5, confidence=0.5, base=50.0, minimum=40.0, maximum=75.0):
    difficulty = min(1.0, max(0.0, float(difficulty)))
    confidence = min(1.0, max(0.0, float(confidence)))
    minimum, maximum = float(minimum), float(maximum)
    if minimum > maximum:
        raise ValueError("minimum threshold must not exceed maximum threshold")
    base = min(maximum, max(minimum, float(base)))
    value = base + 10.0 * (0.5 - confidence) - 12.0 * (difficulty - 0.5)
    return round(min(maximum, max(minimum, value)), 2)


def save_comparisons(original_path: Path, candidate_path: Path, out_dir: Path, detail="all", grid_spec="0.95S", shift_pct=0.10, crop=(0, 0, 100, 100), width=0, threshold=50.0, threshold_policy=None):
    out_dir.mkdir(parents=True, exist_ok=True)
    src_img = Image.open(original_path).convert("RGB")
    cand_img = Image.open(candidate_path).convert("RGB")
    source_w, source_h = src_img.size
    iw, ih = source_w, source_h
    candidate_w, candidate_h = cand_img.size
    if cand_img.width != iw:
        candidate_h = max(1, go_round(cand_img.height * iw / cand_img.width))
        cand_img = cand_img.resize((iw, candidate_h), Image.Resampling.LANCZOS)
        candidate_w = iw
    canvas_h = max(ih, candidate_h)
    src_canvas = Image.new("RGB", (iw, canvas_h), "white")
    cand_canvas = Image.new("RGB", (iw, canvas_h), "white")
    src_canvas.paste(src_img, (0, (canvas_h - ih) // 2))
    cand_canvas.paste(cand_img, (0, (canvas_h - candidate_h) // 2))
    src_img, cand_img = src_canvas, cand_canvas
    src, cand = np.asarray(src_img), np.asarray(cand_img)
    src_ink, src_origin = content_ink(src)
    cand_ink, cand_origin = content_ink(cand)
    prior_canvas = max(src_ink.shape[0], src_ink.shape[1], cand_ink.shape[0], cand_ink.shape[1])
    prior_canvas = max(24, min(512, prior_canvas))
    prior_grid = min(parse_finest(grid_spec, prior_canvas), prior_canvas)
    csy, csx = ink_centroid(src_ink)
    cdy, cdx = ink_centroid(cand_ink)
    centroid_start = (
        go_round((csy - cdy) * prior_grid / prior_canvas),
        go_round((csx - cdx) * prior_grid / prior_canvas),
    )
    result = progressive_overlap(src_ink, cand_ink, grid_spec, shift_pct, starts=[centroid_start])
    dx = result["shift_px"]["x"] + (src_origin[0] - cand_origin[0])
    dy = result["shift_px"]["y"] + (src_origin[1] - cand_origin[1])
    aligned = translated(cand, dx, dy)

    # Compare luminance on the same canonical grid with the same final offset.
    canvas = result["canvas"]
    grid = result["grid"]
    gray_src, gray_src_origin = content_ink(np.repeat(np.asarray(Image.fromarray(src).convert("L"))[:, :, None], 3, axis=2))
    gray_cand, gray_cand_origin = content_ink(np.repeat(np.asarray(Image.fromarray(cand).convert("L"))[:, :, None], 3, axis=2))
    gray_dx = dx - (gray_src_origin[0] - gray_cand_origin[0])
    gray_dy = dy - (gray_src_origin[1] - gray_cand_origin[1])
    gray_shift_x = go_round(gray_dx * grid / canvas)
    gray_shift_y = go_round(gray_dy * grid / canvas)
    gs, gc = to_grid(gray_src, canvas, grid), to_grid(gray_cand, canvas, grid)
    gray_channel = overlap_scores(gs, gc, gray_shift_y, gray_shift_x)[0]
    channel_scores = {**result["channel_scores"], "gray": gray_channel}
    channels = ["gray", "r", "g", "b"]
    paths = {}
    for name in channels:
        p = out_dir / f"{name}.png"
        tint_overlay(src, aligned, name).save(p)
        paths[name] = str(p)

    crop_box = tuple(crop)
    full_panels = [raw_overlay(src, aligned, ch) for ch in ("gray", "r", "g", "b")]
    panel_labels = [
        f"GRAY {channel_scores['gray'] * 100:.2f}%",
        f"R {channel_scores['r'] * 100:.2f}%",
        f"G {channel_scores['g'] * 100:.2f}%",
        f"B {channel_scores['b'] * 100:.2f}%",
    ]
    if detail in paths:
        selected_img = crop_percent_image(Image.open(paths[detail]), *crop_box)
    elif detail in ("rg", "rb", "gb"):
        selected_img = crop_percent_image(raw_overlay(src, aligned, detail), *crop_box)
    else:
        panels = [crop_percent_image(p, *crop_box) for p in full_panels]
        crop_size = list(panels[0].size)
        selected_img = compose_board(panels, panel_labels)
        if width and selected_img.width != width:
            out_h = max(1, go_round(selected_img.height * width / selected_img.width))
            selected_img = selected_img.resize((width, out_h), Image.Resampling.NEAREST)
        selected_path = out_dir / "selected.png"
        selected_img.save(selected_path)
        montage = out_dir / "montage.png"
        compose_board(full_panels, panel_labels).save(montage)
        worst = worst_region(src, aligned)
        metrics = {
            "similarity_percent": round(result["score"] * 100, 2),
            "channel_similarity_percent": {k: round(v * 100, 2) for k, v in channel_scores.items()},
            "score_method": "墨迹重叠率（soft-ink Jaccard）, 3-channel mean; grayscale is reported separately",
            "threshold_percent": threshold,
            "threshold_policy": threshold_policy,
            "pass": result["score"] >= threshold / 100.0,
            "translation_px": {"x": dx, "y": dy},
            "translation_percent": {"x": round(dx * 100 / iw, 3), "y": round(dy * 100 / ih, 3)},
            "canvas": result["canvas"],
            "final_grid": result["grid"],
            "grid_levels": result["levels"],
            "tested_offsets": result["tested_offsets"],
            "shift_cells": result["shift_cells"],
            "alignment_runs": result.get("alignment_runs"),
            "source_size": [source_w, source_h],
            "render_size": [candidate_w, candidate_h],
            "comparison_canvas": [iw, canvas_h],
            "images": {**paths, "montage": str(montage), "selected": str(selected_path)},
            "selected_channel": detail,
            "crop_percent": list(crop),
            "crop_size": crop_size,
            "width": width,
            "output_size": list(selected_img.size),
            "worst_region": worst,
        }
        (out_dir / "metrics.json").write_text(json.dumps(metrics, ensure_ascii=False, indent=2), encoding="utf-8")
        return metrics, str(selected_path)
    crop_size = list(selected_img.size)
    # Same zoom vocabulary as view_pdf/view_image: `width` is the target output
    # width in px; 0 keeps the cropped panel at native pixel size (no resampling).
    if width and width != selected_img.width:
        out_h = max(1, go_round(selected_img.height * width / selected_img.width))
        selected_img = selected_img.resize((width, out_h), Image.Resampling.NEAREST)
    selected_path = out_dir / "selected.png"
    selected_img.save(selected_path)

    full_panels = [raw_overlay(src, aligned, ch) for ch in ("gray", "r", "g", "b")]
    montage = out_dir / "montage.png"
    compose_board(full_panels, [
        f"GRAY {channel_scores['gray'] * 100:.2f}%",
        f"R {channel_scores['r'] * 100:.2f}%",
        f"G {channel_scores['g'] * 100:.2f}%",
        f"B {channel_scores['b'] * 100:.2f}%",
    ]).save(montage)
    selected = str(selected_path)
    metrics = {
        "similarity_percent": round(result["score"] * 100, 2),
        "channel_similarity_percent": {k: round(v * 100, 2) for k, v in channel_scores.items()},
        "score_method": "墨迹重叠率（soft-ink Jaccard）, 3-channel mean; grayscale is reported separately",
        "threshold_percent": threshold,
        "threshold_policy": threshold_policy,
        "pass": result["score"] >= threshold / 100.0,
        "translation_px": {"x": dx, "y": dy},
        "translation_percent": {"x": round(dx * 100 / iw, 3), "y": round(dy * 100 / ih, 3)},
        "canvas": result["canvas"],
        "final_grid": result["grid"],
        "grid_levels": result["levels"],
        "tested_offsets": result["tested_offsets"],
        "shift_cells": result["shift_cells"],
        "alignment_runs": result.get("alignment_runs"),
        "source_size": [source_w, source_h],
        "render_size": [candidate_w, candidate_h],
        "comparison_canvas": [iw, canvas_h],
        "images": {**paths, "montage": str(montage), "selected": str(selected_path)},
        "selected_channel": detail,
        "crop_percent": list(crop),
        "crop_size": crop_size,
        "width": width,
        "output_size": list(selected_img.size),
        "worst_region": worst_region(src, aligned),
    }
    (out_dir / "metrics.json").write_text(json.dumps(metrics, ensure_ascii=False, indent=2), encoding="utf-8")
    return metrics, selected


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--original", required=True, type=Path)
    ap.add_argument("--pdf", required=True, type=Path)
    ap.add_argument("--out", required=True, type=Path)
    ap.add_argument("--detail", default="all", choices=["all", "gray", "r", "g", "b", "rg", "rb", "gb"])
    ap.add_argument("--left", type=float, default=0)
    ap.add_argument("--top", type=float, default=0)
    ap.add_argument("--right", type=float, default=100)
    ap.add_argument("--bottom", type=float, default=100)
    ap.add_argument("--width", type=int, default=0)
    ap.add_argument("--grid", default="0.95S")
    ap.add_argument("--shift", type=float, default=0.10)
    ap.add_argument("--pass-threshold", type=float, default=50.0, help="pass threshold in percent for the submit gate")
    ap.add_argument("--threshold-policy", default="fixed", choices=["fixed", "adaptive-v1"], help="fixed threshold or explicit adaptive threshold policy")
    ap.add_argument("--difficulty", type=float, default=0.5, help="normalized case difficulty for adaptive-v1 (0 easy, 1 difficult)")
    ap.add_argument("--confidence", type=float, default=0.5, help="model confidence for adaptive-v1 (0 low, 1 high)")
    ap.add_argument("--threshold-min", type=float, default=40.0, help="adaptive-v1 lower bound in percent")
    ap.add_argument("--threshold-max", type=float, default=75.0, help="adaptive-v1 upper bound in percent")
    args = ap.parse_args()
    threshold_policy = None
    if args.threshold_policy == "adaptive-v1":
        difficulty = min(1.0, max(0.0, args.difficulty))
        confidence = min(1.0, max(0.0, args.confidence))
        args.pass_threshold = threshold_for(difficulty, confidence, args.pass_threshold, args.threshold_min, args.threshold_max)
        threshold_policy = {"name": "adaptive-v1", "difficulty": difficulty, "confidence": confidence, "minimum": args.threshold_min, "maximum": args.threshold_max}
    original = Image.open(args.original).convert("RGB")
    args.out.mkdir(parents=True, exist_ok=True)
    rendered = args.out / "candidate.png"
    size_info = render_cached(args.pdf, rendered, original.size)
    metrics, selected = save_comparisons(args.original, rendered, args.out, args.detail, args.grid, args.shift, (args.left, args.top, args.right, args.bottom), args.width, threshold=args.pass_threshold, threshold_policy=threshold_policy)
    metrics.update(size_info)
    (args.out / "metrics.json").write_text(json.dumps(metrics, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({"metrics": metrics, "selected_image": selected}, ensure_ascii=False))


if __name__ == "__main__":
    main()
