#!/usr/bin/env python3
"""Reproducible targeted mutation testing for the Wasl Go API.

Each mutant is applied to a fresh temporary copy of pipeline/. No source file in the checkout is modified.
A non-zero focused Go test result kills the mutant; a passing non-equivalent mutant is preserved in the report
as surviving. Equivalent mutants are declared in the catalog and must pass their behavioral oracle.
"""

from __future__ import annotations

import argparse
import dataclasses
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
from typing import Iterable


class MutationError(RuntimeError):
    pass


@dataclasses.dataclass(frozen=True)
class Mutant:
    id: str
    file: str
    old: str
    new: str
    tests: str
    rationale: str
    equivalent: bool = False


CONTRACT = "TestAPIContractReadModelFiltersPaginationAndDashboard"
WORKFLOW = "TestAPIContractWorkflowReplayAndRestart"
FUZZ_SEEDS = "TestAPIContractMalformedWritersAreAtomicAcrossContentTypes|FuzzAPIContractWriters"

CATALOG = (
    Mutant(
        id="cases-remove-id-sort",
        file="api.go",
        old="\tids := append([]string{}, s.order...)\n\tsort.Strings(ids)\n\tsel := []map[string]interface{}{}\n",
        new="\tids := append([]string{}, s.order...)\n\tsel := []map[string]interface{}{}\n",
        tests=CONTRACT,
        rationale="The independent read model requires sorted IDs on every filtered page.",
    ),
    Mutant(
        id="cases-drop-programme-filter",
        file="api.go",
        old='return eq("programme", c.Programme) && eq("state", c.State)',
        new='return eq("state", c.State)',
        tests=CONTRACT,
        rationale="Programme is one conjunct in the compound list filter.",
    ),
    Mutant(
        id="cases-total-before-filter",
        file="api.go",
        old="\ttotal := len(sel)\n",
        new="\ttotal := len(s.order)\n",
        tests=CONTRACT,
        rationale="total is the full filtered count, not the canonical run count.",
    ),
    Mutant(
        id="cases-remove-negative-offset-clamp",
        file="api.go",
        old="\tif offset < 0 {\n\t\toffset = 0\n\t}\n",
        new="",
        tests=CONTRACT,
        rationale="Negative offsets normalize to zero and must never slice at a negative index.",
    ),
    Mutant(
        id="dashboard-drop-closed-counter",
        file="api.go",
        old="\t\tif c.Closed {\n\t\t\tclosed++\n\t\t\tbyProg[c.Programme][\"closed\"]++\n\t\t}",
        new="\t\tif c.Closed {\n\t\t\tbyProg[c.Programme][\"closed\"]++\n\t\t}",
        tests=CONTRACT,
        rationale="The top-level closed count must reconcile with the independent case model.",
    ),
    Mutant(
        id="actions-bypass-rbac",
        file="api.go",
        old="if err := Authorize(role, ActCoordinateCase); err != nil {",
        new="if err := Authorize(role, ActCoordinateCase); false {",
        tests=WORKFLOW,
        rationale="Only coordinator may append coordinator actions.",
    ),
    Mutant(
        id="actions-accept-stale-head",
        file="api.go",
        old="if head := s.caseHead(c); expected != head {",
        new="if head := s.caseHead(c); false {",
        tests="TestActionsStaleHeadConflict|TestActionsSameHeadConcurrencyExactlyOneWins|" + WORKFLOW,
        rationale="The overlay expected_head token permits exactly one same-head writer and rejects replay.",
    ),
    Mutant(
        id="actions-ignore-duplicate-keys",
        file="api.go",
        old="if err := rejectDuplicateKeys(raw); err != nil {",
        new="if err := rejectDuplicateKeys(raw); false {",
        tests="TestActionInputRejectsDuplicateKeysAndFormatCharacters",
        rationale="Coordinator JSON rejects duplicate keys instead of using last-value-wins.",
    ),
    Mutant(
        id="consent-allow-grant-after-optout",
        file="api_consent.go",
        old='if cur, _ := s.overlayLocked(c); cur["status"] == "opted_out" && req.Event != "consent_opted_out" {',
        new='if cur, _ := s.overlayLocked(c); cur["status"] == "impossible" && req.Event != "consent_opted_out" {',
        tests=WORKFLOW,
        rationale="Opt-out is terminal in this demo flow; a later grant is a conflict.",
    ),
    Mutant(
        id="undo-allow-self-compensation",
        file="api_undo.go",
        old='if na == normActor(origActor) || (evKindNew == evKindResolution && na == normActor(sstr(target, "actor"))) {',
        new='if na == "\\x00" {',
        tests="TestUndoSameActorAndUnauthorized|TestContestIsSeparateFromCanonicalAndNeedsResolution",
        rationale="Undo, contest and resolution require an independent actor.",
    ),
    Mutant(
        id="undo-skip-disk-integrity-check",
        file="api_undo.go",
        old="\t\terr = sameLedger(disk, s.reviews)\n",
        new="",
        tests="TestUndoRejectsTruncatedOrDeletedLedger|TestUndoLedgerTamperRejected",
        rationale="Undo fails closed when the on-disk and in-memory ledgers diverge.",
    ),
    Mutant(
        id="export-hash-loaded-at",
        file="api_export.go",
        old='if k != "loaded_at" {',
        new="if true {",
        tests="TestExportComparisonHashRestartStability|" + WORKFLOW,
        rationale="comparison_sha256 excludes only meta.loaded_at in addition to the two hash fields.",
    ),
    Mutant(
        id="shadow-trust-foreign-status",
        file="api_shadow_status.go",
        old="\tif len(out) == 0 {",
        new="\tif false {",
        tests="TestAPIContractShadowMalformedStatus",
        rationale="A foreign or empty status document returns the 503 invalid envelope, never a live status.",
    ),
    Mutant(
        id="review-drop-queue-membership-check",
        file="api.go",
        old="\t\tif !found {\n",
        new="\t\tif false {\n",
        tests="TestAPIContractWorkflowReplayAndRestart|FuzzAPIContractWriters",
        rationale="A non-empty review item must have been raised for the same case.",
    ),
    Mutant(
        id="equivalent-limit-condition-order",
        file="api.go",
        old="if limit <= 0 || limit > 500 {",
        new="if limit > 500 || limit <= 0 {",
        tests=CONTRACT,
        equivalent=True,
        rationale="Both comparisons are pure and short-circuit order has no observable effect.",
    ),
)


