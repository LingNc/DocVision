"""Contract and policy boundary for an optional independent semantic judge."""
from __future__ import annotations

from typing import Any


VERDICTS = {"pass", "revise", "reject", "uncertain"}
ELEMENT_STATUSES = {"match", "shifted", "distorted", "wrong", "missing"}


def validate_judgment(value: Any) -> dict:
    """Validate a structured judge response; reject malformed/free-form output."""
    if not isinstance(value, dict):
        raise ValueError("judge result must be a JSON object")
    verdict = value.get("verdict")
    if verdict not in VERDICTS:
        raise ValueError("verdict must be pass, revise, reject, or uncertain")
    score = value.get("semantic_score")
    if not isinstance(score, int) or isinstance(score, bool) or not 1 <= score <= 10:
        raise ValueError("semantic_score must be an integer in [1, 10]")
    confidence = value.get("confidence")
    if not isinstance(confidence, (int, float)) or isinstance(confidence, bool) or not 0 <= confidence <= 1:
        raise ValueError("confidence must be a number in [0, 1]")
    rationale = value.get("rationale")
    if not isinstance(rationale, str) or not rationale.strip():
        raise ValueError("rationale must be a non-empty string")
    elements = value.get("element_check", [])
    if not isinstance(elements, list):
        raise ValueError("element_check must be a list")
    normalized_elements = []
    for item in elements:
        if not isinstance(item, dict) or not isinstance(item.get("element"), str):
            raise ValueError("each element_check item requires an element string")
        if item.get("status") not in ELEMENT_STATUSES:
            raise ValueError("element status must be match, shifted, distorted, wrong, or missing")
        note = item.get("note", "")
        if not isinstance(note, str):
            raise ValueError("element note must be a string")
        normalized_elements.append({"element": item["element"], "status": item["status"], "note": note})
    boxes = value.get("evidence_boxes", [])
    if not isinstance(boxes, list):
        raise ValueError("evidence_boxes must be a list")
    normalized_boxes = []
    for box in boxes:
        if not isinstance(box, dict):
            raise ValueError("each evidence box must be an object")
        coords = [box.get(k) for k in ("left", "top", "right", "bottom")]
        if any(not isinstance(x, (int, float)) or isinstance(x, bool) for x in coords):
            raise ValueError("evidence box coordinates must be numbers")
        left, top, right, bottom = map(float, coords)
        if not (0 <= left < right <= 100 and 0 <= top < bottom <= 100):
            raise ValueError("evidence box must use a valid percentage rectangle")
        normalized_boxes.append({"left": left, "top": top, "right": right, "bottom": bottom,
                                 "claim": str(box.get("claim", ""))})
    missing = value.get("missing_in_redraw", [])
    if not isinstance(missing, list) or any(not isinstance(x, str) for x in missing):
        raise ValueError("missing_in_redraw must be a list of strings")
    return {"verdict": verdict, "semantic_score": score, "confidence": float(confidence),
            "rationale": rationale.strip(), "element_check": normalized_elements,
            "missing_in_redraw": missing, "evidence_boxes": normalized_boxes}


def combine_decision(robust_visual_score: float, judgment: dict | None, visual_floor: float = 0.25) -> dict:
    """Combine evidence conservatively; no judge means never semantic-pass."""
    if not 0 <= robust_visual_score <= 1:
        raise ValueError("robust_visual_score must be in [0, 1]")
    if visual_floor < 0 or visual_floor > 1:
        raise ValueError("visual_floor must be in [0, 1]")
    if judgment is None:
        return {"status": "needs_semantic_review", "candidate_pass": None,
                "reason": "semantic judge unavailable", "visual_floor": visual_floor}
    judgment = validate_judgment(judgment)
    if robust_visual_score < visual_floor:
        return {"status": "reject_visual_floor", "candidate_pass": False,
                "reason": "robust visual evidence is below the conservative floor",
                "visual_floor": visual_floor, "judge": judgment}
    # Judge veto has priority, including a low-confidence explicit reject.
    if judgment["verdict"] in {"reject", "revise"}:
        return {"status": "semantic_fail", "candidate_pass": False,
                "reason": "judge did not approve the redraw", "visual_floor": visual_floor,
                "judge": judgment}
    if judgment["verdict"] == "pass" and judgment["confidence"] >= 0.70:
        return {"status": "semantic_pass", "candidate_pass": True,
                "reason": "independent judge passed with sufficient confidence",
                "visual_floor": visual_floor, "judge": judgment}
    return {"status": "needs_semantic_review", "candidate_pass": None,
            "reason": "judge confidence is insufficient for an automatic decision",
            "visual_floor": visual_floor, "judge": judgment}
