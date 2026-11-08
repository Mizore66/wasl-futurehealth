# r1agent8 Evidence Investigator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a sanitized Gemini evidence investigator as one read in a deterministic offline replay, then measure whether it recovers recall while preserving r1photo8's contamination improvement.

**Architecture:** A Python sidecar turns allow-listed lane evidence into opaque-alias tool calls and terminal recommendations. Go applies those recommendations inside the existing reviewer and fixed-point loop, then materializes a complete offline run directory. Existing audit/report/funnel tools measure that directory unchanged; a Python driver iterates until no eligible item lacks a terminal record and verifies byte-identical replay.

**Tech Stack:** Python stdlib + `urllib`, Go `pipeline/`, existing Gemini OpenAI-compatible gateway, `scripts/pairfeat_compare.py`, Bash experiment runner.

**Spec:** `docs/superpowers/specs/2026-11-04-crc-r1agent8-evidence-design.md` (binding).

## Global Constraints

- Synthetic data only. No human double-labelling and no clinical-effectiveness claim.
- No live-NATS agent integration in this plan. The agent never writes `fhir.review`; human sign-off remains mandatory.
- No truth, answer-key, reviewer-decision, person-source, raw real report text or funnel access for the agent.
- Selection 881 883 887; validation 907 911 919 929 937. Debug only on used r1photo8 development lanes 827 829 839 and smoke 101.
- At most 2 Gemini calls and 6 tool calls per item; at most 1,500 uncached calls in selection and stress, 2,500 in validation. Provider errors or cap exhaustion abort.
- At least 80% eligible-item coverage and 100% valid citations on accepted `same`/`different` recommendations.
- Every earlier candidate's argv, gates, outputs and digests remain byte-identical.
- No `/tmp`, no `/Users` paths, no patient-level identifiers, secrets, binaries or models in Git. `PATH=/opt/anaconda3/bin:$PATH` for Python.
- Commit trailer `Co-Authored-By: Claude Code <noreply@anthropic.com>`. Path-only commits. Never commit `docs/superpowers/plans/2026-11-02-crc-link-recall.md`. No push or merge.
- After documentation, thin non-record raw artifacts; keep summaries and the run of record.

## File Structure

- `scripts/agent_evidence.py`: controller-side evidence loading, alias map, eligible-target construction and tool API.
- `scripts/agent_investigator.py`: Gemini function-calling, validation, cache, terminal-record writer, replay driver and smoke CLI.
- `pipeline/agentrecs.go`: recommendation loading, Go eligibility, reviewer read semantics and need/stats output.
- `pipeline/agentreplay.go`: fixed-point materializer and CLI implementation.
- `pipeline/sweep.go`, `pipeline/audit.go`: final-list access and optional offline trace hook.
- `scripts/pairfeat_compare.py`, `scripts/pairfeat_experiment.sh`: comparator arms, driver stage, metrics and gates.
- Tests beside each component; run artifacts outside Git under `~/wasl-ml-data/agents2/hardening/r1agent8/`.

## Data Contract

`agent_recs.ndjson` is outside the Gemini sandbox and contains raw registration IDs only after controller-side translation:

```json
{"item":"rv-...","pair":["provider|mrn-a","provider|mrn-b"],"label":"same|different|uncertain","citations":[{"tool":"get_record","arg":"reg_...","field":"dob","value":"1980-02-22"}],"valid":true,"reason":"","model":"pinned-model","prompt_ver":"agent-v1","schema_ver":"tools-v1","calls":2,"tokens":1234,"latency_s":1.25,"trace":[]}
```

Every investigated item gets a terminal record. Parse failure, invalid citation or per-item budget exhaustion becomes `label="uncertain", valid=true, reason=<stable reason>` with no citations. `valid=false` is reserved for malformed files and is rejected by Go. Coverage includes terminal abstentions; accepted `same`/`different` records require valid citations.

---

### Task 1: Sanitized evidence and tool API

**Files:**
- Create: `scripts/agent_evidence.py`
- Test: `scripts/test_agent_evidence.py`

**Interfaces:**
- Produces: `EvidenceStore.from_lane(lane_dir)`, `store.need_items(need_path)`, `store.tools_for(item_id)`, `store.terminal_record(item_id, model_output, metadata)`.
- `ToolAPI` methods: `get_pair_context(item_alias)`, `get_record(reg_alias)`, `get_identifiers(reg_alias)`, `get_report_fields(report_alias)`, `get_photo_verdict(photo_alias)`.
- Raw `pk`/item IDs remain controller-side. Gemini sees only HMAC aliases and tool outputs.