def apply_mutation(source: str, mutant: Mutant) -> str:
    count = source.count(mutant.old)
    if count != 1:
        raise MutationError(f"{mutant.id}: target occurs {count} times in {mutant.file}; want exactly once")
    return source.replace(mutant.old, mutant.new, 1)


def run_go_test(pipeline: str | Path, mutant: Mutant) -> subprocess.CompletedProcess[str]:
    command = ["go", "test", ".", "-run", "^(?:" + mutant.tests + ")$", "-count=1"]
    env = os.environ.copy()
    env["GOFLAGS"] = (env.get("GOFLAGS", "") + " -mod=readonly").strip()
    try:
        return subprocess.run(command, cwd=pipeline, env=env, text=True, capture_output=True, timeout=180, check=False)
    except subprocess.TimeoutExpired as exc:
        stdout = exc.stdout.decode() if isinstance(exc.stdout, bytes) else (exc.stdout or "")
        stderr = exc.stderr.decode() if isinstance(exc.stderr, bytes) else (exc.stderr or "")
        return subprocess.CompletedProcess(command, 124, stdout, stderr + "\ngo test timed out after 180s\n")


def evaluate_mutant(pipeline: str | Path, mutant: Mutant) -> dict:
    completed = run_go_test(pipeline, mutant)
    if completed.returncode != 0:
        status = "killed"
    elif mutant.equivalent:
        status = "equivalent"
    else:
        status = "surviving"
    output = (completed.stdout + completed.stderr).strip()
    # Keep enough failure context for reproduction without allowing an enormous report to hide other outcomes.
    if len(output) > 12000:
        output = output[:12000] + "\n... output truncated ..."
    return {
        "id": mutant.id,
        "file": mutant.file,
        "old": mutant.old,
        "new": mutant.new,
        "tests": mutant.tests,
        "rationale": mutant.rationale,
        "declared_equivalent": mutant.equivalent,
        "status": status,
        "exit_code": completed.returncode,
        "command": ["go", "test", ".", "-run", "^(?:" + mutant.tests + ")$", "-count=1"],
        "output": output,
    }


def summary(results: Iterable[dict]) -> dict[str, int]:
    counts = {"killed": 0, "surviving": 0, "equivalent": 0}
    for result in results:
        counts[result["status"]] += 1
    return counts


def selected_catalog(raw: str | None) -> list[Mutant]:
    if not raw:
        return list(CATALOG)
    wanted = set(raw.split(","))
    selected = [m for m in CATALOG if m.id in wanted]
    missing = wanted - {m.id for m in selected}
    if missing:
        raise MutationError("unknown mutant IDs: " + ",".join(sorted(missing)))
    return selected


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--only", help="comma-separated mutant IDs (default: full catalog)")
    parser.add_argument("--report", type=Path, help="also write the JSON report to this path")
    parser.add_argument("--artifacts", type=Path, help="preserve each mutated pipeline tree under this new directory")
    args = parser.parse_args(argv)
    root = Path(__file__).resolve().parents[1]
    source = root / "pipeline"
    mutants = selected_catalog(args.only)
    if args.artifacts and args.artifacts.exists():
        parser.error("--artifacts must name a new directory")
    results = []
    with tempfile.TemporaryDirectory(prefix="wasl-api-mutation-") as temp:
        for index, mutant in enumerate(mutants):
            if args.artifacts:
                destination = args.artifacts / f"{index:02d}-{mutant.id}" / "pipeline"
                destination.parent.mkdir(parents=True)
                isolated = destination
            else:
                isolated = Path(temp) / f"{index:02d}-{mutant.id}"
            shutil.copytree(source, isolated, ignore=shutil.ignore_patterns("testdata", "artifacts", "*.test"))
            target = isolated / mutant.file
            target.write_text(apply_mutation(target.read_text(), mutant))
            result = evaluate_mutant(isolated, mutant)
            results.append(result)
            print(f"{result['status']:10s} {mutant.id} [{mutant.tests}]", flush=True)
    report = {
        "schema": "wasl-api-mutation/1",
        "base_commit": subprocess.run(["git", "-C", root, "rev-parse", "HEAD"], text=True, capture_output=True, check=True).stdout.strip(),
        "go_version": subprocess.run(["go", "version"], text=True, capture_output=True, check=True).stdout.strip(),
        "results": results,
        "summary": summary(results),
    }
    rendered = json.dumps(report, indent=2, sort_keys=True) + "\n"
    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(rendered)
    print(rendered, end="")
    return 2 if report["summary"]["surviving"] else 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (MutationError, OSError, subprocess.CalledProcessError) as exc:
        print(f"mutation runner: {exc}", file=sys.stderr)
        sys.exit(1)
