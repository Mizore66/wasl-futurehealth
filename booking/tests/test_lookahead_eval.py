"""Reproducibility checks for the paired booking evaluation grid and selectable baseline."""
import os, sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from sim import lookahead_eval as ev  # noqa: E402


def test_grid_and_base():
    assert ev.ABLATION_OPTIMISERS["myopic"] == []
    assert ev.ABLATION_OPTIMISERS["greedy"] == ["--optimiser", "greedy"]
    assert ev.ABLATION_OPTIMISERS["lookahead"] == ["--optimiser", "lookahead"]
    recs = []
    for run, shares in [("r1", (0.5, 0.6, 0.8)), ("r2", (0.7, 0.6, 0.6))]:
        for opt, value in zip(("myopic", "greedy", "lookahead"), shares):
            recs.append({"run": run, "seed": 2026, "condition": "adequate", "stress": "base",
                         "optimiser": opt, "within_target_share": value})
    summ = ev.summarise(recs, base="greedy")
    la = summ["adequate|base|lookahead"]
    assert la["d_within_target_share"] == 0.1, la
    assert la["d_within_target_share_pos"] == 1 and la["d_within_target_share_neg"] == 0, la
    my = summ["adequate|base|myopic"]
    assert my["d_within_target_share"] == 0.0, my
    default = ev.summarise(recs)
    assert default["adequate|base|lookahead"]["d_within_target_share"] == 0.1
    return "ablation includes myopic/greedy/default-lookahead; --base selects paired delta reference"


if __name__ == "__main__":
    print(test_grid_and_base.__name__, "->", test_grid_and_base())