- [ ] **Step 1: Write failing sandbox and eligibility tests**

Use small synthetic lane fixtures, not real cohorts. `fixture_lane` contains four link items with empty reasons and matching `near/demographic_missing` trace rows: `rv-missing-dob` (one blank DOB, same known sex, no valid EID conflict), `rv-dob-conflict` (different real DOBs), `rv-sex-conflict` (same real DOB, different known sex) and `rv-eid-conflict` (different Luhn-valid EIDs). `fixture_need` lists all four item IDs. Test:

```python
def test_eligible_join_and_hard_conflict_refusal():
    store = EvidenceStore.from_lane(fixture_lane)
    ids = [x["item"] for x in store.need_items(fixture_need)]
    assert ids == ["rv-missing-dob"]
    assert "rv-dob-conflict" not in ids
    assert "rv-sex-conflict" not in ids
    assert "rv-eid-conflict" not in ids
```

Add tests proving:

- Only link-kind, empty-reason items joined to `near/demographic_missing` are eligible.
- Controller recomputes real-DOB, placeholder, sex and valid-EID conflict semantics before emitting a target.
- `../`, absolute paths, symlink escapes and unknown aliases are rejected.
- No bundle byte contains a truth QID, raw `pk_qid`, `wrong` flag, report answer key or reviewer decision.
- `get_report_fields` returns structured fields only and rejects keys `quote`, `quotes`, `text`, `raw`, `truth` and `answer`.
- `get_photo_verdict` returns cached reader fields only, never `PhotoTruth`.
- Pair context contains only opaque item/pair aliases and numeric model score; `Chain`, `Quote`, `Verdict`, `LoopID`, `Day` and `InSeq` are absent.
- Fabricated citations and values that differ from a fresh tool call are rejected.

- [ ] **Step 2: Watch the tests fail**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest scripts.test_agent_evidence
```

Expected: import failure because `agent_evidence.py` does not exist.

- [ ] **Step 3: Implement the evidence store**

Implement controller-only readers for `review_queue.json`, `agent_need.ndjson`, `prep_log.ndjson`, cohort FHIR and cached structured verdicts. Never pass file objects or paths to the tool consumer.

Use:

```python
class EvidenceStore:
    def __init__(self, records, identifiers, reports, photos, targets, salt): ...
    def need_items(self, need_path): ...
    def tools_for(self, item_id): ...

class ToolAPI:
    def get_pair_context(self, item_alias): ...
    def get_record(self, reg_alias): ...
    def get_identifiers(self, reg_alias): ...
    def get_report_fields(self, report_alias): ...
    def get_photo_verdict(self, photo_alias): ...
```

Alias with HMAC-SHA256 and a run-local salt. Resolve aliases only through strict `^(item|reg|rep|photo)_[a-f0-9]{32}$` patterns. Keep a per-item six-call counter. Return raw Emirates-ID comparison status (`equal`, `different`, `missing`, `invalid`) rather than raw digits.

`terminal_record` re-executes every citation and compares canonical JSON field/value. It translates aliases back to raw item/pair only in the final NDJSON record. Invalid output becomes a terminal uncertain record with a stable reason.

- [ ] **Step 4: Verify the suite**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest scripts.test_agent_evidence
```

All sandbox, conflict and citation tests must pass.

- [ ] **Step 5: Commit Task 1**

```bash
git add scripts/agent_evidence.py scripts/test_agent_evidence.py
git commit -m "Add sanitized evidence tools for investigator agent"
```

---

### Task 2: Cached Gemini investigator and replay driver

**Files:**
- Create: `scripts/agent_investigator.py`
- Test: `scripts/test_agent_investigator.py`

**Interfaces:**
- Consumes: Task 1 `EvidenceStore` and `ToolAPI`; Go `agentreplay` from Task 4.
- Produces: `investigate(store, item_id, client)`, `GeminiClient`, `iterate(args)`, `smoke(args)`.
- Environment: `WASL_LLM_URL`, `WASL_LLM_KEY`, `WASL_LLM_MODEL`; no secret in cache or logs.

- [ ] **Step 1: Write failing client, budget and cache tests**

Use a fake transport; no network in tests. Cover:

1. First response requests two tools, second returns valid `same`; output has two Gemini calls, valid citations and a terminal raw record.
2. Second response requests another tool instead of finishing: terminal `uncertain/per_item_budget`, no third call.
3. More than six tool calls: terminal `uncertain/tool_budget`.
4. Invalid JSON, unsupported label and fabricated citation: terminal uncertain with distinct reasons.
5. Network error, HTTP 429/500 and timeout: raise `ProviderError` and abort the driver; do not convert provider failure to a model abstention.
6. Global cap: 1,501st selection call and 2,501st validation call abort before sending.
7. Cache key includes model, prompt version, tool schema, bundle and full message history. Identical replay has zero uncached calls; changed history misses.
8. No URL, key or authorization header appears in cache, NDJSON or logs.

- [ ] **Step 2: Watch the tests fail**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest scripts.test_agent_investigator
```

Expected: import failure.

- [ ] **Step 3: Implement the Gemini client and investigation loop**

Implement `GeminiClient` with stdlib `urllib`, temperature 0, structured tool definitions and response caches. Follow the endpoint and retry style of `scripts/reader_cache_api.py`, but cap retries inside the per-item/global budgets. Provider errors always raise `ProviderError`.

`investigate` permits two responses: response 1 may request tools; response 2 must return final JSON. Validate with `store.terminal_record`. Cache each response by model, prompt/tool versions, bundle and complete message history.

Implement `write_recs` with write-once semantics: duplicate pair with different content is an error; identical retry is a no-op.

- [ ] **Step 4: Implement the replay driver and smoke CLI**

`iterate`:

1. Invoke `wsi7 agentreplay` with the current recs file and the arm's existing model/noise/reviewer flags.
2. If `agent_need.ndjson` is empty, stop. Otherwise call `investigate` for each need item, append terminal records and rerun from step 1.
3. Controls pass no recs and perform exactly one replay/materialization without Gemini; they do not iterate on `agent_need.ndjson`, because their uncovered items intentionally use ordinary human review. Only the candidate iterates on need items.
4. Run final replay a second time into a sibling check directory. Require byte equality for `fhir_log.ndjson`, `out_log.ndjson`, `prep_log.ndjson`, `review_queue.json`, `reviewer_decisions.ndjson` and `link_funnel.ndjson`; require zero new uncached calls.
5. Write `agent_replay.json` with eligible/covered/abstain/invalid/calls/tokens/latency/human-seconds-saved and the determinism result.

`smoke --lane USED_LANE --n 20` investigates at most 20 eligible items from an already-used r1photo8 development lane, with a 40-call cap. It does not generate a new cohort or run the guarded seed runner.

- [ ] **Step 5: Verify the suite**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest scripts.test_agent_investigator
```

All tests pass with the fake transport and a fake `agentreplay` executable.

- [ ] **Step 6: Commit Task 2**

```bash
git add scripts/agent_investigator.py scripts/test_agent_investigator.py
git commit -m "Add cached Gemini evidence investigator and replay driver"
```

---

### Task 3: Go agent recommendations and reviewer semantics

**Files:**
- Create: `pipeline/agentrecs.go`
- Modify: `pipeline/governance.go`, `pipeline/types.go`, `pipeline/sweep.go`
- Test: `pipeline/agentrecs_test.go`, `pipeline/governance_test.go`

**Interfaces:**
- Produces: `AgentRec`, `AgentRecs`, `LoadAgentRecs(path)`, `(*AgentRecs).read(it)`, `WriteAgentNeed(path, rp)`, `reviewLoopFull`.
- `ReviewerCfg.Agent *AgentRecs` with `json:"-"`; nil means byte-identical existing behavior.
- `Signoff.AgentRead string`, `Signoff.AgentS float64`; `DurationS` remains human-only.

- [ ] **Step 1: Write failing loader, eligibility and four-cell tests**

Cover all table rows exactly:

| Agent | Human r1 | Required result |
|---|---|---|
| same | same | final same; reads contain `r1` and `agent`; no human r2 seconds |
| same | different | existing r3 senior human decides; agent does not sign |
| different/uncertain | same | existing r2/r3 path and seconds unchanged |
| different/uncertain | different | existing rejection unchanged |

Also test: invalid rec, non-link item, DOB conflict, sex conflict and valid-EID conflict all fall back; duplicate pair is a load error; empty file is equivalent to nil; `CheckSignoff` still requires reviewer role; `r1` and `r3` hash draws match control; `agentread` never changes `DurationS`.

