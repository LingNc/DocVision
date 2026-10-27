import json
import tempfile
import unittest
from pathlib import Path

from PIL import Image, ImageDraw

from compare_vnext import evaluate, view_region


def diagram(missing=False):
    image = Image.new("RGB", (240, 180), "white")
    draw = ImageDraw.Draw(image)
    draw.line((30, 145, 210, 145), fill="black", width=3)
    draw.line((45, 155, 45, 20), fill="black", width=3)
    draw.line((60, 125, 95, 90, 130, 105, 175, 35), fill="black", width=3)
    if not missing:
        draw.ellipse((170, 28, 180, 38), fill="black")
    return image


class FigureEvidenceTests(unittest.TestCase):
    def test_identical_pair_is_perfect_and_has_overlay_evidence(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            diagram().save(root / "original.png")
            diagram().save(root / "candidate.png")
            result = evaluate(root / "original.png", root / "candidate.png", root / "eval")
            self.assertAlmostEqual(result["soft_ink_jaccard"], 1.0, places=6)
            self.assertAlmostEqual(result["robust_visual"]["balanced_visual_score"], 1.0, places=6)
            self.assertIsNone(result["decision"]["candidate_pass"])
            self.assertEqual(result["decision"]["status"], "needs_semantic_review")
            self.assertEqual(result["signals"]["dinov2"]["status"], "not_configured")
            self.assertTrue(Path(result["evidence"]["overlays"]["gray"]).is_file())
            self.assertTrue(Path(result["evidence"]["pixel_heatmap"]["image"]).is_file())
            second = evaluate(root / "original.png", root / "candidate.png", root / "eval")
            self.assertTrue(second["cache"]["hit"])
            region = view_region(root / "original.png", root / "candidate.png", (0, 0, 100, 100), root / "eval" / "region.png")
            self.assertTrue(region["cache_hit"])
            saved = json.loads((root / "eval" / "metrics.json").read_text(encoding="utf-8"))
            self.assertEqual(saved["schema"], "figure-evaluation/v1")

    def test_missing_component_lowers_score_and_region_uses_percent_box(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            diagram().save(root / "original.png")
            diagram(missing=True).save(root / "candidate.png")
            result = evaluate(root / "original.png", root / "candidate.png", root / "eval")
            self.assertLess(result["soft_ink_jaccard"], 1.0)
            region = view_region(root / "original.png", root / "candidate.png", (60, 0, 100, 55), root / "region.png")
            self.assertEqual(region["box_percent"], [60.0, 0.0, 100.0, 55.0])
            with Image.open(region["image"]) as region_image:
                self.assertTrue(region_image.width > region["source_crop_size"][0])

    def test_region_rejects_invalid_coordinates(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            diagram().save(root / "original.png")
            diagram().save(root / "candidate.png")
            with self.assertRaises(ValueError):
                view_region(root / "original.png", root / "candidate.png", (80, 0, 20, 100), root / "bad.png")


if __name__ == "__main__":
    unittest.main()
