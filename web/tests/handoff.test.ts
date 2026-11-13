import { describe, expect, it } from "vitest";
import { CaseList, CurrentDecision } from "@/lib/api/schema";
import { addBusinessDays, handoffActBy, handoffUrgent } from "@/lib/domain/handoff";
import { formatDay } from "@/lib/domain/day";
import { handoffOf, handoffWords, referralFrom } from "@/components/worklist/referrals";
import { loadSample } from "./samples";

const page = CaseList.parse(loadSample("cases_crc_page1.json"));
const base = page.cases.find((c) => c.id.endsWith("sr-1011"))!;
const returned = (id: string) => ({ ...base, id: `ahs-mushrif/ServiceRequest/${id}`, state: "open" as const, closed: false, dropoff: "referral_rejected" as const });

describe("referral hand-off clock (mirrors pipeline/handoff.go)", () => {
  it("skips weekends and UAE holidays like addBD", () => {
    expect(addBusinessDays(0, 2)).toBe(4);
    expect(addBusinessDays(0.5, 5)).toBe(7.5);
    expect(addBusinessDays(24, 2)).toBe(32);
  });

  it("gives an urgent return two business days and a routine one five", () => {
    expect(handoffActBy(0, true)).toBe(4);
    expect(handoffActBy(0, false)).toBe(7);
    expect(handoffActBy(120, true)).toBe(123);
    expect(handoffActBy(120, false)).toBe(126);
  });

  it("treats every programme but cervical as urgent, and cervical by priority", () => {
    expect([handoffUrgent("crc", ""), handoffUrgent("cervical", "routine"), handoffUrgent("cervical", "urgent")]).toEqual([true, false, true]);
  });
});

describe("returned referrals", () => {
  const detail = (handoff?: object) => CurrentDecision.parse({ id: "d", loop: "l", state: "open", ...(handoff && { handoff }) });
  const handoff = (act_by: number, status: string) => handoffOf(detail({ act_by, status, clinical_priority: "urgent", decided_at: act_by - 2 }));

  it("carries each case's own canonical act-by date, not a shared run-day deadline", () => {
    const a = referralFrom(returned("A"), 120, false, handoff(103, "coordinator"));
    const b = referralFrom(returned("B"), 120, false, handoff(121, "awaiting_referrer"));
    expect([a?.handoff?.actBy, b?.handoff?.actBy]).toEqual([103, 121]);
  });

  it("words each status from the packet and never offers an overdue packet a new deadline", () => {
    const due = formatDay(103);
    expect(handoffWords(handoff(121, "awaiting_referrer"))).toEqual({ what: "Act on the return", when: `by ${formatDay(121)}` });
    expect(handoffWords(handoff(103, "coordinator"))).toEqual({ what: "Escalated to the care coordinator", when: `was due ${due}` });
    expect(handoffWords(handoff(103, "medical_director"))).toEqual({ what: "Escalated to the medical director", when: `was due ${due}` });
    expect(handoffWords(handoff(103, "referrer_acted"))).toEqual({ what: "The GP has acted", when: `was due ${due}` });
  });

  it("says the deadline is unavailable when the case has no hand-off", () => {
    const r = referralFrom(returned("C"), 120, false, null)!;
    expect(r.handoff).toBeNull();
    expect(handoffWords(r.handoff)).toEqual({ what: "Act on the return", when: "deadline unavailable" });
    expect(handoffOf(detail())).toBeNull();
  });
});
