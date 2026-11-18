import { describe, expect, it } from "vitest";
import { LANGS } from "@/components/patient/i18n";
import { ALL_DICTS as DICTS } from "@/components/patient/i18n/all";
import { OUTCOME_GATE_COPY } from "@/components/patient/outcome-copy";
import { outcomeWithheldLine } from "@/components/patient/outcome-handoff";
import { OUTCOME_NEXT_STEPS, PatientOutcomeResponse } from "@/lib/api/patient-schema";
import { CaseDetail } from "@/lib/api/schema";

describe("patient outcome hand-off", () => {
  it("shows content only for released or acknowledged hand-offs", () => {
    const t = DICTS.en.outcome;
    expect(outcomeWithheldLine("not_eligible", t)).toBe(t.notReady);
    expect(outcomeWithheldLine("pending_approval", t)).toBe(t.pending);
    expect(outcomeWithheldLine("pending_release", t)).toBe(OUTCOME_GATE_COPY.pendingRelease);
    expect(outcomeWithheldLine("held_consent", t)).toBe(t.held);
    expect(outcomeWithheldLine("suppressed", t)).toBe(t.suppressed);
    expect(outcomeWithheldLine("released", t)).toBeNull();
    expect(outcomeWithheldLine("acknowledged", t)).toBeNull();
  });

  it("has copy for every approvable next step in every language", () => {
    for (const lang of LANGS) {
      for (const step of OUTCOME_NEXT_STEPS) expect(DICTS[lang].outcome.next[step].length).toBeGreaterThan(0);
    }
  });

  it("parses the Go patient view and rejects a free-text next step", () => {
    const released = {
      handoff: {
        status: "released",
        delivery: "simulated",
        version: "a".repeat(64),
        approved_at: "2026-10-01T08:00:00Z",
        content: { outcome: "follow_up_complete", next_step: "surveillance", safety_net: "safety_net_v1" },
      },
    };
    expect(PatientOutcomeResponse.parse(released).handoff.content?.next_step).toBe("surveillance");
    const forged = { handoff: { ...released.handoff, content: { ...released.handoff.content, next_step: "start chemotherapy" } } };
    expect(PatientOutcomeResponse.safeParse(forged).success).toBe(false);
  });

  it("accepts the staff hand-off block on case detail", () => {
    const shape = CaseDetail.shape.outcome_handoff;
    const parsed = shape.parse({
      status: "acknowledged",
      referrer_status: "acknowledged",
      responsible_referrer: "referrer/ahs",
      consent: "granted",
      delivery: "simulated",
      history: [{ kind: "content_approved", version: "v", role: "clinician", actor: "dr-gp", at: "2026-10-01T08:00:00Z", event: "e1" }],
    });
    expect(parsed?.history[0].kind).toBe("content_approved");
  });

  it("accepts the release record, its consent gate and patient_informed", () => {
    const parsed = CaseDetail.shape.outcome_handoff.parse({
      status: "suppressed",
      referrer_status: "acknowledged",
      responsible_referrer: "referrer/ahs",
      consent: "denied",
      delivery: "simulated",
      patient_informed: false,
      release: { gate: "suppressed", at: "2026-10-01T09:00:00Z", actor: "coordinator-one", event: "e2" },
      history: [
        { kind: "content_approved", version: "v", role: "clinician", actor: "dr-gp", at: "2026-10-01T08:00:00Z", event: "e1" },
        { kind: "release_recorded", gate: "suppressed", version: "v", role: "coordinator", actor: "coordinator-one", at: "2026-10-01T09:00:00Z", event: "e2" },
      ],
    });
    expect(parsed?.release?.gate).toBe("suppressed");
    expect(parsed?.patient_informed).toBe(false);
  });

  it("parses a released but unverified patient view without content", () => {
    const view = PatientOutcomeResponse.parse({ handoff: { status: "released", delivery: "simulated", verified: false } });
    expect(view.handoff.content).toBeUndefined();
    expect(view.handoff.verified).toBe(false);
  });
});
