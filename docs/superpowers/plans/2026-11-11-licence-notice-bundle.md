# Licence Notice Bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a deterministic, offline-verifiable third-party notice bundle for the 81 reviewed npm packaging-concern rows without overstating provenance or clearing the 13 Sharp/native gaps.

**Architecture:** A tracked JSON evidence manifest references tracked SHA-256-bound notice texts. A small stdlib-only Python module validates exact lockfile membership and evidence classes, then renders a deterministic text bundle. The existing supply-chain shell and CI run validation before scanner-evidence comparison.

**Tech Stack:** Python 3.12 stdlib, JSON, Bash, existing `scripts/supply-chain.sh`, `unittest`, Trivy 0.76.2.

**Spec:** `docs/superpowers/specs/2026-11-11-licence-notice-bundle-design.md`; primary evidence: `supply-chain/LICENSE-REVIEW.md` addendum at lines 223 onward.

## Global Constraints

- Exactly 47 `direct-gitHead`, 21 `tag-candidate`, and 13 `unresolved-native` production rows; 81 unique name/version pairs.
- The 21 tag candidates are not direct npm-artifact-to-commit proof; the 13 Sharp/native rows remain unresolved.
- No legal-clearance claim and no exhaustive native/bundled-component audit claim.
- No network or third-party code execution during validation.
- No manually edited scanner revision metadata; regenerate only on a clean committed tree with Trivy 0.76.2.
- Preserve every existing scanner/gate behavior and all unrelated files.

---

### Task 1: Evidence validator and deterministic renderer

**Files:**
- Create: `scripts/notices.py`
- Create: `scripts/test_notices.py`
- Create: `supply-chain/notice-evidence.json` (one temporary fixture record at this task; production data arrives in Task 2)
- Create: `supply-chain/notices/mit-example.txt`

**Interfaces:**
- Consumes: existing `_npm_integrity(lockfile: Path) -> dict[(name, version), str]` from `scripts/supply_chain.py` for exact lockfile membership.
- Produces: `validate_notice_manifest(manifest: dict, root: Path, expected_counts: dict[str, int] | None = None) -> None`, raising `ValueError` on every invalid input; `render_notice_bundle(manifest: dict, root: Path) -> str`; CLI `python3 -I scripts/notices.py check --root ROOT` and `render --root ROOT --output PATH`. `root` is the repository root; notice `path` values resolve under `root / "supply-chain"`.

- [ ] **Step 1: Write failing validator tests**

Use `unittest` and `tempfile.TemporaryDirectory`. Build a root containing `web/pnpm-lock.yaml` and `supply-chain/`. The minimal lockfile must include:

```yaml
packages:
  example@1.0.0:
    resolution: {integrity: sha512-AAAA}
```

Create `supply-chain/notices/mit-example.txt` containing exactly `Example MIT notice\n` (one trailing newline). Its SHA-256 is `b2fdef494b60e26685728b6807eeb78bd91c2e6f62da0c9b79f78450eb628077`. Use this manifest shape:

```json
{
  "schema_version": 1,
  "packages": [{
    "name": "example",
    "version": "1.0.0",
    "declaration": "MIT",
    "evidence_class": "direct-gitHead",
    "source": {"url": "https://example.test/LICENSE", "commit": "0123456789012345678901234567890123456789"},
    "qualification": "Upstream copy; does not prove shipment in the npm tarball.",
    "notices": [{"path": "notices/mit-example.txt", "sha256": "b2fdef494b60e26685728b6807eeb78bd91c2e6f62da0c9b79f78450eb628077"}]
  }]
}
```

Test the valid record; missing file; wrong hash; lockfile at `1.0.1`; duplicate name/version; direct row with empty notices; tag row without `qualification`; unresolved row with notices; malformed schema; and output containing the exact name/version, declaration, evidence class, qualification and text. Add a production-count test calling `expected_counts={"direct-gitHead": 47, "tag-candidate": 21, "unresolved-native": 13}` against a deliberately small manifest and expecting failure.

- [ ] **Step 2: Run tests and confirm RED**

Run: `PATH=/opt/anaconda3/bin:$PATH python3 -m unittest -v scripts.test_notices`
Expected: import failure because `scripts/notices.py` does not exist.

- [ ] **Step 3: Implement the validator and renderer**