- [ ] **Step 2: Watch the tests fail**

```bash
cd pipeline && go test . -run 'AgentRec|AgentRead' -count=1
```

Expected: undefined AgentRecs/field failures.

- [ ] **Step 3: Implement loading and Go-side eligibility**

Use the data contract above. Key records by unordered registration pair, not loop-specific item ID. Reject malformed labels, duplicate pairs and invalid JSON.

`AgentRecs.read` returns a recommendation only when `DemoGuard` is on, `isLinkQuestion(it)` is true, `demoClassify` is `demoNeedsReview`, there is no valid-EID conflict, and a terminal record exists. `reviewLoopFull` refreshes the eligibility closure from each replay's linker. A rec for any other pair is ignored and counted as ineligible.

- [ ] **Step 4: Implement reviewer semantics**

Add `Agent *AgentRecs` to `ReviewerCfg` and the two optional sign-off fields. Insert the agent path after human r1 without changing `oneRead` reader names or draw order:

- r1 same + agent same: append `agent` to reads, set `AgentRead`/`AgentS`, skip only the human r2 block and keep final reviewer sign-off.
- r1 different + agent same: use existing r3 senior read and append `agent`, `r3`; final decision comes from r3.
- Otherwise execute the existing high-risk r2/r3 code unchanged.

`WriteAgentNeed` writes only eligible pairs with no terminal record as `{item,pair,p}`. Terminal uncertain records are not requested again.

Add `reviewLoopFull(o, P, cfg) (rp, sos, iters, list)` and keep `reviewLoop` as a thin wrapper with its old signature. Do not invoke the agent hook from parallel sweep jobs.

- [ ] **Step 5: Verify focused and full Go suites**

```bash
cd pipeline && go test . -run 'AgentRec|AgentRead|Dual|Demo' -count=1
cd pipeline && go vet . && go test . -count=1
```

- [ ] **Step 6: Commit Task 3**

```bash
git add pipeline/agentrecs.go pipeline/agentrecs_test.go pipeline/governance.go pipeline/governance_test.go pipeline/types.go pipeline/sweep.go
git commit -m "Add agent as one read in offline reviewer"
```

---

### Task 4: Offline run materializer and `agentreplay`

**Files:**
- Create: `pipeline/agentreplay.go`
- Modify: `pipeline/audit.go`, `pipeline/main.go`
- Test: `pipeline/agentreplay_test.go`

**Interfaces:**
- CLI: `wsi7 agentreplay -data COHORT -wasl MODELS -run OUTPUT_RUN [-agentrecs FILE]`.
- Uses Task 3 `reviewLoopFull`; writes `agent_need.ndjson` and `agent_stats.json` plus a complete `OUTPUT_RUN/main` directory.
- `ReplayTrace func(LinkTrace)` is nil by default and never added to live child argv.

- [ ] **Step 1: Write failing materialization tests**

Using a tiny offline fixture, assert:

1. `fhir_log.ndjson`, `prep_log.ndjson` and `link_funnel.ndjson` have contiguous matching sequence numbers.
2. Every final sign-off is serialized with `signoffResource` and `Nats-Msg-Id: signoff/<item>`.
3. Replaying the materialized log with `replay(..., false)` reproduces the loop's final signatures and zero lost outputs.
4. `review_queue.json` and deterministic `reviewer_decisions.ndjson` contain final items; decision `t` equals sign-off sequence, not wall-clock.
5. Two materializations are byte-identical; no Gemini or agent flags leak into live child argv.
6. `ReplayTrace` produces the same pair/cluster records as a direct live-style trace fixture and is nil for all existing callers.

- [ ] **Step 2: Watch the tests fail**

```bash
cd pipeline && go test . -run 'AgentReplay|Materialize' -count=1
```

Expected: undefined materializer/command failures.

- [ ] **Step 3: Implement the trace hook**

Add package-level `ReplayTrace func(LinkTrace)`, default nil. In `replay`, assign it to the new linker only when non-nil. Do not change existing behavior or use it from parallel sweep jobs. Add a final clean replay for tracing so earlier fixed-point iterations cannot leave stale records.

- [ ] **Step 4: Implement `agentReplay` and serialization**

Call `loadOffline(data, wasl)` and `reviewLoopFull`. Reuse existing serializers wherever possible:

- `LogMsg` for `fhir_log.ndjson`; final list for `prep_log.ndjson`.
- `signoffResource` for sign-offs; `OutEnvelope` for `out_log.ndjson`.
- `analyze` for `review_queue.json`; `writeStatus`/`linkerStatus` for `status-linker.json`.
- `newLinkFunnelWriter` for final trace, with the same candidate/ANN/model configuration as the arm.
- Deterministic reviewer decision lines ordered by sign-off sequence, including `agent_read`, `agent_s`, `sim_s` and `reads`.

Write a minimal `orchestrate.json` throughput stub, `agent_need.ndjson`, and `agent_stats.json` with eligible/covered/abstain/invalid/calls/tokens/latency/human-seconds-saved. Cohort `forged.json` and `consent_detail.json` remain in `-data`; do not copy truth into the run directory.

Register `case "agentreplay"` in `main.go` and the `-agentrecs` string flag. Restrict it to this parent command. Do not append agent flags to `v7Args`. Stamp policy/reviewer identity with `+agent` only when recs are loaded.

- [ ] **Step 5: Verify round-trip and full suites**

```bash
cd pipeline && go test . -run 'AgentReplay|Materialize|AgentRec' -count=1
cd pipeline && go vet . && go test . -count=1
```

- [ ] **Step 6: Commit Task 4**

```bash
git add pipeline/agentreplay.go pipeline/agentreplay_test.go pipeline/audit.go pipeline/main.go
git commit -m "Materialize deterministic offline agent replay lanes"
```

---

### Task 5: Comparator arms, harness metrics and gates

**Files:**
- Modify: `scripts/pairfeat_compare.py`, `scripts/pairfeat_experiment.sh`, `scripts/test_pairfeat_compare.py`

**Interfaces:**
- Arms: `r1photo8` guarded-human comparator, `r1photo8ref` unguarded comparator, `r1agent8` candidate. No default-model arm.
- `base_arm(candidate)` defaults to `baseline`, returning `r1photo8` only for r1agent8.
- Candidate: pairfeat8 shared chain, noise v3, image on, `agent_replay=True`, reference `r1photo8ref`, seeds from the spec.
- Driver stage runs between orchestrate and audit for all three arms; audit/report7/classifier consume the materialized directory.

- [ ] **Step 1: Pin old behavior and write failing harness tests**

Regenerate pairfeat7/r1photo7/r1photo8 characterization digests before editing. New tests must prove:

1. Arm tuple is exactly `(r1photo8, r1photo8ref, r1agent8)`; no default baseline appears.
2. All arms share pairfeat8, v3 cohorts and image settings; guarded and candidate have `-demoguard`, unguarded does not.
3. `agentreplay` stage appears for all arms; only candidate invokes Gemini and has a recs file.
4. Audit/report7/classifier read the materialized run directory; earlier candidates retain old paths/argv.
5. Gemini environment survives only into the sidecar stage and is stripped from Go subprocesses.
6. Selection/stress budget 1,500 and validation budget 2,500 are passed correctly.
7. Metrics come from the same offline decisions in every arm: recall, contamination, wrong pairs, human hours, safety and agent stats.
8. Candidate live-vs-offline divergence is reported only; final replay determinism is gated for all arms.
9. Comparator-only seed sharing is allowed, reference-only arms cannot be selected, and all old digests remain unchanged.

- [ ] **Step 2: Write failing gate tests**

For `_r1agent8_gate(unguarded, guarded, cand, kind)`, independently test:

- Recall below either comparator fails by 0.01pp.
- Candidate contamination above guarded fails; equal guarded passes.
- Selection/validation contamination or wrong pairs equal to unguarded fails strictly-below; stress equality passes.
- Hours above guarded or above 1.10× unguarded fails.
- Any SAFETY key in any arm fails; a second deterministic replay mismatch fails.
- Coverage 0.7999 fails, 0.80 passes; any accepted invalid citation fails.
- Impure clusters, wrong final sign-offs and live-vs-offline divergence do not decide pass.

