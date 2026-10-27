import ast
import importlib.util
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SKILL_DIR = ROOT / "compare_figure_skill"
SKILL_SOURCE = SKILL_DIR / "compare.py"
ALGORITHM_SOURCE = ROOT / "algorithms" / "compare.py"


def load_threshold_function():
    tree = ast.parse(SKILL_SOURCE.read_text(encoding="utf-8"))
    node = next(n for n in tree.body if isinstance(n, ast.FunctionDef) and n.name == "threshold_for")
    module = ast.Module(body=[node], type_ignores=[])
    namespace = {}
    exec(compile(module, str(SKILL_SOURCE), "exec"), namespace)
    return namespace["threshold_for"]


def load_algorithm():
    spec = importlib.util.spec_from_file_location("figure_compare_algorithm_test", ALGORITHM_SOURCE)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class CompareFigureSkillTests(unittest.TestCase):
    def test_threshold_policy(self):
        threshold_for = load_threshold_function()
        self.assertEqual(threshold_for(), 50.0)
        self.assertGreater(threshold_for(0, 1), 50.0)
        self.assertGreater(threshold_for(0.5, 0), threshold_for(0.5, 1))
        self.assertLess(threshold_for(1, 0.5), 50.0)
        self.assertEqual(threshold_for(1, 0, minimum=50, maximum=62), 50.0)
        self.assertEqual(threshold_for(0, 0, minimum=47, maximum=60), 60.0)
        self.assertEqual(threshold_for(-4, 9), threshold_for(0, 1))
        with self.assertRaises(ValueError):
            threshold_for(minimum=70, maximum=40)

    def test_skill_contains_full_algorithm_without_parent_imports(self):
        source = SKILL_SOURCE.read_text(encoding="utf-8")
        canonical = ALGORITHM_SOURCE.read_text(encoding="utf-8")
        for name in ("render_pdf", "progressive_overlap", "content_ink", "raw_overlay", "worst_region", "save_comparisons"):
            self.assertIn(f"def {name}(", source)
            self.assertIn(f"def {name}(", canonical)
        self.assertNotIn("parents[", source)
        self.assertNotIn("algorithms/compare.py", source)

    def test_skill_folder_has_only_self_contained_runtime_files(self):
        files = {p.name for p in SKILL_DIR.iterdir() if p.is_file()}
        self.assertTrue({"compare.py", "SKILL.md", "viewer.html"}.issubset(files))

    @unittest.skipUnless(importlib.util.find_spec("numpy") and importlib.util.find_spec("PIL"), "requires NumPy and Pillow")
    def test_asymmetric_content_origin_compensation(self):
        import numpy as np
        from PIL import Image

        module = load_algorithm()
        reference = np.full((96, 96, 3), 255, dtype=np.uint8)
        candidate = np.full_like(reference, 255)
        reference[40:64, 30:58] = 0
        candidate[32:56, 29:57] = 0
        with tempfile.TemporaryDirectory() as td:
            td = Path(td)
            ref_path, cand_path = td / "reference.png", td / "candidate.png"
            Image.fromarray(reference).save(ref_path)
            Image.fromarray(candidate).save(cand_path)
            metrics, _ = module.save_comparisons(ref_path, cand_path, td / "out")
        self.assertEqual(metrics["translation_px"], {"x": 1, "y": 8})
        self.assertGreater(metrics["similarity_percent"], 95.0)
        self.assertGreaterEqual(len(metrics["alignment_runs"]), 2)


if __name__ == "__main__":
    unittest.main()