Implement the manifest shape above. Reject duplicate keys during JSON loading, non-UTF-8 texts, absolute/parent-escaping notice paths, duplicate package tuples, unknown evidence classes, empty declarations/source URLs, missing/invalid 40-character direct commits, missing tag `tag` and `resolved_commit`, missing notices on direct/tag rows, notices on unresolved rows, missing limitations on unresolved rows, stale lockfile versions, and SHA-256 mismatches. `expected_counts` must compare exact class totals and total uniqueness.

Render in input order with a fixed header, the three evidence classes, explicit upstream/tarball limitation, unresolved limitations, source URLs and exact notice texts. Do not include current time, machine paths or network fetches.

- [ ] **Step 4: Run tests and confirm GREEN**

Run: `PATH=/opt/anaconda3/bin:$PATH python3 -m unittest -v scripts.test_notices`
Expected: all validator/renderer tests pass; each negative test has a specific error assertion.

- [ ] **Step 5: Commit**

```bash
git add scripts/notices.py scripts/test_notices.py supply-chain/notice-evidence.json supply-chain/notices/mit-example.txt
git commit -m "feat: add offline licence notice validation" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

### Task 2: Populate the reviewed 81-row bundle

**Files:**
- Replace: `supply-chain/notice-evidence.json`
- Create: `supply-chain/notices/*.txt` (only exact texts documented in the review; deduplicate identical texts by content)
- Create: `supply-chain/THIRD-PARTY-NOTICES.txt`
- Modify: `scripts/test_notices.py`

**Interfaces:**
- Consumes: Task 1 validator/renderer and the exact rows, URLs, commits, qualifications and hashes in `supply-chain/LICENSE-REVIEW.md`.
- Produces: production evidence satisfying `expected_counts={"direct-gitHead": 47, "tag-candidate": 21, "unresolved-native": 13}` and deterministic bundle text.

- [ ] **Step 1: Add failing production-evidence tests**

Assert 81 unique rows and exact class totals; every row exists at the same version in `web/pnpm-lock.yaml`; seven Next.js and 14 Rolldown rows are `tag-candidate`; nine Sharp libvips and four Sharp compound rows are `unresolved-native`; every direct/tag notice hash matches; `THIRD-PARTY-NOTICES.txt` equals `render_notice_bundle(...)`. Assert the bundle contains `does not prove shipment` and an `UNRESOLVED` Sharp section, and never calls tag candidates `direct` or the bundle `legally cleared`.

- [ ] **Step 2: Run the production tests and confirm RED**

Run: `PATH=/opt/anaconda3/bin:$PATH python3 -m unittest -v scripts.test_notices`
Expected: count/lockfile failures against the Task 1 example manifest.

- [ ] **Step 3: Populate exact evidence and texts**

Transcribe the 81 exact records from the addendum, retaining its original row order and declarations. Use exact commit URLs/hashes for the 47 direct rows. For the seven Next.js and 14 Rolldown rows, include exact version tag and resolved commit plus the review's qualification that the tag is context, not direct artifact binding. For the nine libvips and four compound Sharp rows, include the precise unresolved limitation and no invented notice. Store each included upstream text under `supply-chain/notices/` and compute its actual SHA-256. Any retrieval must use only exact URLs in the review, a fresh empty per-record temporary directory, scripts outside that directory, and no package execution. Remove the Task 1 example record/text.

- [ ] **Step 4: Generate and verify the bundle**

Run:

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I scripts/notices.py render --root "$PWD" --output supply-chain/THIRD-PARTY-NOTICES.txt
PATH=/opt/anaconda3/bin:$PATH python3 -I scripts/notices.py check --root "$PWD"
PATH=/opt/anaconda3/bin:$PATH python3 -m unittest -v scripts.test_notices
```

Expected: offline check and all tests pass; rendering again produces no diff.

- [ ] **Step 5: Commit**

```bash
git add supply-chain/notice-evidence.json supply-chain/notices supply-chain/THIRD-PARTY-NOTICES.txt scripts/test_notices.py
git commit -m "docs: package reviewed third-party notices" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

### Task 3: Enforce the bundle in supply-chain verification and CI

**Files:**
- Modify: `scripts/supply-chain.sh:82-89`
- Modify: `scripts/test_supply_chain.py` shell-contract tests
- Modify: `scripts/test_ci_contract.py` workflow contract tests
- Modify: `docs/CI.md`

**Interfaces:**
- Consumes: Task 1 CLI `check --root ROOT`.
- Produces: every `generate` and `verify` invocation validates the manifest and exact generated bundle before reporting success.

- [ ] **Step 1: Add failing shell/CI contract tests**

Assert `scripts/supply-chain.sh` contains an offline call equivalent to:

```bash
"$PYTHON" -I "$ROOT/scripts/notices.py" check --root "$ROOT"
```

in the `generate|verify` path before its successful completion. Assert the checker runs for both modes, is not inserted only in `image`, and `docs/CI.md` documents it. Add a fake-Python shell test proving an unexpected checker failure is returned rather than ignored.

- [ ] **Step 2: Run focused tests and confirm RED**

Run: `PATH=/opt/anaconda3/bin:$PATH python3 -m unittest -v scripts.test_supply_chain.SupplyChainShellTests scripts.test_ci_contract.WorkflowTests`
Expected: missing-notice-check contract failure.

- [ ] **Step 3: Wire the checker and documentation**

Invoke `notices.py check` after manifest generation/comparison and before the final successful gate completion in `generate|verify`. Preserve `set -euo pipefail` behavior and all existing scanner calls. Document that no network is needed for notice validation and that 13 unresolved rows remain visible rather than being silently ignored.

- [ ] **Step 4: Run focused tests and shell syntax checks**

Run:

```bash
bash -n scripts/supply-chain.sh
PATH=/opt/anaconda3/bin:$PATH python3 -m unittest -v scripts.test_supply_chain.SupplyChainShellTests scripts.test_ci_contract.WorkflowTests
PATH=/opt/anaconda3/bin:$PATH python3 -I scripts/notices.py check --root "$PWD"
```

Expected: all pass; forced checker failure returns nonzero in the shell-contract test.

- [ ] **Step 5: Commit**

```bash
git add scripts/supply-chain.sh scripts/test_supply_chain.py scripts/test_ci_contract.py docs/CI.md
git commit -m "ci: enforce third-party notice evidence" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

### Task 4: Refresh provenance, document limits and verify the integrated result

**Files:**
- Modify: `supply-chain/README.md`
- Modify: `supply-chain/LICENSE-REVIEW.md` (append a bundle section; do not rewrite original row evidence)
- Regenerate: `supply-chain/repository.cdx.json`, `vulnerabilities.json`, `licenses.json`, `manifest.json` only through the existing generator

**Interfaces:**
- Consumes: committed Tasks 1–3 and pinned Trivy 0.76.2.
- Produces: current-revision scanner evidence and a documented offline notice-verification path.

- [ ] **Step 1: Document bundle usage and limitations**

Add exact commands:

```bash
python3 -I scripts/notices.py check --root "$PWD"
./scripts/supply-chain.sh verify
```

State 47/21/13 counts, tag-based qualification, upstream-text-not-tarball limitation, 13 unresolved Sharp/native records, and no legal-clearance or exhaustive bundled-component claim. Append rather than altering historical row evidence.

- [ ] **Step 2: Commit documentation before provenance generation**

```bash
git add supply-chain/README.md supply-chain/LICENSE-REVIEW.md
git commit -m "docs: explain notice bundle evidence limits" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
test -z "$(git status --porcelain)"
```

Expected: clean tree, excluding only explicitly ignored local artifacts.

- [ ] **Step 3: Regenerate and verify scanner provenance**

Run:

```bash
export PATH=/opt/anaconda3/bin:$PATH
trivy version --format json | python3 -I -c 'import json,sys; assert json.load(sys.stdin)["Version"] == "0.76.2"'
./scripts/supply-chain.sh generate
./scripts/supply-chain.sh verify
```

Expected: Trivy 0.76.2; current clean source revision in `manifest.json`; license/vulnerability gates and notice validation pass. Do not manually edit revision fields.

- [ ] **Step 4: Run final verification and independent review**

Run:

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -m unittest discover -s scripts -p 'test_*.py'
go -C pipeline test -count=1 -timeout=20m ./...
go -C pipeline vet ./...
git diff --check
```

Expected: all pass. If the known worktree-path Podman build fails, build the exact NATS context from a fresh `/tmp` directory and rerun that test with `WASL_NATS_TEST_IMAGE`; report the environment workaround without claiming the original path passed. Request an independent read-only review of Tasks 1–4; fix Critical/Important findings before pushing. Commit regenerated evidence and any review fixes:

```bash
git add supply-chain
git commit -m "chore: refresh supply-chain evidence" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
./scripts/supply-chain.sh verify
```

Expected: clean tracked tree and successful final verification. No plan is complete merely because files exist; the independent review and final commands are mandatory.