- [ ] **Step 3: Watch tests fail**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest discover -s scripts -p 'test_*.py'
bash -n scripts/pairfeat_experiment.sh
```

- [ ] **Step 4: Implement arm mapping, driver stages and metrics**

Add the candidate configuration and `base_arm`. Generalize shared-chain arm maps, `arms`, `analyse`, `mode_settings`, `read_build`, shell rechecks and CLI root parsing without changing defaults for old candidates.

Insert the Task 2 `iterate` stage for all three arms. Point subsequent consumers at its final materialized directory. Preserve Gemini environment only for that Python process. Keep caches in the experiment parent, not a per-arm temporary work directory.

Aggregate eligible/covered/abstain/invalid/calls/tokens/latency/human-seconds-saved and the agent confusion matrix (computed against truth only during analysis). Keep human and agent time separate.

- [ ] **Step 5: Implement `_r1agent8_gate`**

Use exact `Fraction` arithmetic for recall and hours. Enforce both comparator relationships, SAFETY, deterministic replay, 80% coverage and 100% accepted-citation validity. Return structured reasons naming comparator, metric, counts and rule. Stop at first failure.

- [ ] **Step 6: Verify all script tests and runner syntax**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest discover -s scripts -p 'test_*.py'
bash -n scripts/pairfeat_experiment.sh
```

- [ ] **Step 7: Commit Task 5**

```bash
git add scripts/pairfeat_compare.py scripts/pairfeat_experiment.sh scripts/test_pairfeat_compare.py
git commit -m "Add r1agent8 comparator experiment and gates"
```

---

### Task 6: Smoke, fresh-seed experiment and documentation

**Files:**
- Modify only for docs: `pipeline/RESULTS.md`, `docs/BACKEND-HANDOFF.md`
- Artifacts: `~/wasl-ml-data/agents2/hardening/r1agent8/run-1/`

- [ ] **Step 1: Verify the run HEAD**

Require a clean tracked tree and ≥20 GB free. Run full Go vet/tests, all Python sidecar tests, 80+ fit tests, scorer tests, all harness tests and `bash -n`. Refit pairfeat8 at that HEAD with the unchanged v3 recipe and initialize the shared-chain experiment.

- [ ] **Step 2: Run the 20-item real-provider smoke**

Use an already-used r1photo8 development lane. Require 20 terminal records, no forbidden access, valid citations on every accepted non-abstention, zero provider errors and complete cache hits on replay. Record model/prompt/schema versions and all calls/tokens/latency. Do not spend beyond the 40-call smoke cap.

- [ ] **Step 3: Run selection and stop at the first failure**

Run arms `r1photo8`, `r1photo8ref`, `r1agent8` on 881 883 887. Require identical cohort/chain hashes and successful deterministic replay. Analyse with those exact three roots. Stop on the first failed gate or provider/cap error.

- [ ] **Step 4: Run stress and frozen validation only after selection passes**

Run dev-stress with the 1,500-call cap, analyse, freeze validation, then run 907 911 919 929 937 with the 2,500-call cap. Never tune on these seeds.

- [ ] **Step 5: Diagnose and document honestly**

Report both comparator tables, agent confusion matrix, coverage, abstentions, invalid outputs, citation validity, calls/tokens/latency, human seconds saved, uncovered/new items and deterministic replay results. Separate simulated-human error from agent errors and do not claim clinician performance. State whether live NATS shadow mode is or is not justified.

Add the r1agent8 section to `pipeline/RESULTS.md` and one paragraph to `docs/BACKEND-HANDOFF.md`; independently review every number and the no-labelling/no-clinical-claims framing. Commit those two files only.

- [ ] **Step 6: Thin and finish**

Keep the record run, caches, manifests, summaries and reviews. Remove smoke/scratch binaries and non-record raw lanes. Record the final HEAD, outcome and next-step recommendation. No push or merge without the user's explicit choice.

---

## Self-Review

- **Spec coverage:** Tasks 1–2 implement tools, sandboxing, Gemini, cache and budgets; Tasks 3–4 implement read semantics and full offline lanes; Task 5 implements all comparators, metrics and gates; Task 6 implements smoke, fresh-seed execution and documentation.
- **Resolved integration rulings:** a new `agentreplay` materializer is required because audit/report7 do not re-answer queues; all arms use offline metrics for comparability; candidate live-vs-offline divergence is reported while deterministic offline replay is gated; terminal abstentions are not repeatedly requested; stress uses the selection call cap because the spec names no separate stress cap.
- **No placeholders:** each task names concrete files, interfaces, test cases, commands and commit boundaries. Experimental outcomes are not prewritten.
- **Type/name consistency:** `agent_recs.ndjson`, `agent_need.ndjson`, `AgentRecs`, `reviewLoopFull`, `agentreplay`, `iterate`, `base_arm` and `_r1agent8_gate` are used consistently across tasks.
