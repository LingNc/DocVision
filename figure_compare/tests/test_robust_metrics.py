import unittest

import numpy as np
from PIL import Image, ImageDraw

from robust_metrics import balanced_score, ink_field, strict_soft_jaccard


def base_figure():
    image = Image.new("RGB", (256, 192), "white")
    draw = ImageDraw.Draw(image)
    draw.line((28, 160, 225, 160), fill="black", width=4)
    draw.line((42, 170, 42, 20), fill="black", width=4)
    draw.line((55, 142, 95, 105, 132, 116, 185, 38), fill="black", width=4)
    draw.ellipse((177, 30, 190, 43), fill="black")
    return image


class RobustMetricTests(unittest.TestCase):
    def test_exact_identity_is_perfect(self):
        a = ink_field(base_figure())
        result = balanced_score(a, a)
        self.assertAlmostEqual(result["strict_soft_ink_jaccard"], 1.0)
        self.assertAlmostEqual(result["balanced_visual_score"], 1.0)

    def test_small_translation_is_more_tolerated_than_strict_overlap(self):
        a = np.asarray(ink_field(base_figure()))
        shifted = np.zeros_like(a)
        shifted[1:, 1:] = a[:-1, :-1]
        self.assertGreater(balanced_score(a, shifted)["balanced_visual_score"], strict_soft_jaccard(a, shifted))

    def test_sparse_speckles_are_tolerated_more_than_missing_structure(self):
        a = np.asarray(ink_field(base_figure()))
        speckled = a.copy()
        speckled[::11, ::13] = np.maximum(speckled[::11, ::13], 0.8)
        missing = a.copy()
        missing[:, 38:48] = 0  # Remove the main vertical axis segment.
        noisy_score = balanced_score(a, speckled)["balanced_visual_score"]
        missing_score = balanced_score(a, missing)["balanced_visual_score"]
        self.assertGreater(noisy_score, missing_score)
        self.assertGreater(noisy_score, strict_soft_jaccard(a, speckled))

    def test_score_is_bounded_and_parameters_validated(self):
        a = np.asarray(ink_field(base_figure()))
        result = balanced_score(a, a)
        self.assertGreaterEqual(result["balanced_visual_score"], 0)
        self.assertLessEqual(result["balanced_visual_score"], 1)
        with self.assertRaises(ValueError):
            balanced_score(a, a, tolerant_weight=1.2)


if __name__ == "__main__":
    unittest.main()
