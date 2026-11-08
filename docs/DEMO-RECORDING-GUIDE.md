# Three-minute demo recording guide

**Status:** script and shot list prepared; video not recorded. Record after Kai's frontend is ready. Use only synthetic fixtures. Do not show credentials, local paths, real patient data or unvalidated experiment claims.

## Setup

From the repository root, run the canonical backend and API:

```bash
scripts/demo.sh
scripts/demo.sh serve
```

The API is at `http://127.0.0.1:8088`; its run data is labelled `data_mode: simulated`. Use the frontend against this API. The static responses in `docs/api-samples/` are a backup, not a substitute for claiming a live run.

Rehearse these four views before recording:

1. Dashboard: EID-only versus truth/Wasl KPI and the review queue.
2. `evidence_closure`: linked identities, two evidence readers and closure timeline.
3. `false_closure_trap` or `review_hold`: the hold, human sign-off and reason no automatic closure occurred.
4. Audit view: verified hash chain, signatures and append-only review history.

## Timeline and narration

### 0:00–0:25 — the measurement problem

**Show:** dashboard KPI comparison, then the worklist.

**Say:** “A positive FIT test starts a 31-day clock, but the colonoscopy often happens at another provider, under another spelling of the patient's name. In the published Abu Dhabi baseline, only 30.5% had a colonoscopy, after a median 72 days. In our merged simulation, an EID-only tracker sees 12.4% within 31 days; truth is 33.0%; Wasl measures 31.9%, 1.1 points from truth. That is one simulated run (seed 2026); in the frozen five-seed results the Wasl gap varies by noise model, about 1 point under v1 and 3.1 under v2, so we do not claim every run matches. Wasl is built to show who actually needs follow-up.”

### 0:25–1:05 — one patient, across providers

**Show:** `evidence_closure`. Move from the Arabic-script referral registration to the Latin-script downstream registration, then to the evidence and closure blocks.

**Say:** “Here the clinic record and the hospital record have no shared Emirates ID. Wasl links them with its Arabic–Latin encoder, calibrated pair scorer and deterministic safety gate. The closure is not a name match alone: the link must pass the strict tier, and both evidence readers must agree on caecal completion, bowel preparation and whether histology is still required. This loop closes with the evidence and the full provenance chain attached.”

### 1:05–1:40 — uncertainty stays with a human

**Show:** the hold fixture, reader disagreement or missing demographic evidence, and the human sign-off.

**Say:** “When the evidence is incomplete, Wasl does not guess. A hard demographic conflict cannot be overridden. Missing evidence goes to review, and a merge or closure needs the configured independent reads. The evidence investigator is only one cited read: it sees sanitized aliases, cannot sign a review and cannot override a conflict. A human remains accountable.”

**Agent sentence rule:** use experimental **r1agent10** and immediately disclose the trade-off: “Development passed; stress recall equals guarded human and uses 10.8% less reviewer time, but has five fewer closures than unguarded. Validation did not run.” Do not call the agent a validated upgrade or an autonomous decision-maker.

### 1:40–2:05 — replayable, not just a dashboard

**Show:** verified audit chain and the frozen-run exactly-once/tamper evidence. Do not improvise a node kill during the final take.

**Say:** “Every decision can be replayed from the immutable input log. The integrated chaos tests found no lost or duplicate outputs, and the tamper suite detects forged or altered evidence. Consent is checked before release: unknown permission holds a message, and opt-out cancels it.”

### 2:05–2:30 — closure creates a capacity problem

**Show:** simulated booking and network-pooling view.

**Say:** “Finding the completed patients is only half the job. Wasl also shows the remaining queue and simulated capacity shortfall. When sites have misplaced capacity, pooling raises within-31-day completion from 22% to 42% in the simulation. When the whole network is short, software cannot create a colonoscopy slot; the dashboard says so.”

### 2:30–3:00 — evidence boundary and next step

**Show:** final dashboard with `simulated` visible and the pilot-gate slide.

**Say:** “These are synthetic cohorts with real public name and report-text sources, not clinical outcomes from Abu Dhabi patients. The 43 reports have clinically verified model-assisted labels, with κ of 1 after adjudication—not independent human double-labelling. Our next step is an unpaid shadow pilot: zero false closures and at least 95% recall against the coordinator's list before any live outreach. Wasl keeps the evidence, the safety gate and the human decision in one auditable loop.”

## Recording checks

- Keep the run/policy/model version visible in at least one shot.
- Say “simulated” for capacity, patient and reviewer results; never imply a clinical pilot has happened.
- Say **clinically verified model-assisted labels**, not independent human double-labelling or clinical ground truth. Do not claim buyer commitments, live Malaffi integration or live cloud-LLM inference.
- The 26B reader is cached in the canonical demo; the local 4B reader is live only when explicitly started in that mode.
- Record a clean backup take without the evidence-agent sentence. The r1agent10 take must include its exact experimental trade-off and must not imply that validation passed.
- Target 145–155 words per minute; trim pauses rather than speeding past the safety and limitation statements.
