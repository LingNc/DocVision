import unittest

from judge_contract import combine_decision, validate_judgment


VALID = {
    "element_check": [{"element": "axis", "status": "match", "note": ""}],
    "missing_in_redraw": [],
    "semantic_score": 9,
    "verdict": "pass",
    "confidence": 0.92,
    "rationale": "Key entities and relations are preserved.",
    "evidence_boxes": [{"left": 0, "top": 0, "right": 30, "bottom": 40, "claim": "axis"}],
}


class JudgeContractTests(unittest.TestCase):
    def test_valid_judge_pass_is_accepted_only_above_floor(self):
        result = combine_decision(0.8, VALID)
        self.assertEqual(result["status"], "semantic_pass")
        self.assertTrue(result["candidate_pass"])
        self.assertFalse(combine_decision(0.1, VALID)["candidate_pass"])

    def test_no_judge_never_claims_semantic_pass(self):
        result = combine_decision(0.99, None)
        self.assertIsNone(result["candidate_pass"])
        self.assertEqual(result["status"], "needs_semantic_review")

    def test_veto_wins_and_malformed_schema_rejected(self):
        veto = {**VALID, "verdict": "revise", "confidence": 0.1}
        self.assertFalse(combine_decision(0.9, veto)["candidate_pass"])
        with self.assertRaises(ValueError):
            validate_judgment({**VALID, "element_check": [{"element": "x", "status": "maybe"}]})
        with self.assertRaises(ValueError):
            validate_judgment({**VALID, "evidence_boxes": [{"left": 20, "top": 0, "right": 5, "bottom": 10}]})


if __name__ == "__main__":
    unittest.main()
