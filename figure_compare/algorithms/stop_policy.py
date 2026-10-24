"""Deterministic stopping policy for long-running redraw sessions.

The 500-round ceiling is only a final guard. Normal stopping is based on
observable artifact/state progress and optional time/cost budgets.
"""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass
class StopPolicy:
    max_rounds: int = 500
    no_improvement_rounds: int = 8
    repeated_state_rounds: int = 3
    max_elapsed_seconds: float | None = None
    max_cost: float | None = None
    best_score: float = -1.0
    stale_rounds: int = 0
    last_state: str | None = None
    repeated_rounds: int = 0
    history: list[dict[str, Any]] = field(default_factory=list)

    def observe(self, round_number: int, score: float | None = None,
                state_fingerprint: str | None = None, elapsed_seconds: float | None = None,
                cost: float | None = None) -> dict[str, Any]:
        if round_number < 1:
            raise ValueError("round_number must be positive")
        reasons: list[str] = []
        improved = score is not None and score > self.best_score + 1e-9
        if score is not None:
            if improved:
                self.best_score = score
                self.stale_rounds = 0
            else:
                self.stale_rounds += 1
        if state_fingerprint is not None:
            if state_fingerprint == self.last_state:
                self.repeated_rounds += 1
            else:
                self.last_state = state_fingerprint
                self.repeated_rounds = 0
        if round_number >= self.max_rounds:
            reasons.append("max_rounds")
        if self.no_improvement_rounds > 0 and self.stale_rounds >= self.no_improvement_rounds:
            reasons.append("no_improvement")
        if self.repeated_state_rounds > 0 and self.repeated_rounds >= self.repeated_state_rounds:
            reasons.append("repeated_state")
        if self.max_elapsed_seconds is not None and elapsed_seconds is not None and elapsed_seconds >= self.max_elapsed_seconds:
            reasons.append("time_budget")
        if self.max_cost is not None and cost is not None and cost >= self.max_cost:
            reasons.append("cost_budget")
        result = {
            "round": round_number,
            "stop": bool(reasons),
            "reasons": reasons,
            "improved": improved,
            "best_score": self.best_score,
            "stale_rounds": self.stale_rounds,
            "repeated_rounds": self.repeated_rounds,
        }
        self.history.append(result)
        return result
