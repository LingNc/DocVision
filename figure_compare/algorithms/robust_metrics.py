"""Deterministic noise-tolerant image scores for figure redraw experiments.

These are structural proxies, not semantic understanding. Keep strict soft-ink
Jaccard alongside them and calibrate weights on human-reviewed pairs.
"""
from __future__ import annotations

import numpy as np
from PIL import Image, ImageFilter


def ink_field(image: Image.Image | np.ndarray) -> np.ndarray:
    """Return normalized grayscale ink [0, 1], suppressing near-white background."""
    if isinstance(image, Image.Image):
        gray = np.asarray(image.convert("L"), dtype=np.float32) / 255.0
    else:
        arr = np.asarray(image)
        if arr.ndim == 3:
            # Rec. 709 luminance; channel order is RGB.
            gray = (0.2126 * arr[..., 0] + 0.7152 * arr[..., 1] + 0.0722 * arr[..., 2]).astype(np.float32) / 255.0
        elif arr.ndim == 2:
            gray = arr.astype(np.float32) / (255.0 if arr.max(initial=0) > 1 else 1.0)
        else:
            raise ValueError("image must be grayscale or RGB")
    return np.clip((0.985 - gray) / 0.785, 0.0, 1.0)


def strict_soft_jaccard(a: np.ndarray, b: np.ndarray) -> float:
    """Soft Jaccard on aligned grayscale ink fields."""
    if a.shape != b.shape:
        raise ValueError("ink fields must have the same shape")
    den = np.maximum(a, b).sum(dtype=np.float64)
    return float(np.minimum(a, b).sum(dtype=np.float64) / den) if den else 1.0


def _dilated_max(field: np.ndarray, radius: int) -> np.ndarray:
    out = field.copy()
    h, w = field.shape
    for dy in range(-radius, radius + 1):
        for dx in range(-radius, radius + 1):
            if dx * dx + dy * dy > radius * radius:
                continue
            y0, y1 = max(0, dy), min(h, h + dy)
            x0, x1 = max(0, dx), min(w, w + dx)
            if y1 <= y0 or x1 <= x0:
                continue
            out[y0:y1, x0:x1] = np.maximum(out[y0:y1, x0:x1], field[y0-dy:y1-dy, x0-dx:x1-dx])
    return out


def tolerant_ink_score(a: np.ndarray, b: np.ndarray, radius: int = 2) -> float:
    """Symmetric soft-ink coverage allowing small local displacement/aliasing."""
    if a.shape != b.shape:
        raise ValueError("ink fields must have the same shape")
    if radius < 0:
        raise ValueError("radius must be nonnegative")
    mass = float(a.sum(dtype=np.float64) + b.sum(dtype=np.float64))
    if mass <= 1e-12:
        return 1.0
    b_near_a = _dilated_max(b, radius)
    a_near_b = _dilated_max(a, radius)
    unmatched = np.clip(a - b_near_a, 0, None).sum(dtype=np.float64)
    unmatched += np.clip(b - a_near_b, 0, None).sum(dtype=np.float64)
    return float(np.clip(1.0 - unmatched / mass, 0, 1))


def _coarsen(field: np.ndarray, side: int = 64) -> np.ndarray:
    """Area-average into a fixed grid; isolated pixel noise largely vanishes."""
    h, w = field.shape
    # Pad to divisible dimensions with white/no-ink, not edge replication.
    side = max(8, int(side))
    out_h, out_w = min(side, h), min(side, w)
    y_edges = np.linspace(0, h, out_h + 1, dtype=int)
    x_edges = np.linspace(0, w, out_w + 1, dtype=int)
    out = np.zeros((out_h, out_w), dtype=np.float32)
    for y in range(out_h):
        for x in range(out_w):
            block = field[y_edges[y]:y_edges[y+1], x_edges[x]:x_edges[x+1]]
            out[y, x] = float(block.mean()) if block.size else 0.0
    return out


def coarse_structure_score(a: np.ndarray, b: np.ndarray, side: int = 64) -> float:
    """Low-frequency ink-mass overlap, less sensitive to rasterization detail."""
    if a.shape != b.shape:
        raise ValueError("ink fields must have the same shape")
    ca, cb = _coarsen(a, side), _coarsen(b, side)
    den = np.maximum(ca, cb).sum(dtype=np.float64)
    return float(np.minimum(ca, cb).sum(dtype=np.float64) / den) if den else 1.0


def balanced_score(a: np.ndarray, b: np.ndarray, radius: int = 2,
                   coarse_side: int = 64, tolerant_weight: float = 0.65) -> dict:
    """Return a transparent blend; this is not a semantic pass/fail verdict."""
    if not 0 <= tolerant_weight <= 1:
        raise ValueError("tolerant_weight must be between 0 and 1")
    strict = strict_soft_jaccard(a, b)
    tolerant = tolerant_ink_score(a, b, radius)
    coarse = coarse_structure_score(a, b, coarse_side)
    return {
        "strict_soft_ink_jaccard": strict,
        "tolerant_ink_coverage": tolerant,
        "coarse_structure_overlap": coarse,
        "balanced_visual_score": tolerant_weight * tolerant + (1 - tolerant_weight) * coarse,
        "parameters": {"tolerance_radius_px": radius, "coarse_grid_side": coarse_side,
                       "tolerant_weight": tolerant_weight},
        "interpretation": "visual-structure proxy; not semantic validation",
    }
