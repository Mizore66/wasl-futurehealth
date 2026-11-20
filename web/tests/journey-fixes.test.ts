import { readFileSync } from "node:fs";
import path from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { CaseDetail, type CaseSummary, type ReviewState } from "@/lib/api/schema";
import { headerStatusWords, statusWords } from "@/components/worklist/model";
import { awaitingSeniorWords, claimAnnouncement, readEndsConcealment } from "@/components/review/review-desk";
import { awaitingObligationConfirmation, obligationConfirmationWords, settledWords } from "@/components/case/crc-confirmation";
import { recordedWords, reviewDateWords, reviewOverdue, todayIso } from "@/components/disposition/model";
import { ledgerWords } from "@/components/case/words";
import { requestState } from "@/components/case/patient-journey-card";
import { dayNumber, slotUnavailableWords } from "@/components/case/booking-words";
import { afterAnswerWords, needsDetail, returnedPacket } from "@/components/worklist/returned-packets";
import { mapSettledBounded } from "@/lib/domain/bounded";
import { patientScript } from "@/components/patient/programme";
import { PREVIEW_OFFERS } from "@/components/patient/book-flow";
import { elsewhereVisible } from "@/components/patient/elsewhere-report";
import { candidateSets, NO_VIEW, parseRecoveryView, recoveryHref, slotsByDay, CANDIDATE_PAGE } from "@/components/recovery/candidates";
import { paginate } from "@/components/worklist/paging";
import { TechTerm } from "@/components/wasl/tech-term";
import { ALL_DICTS } from "@/components/patient/i18n/all";
import { PatientJourney } from "@/lib/api/patient-schema";
import { loadSample } from "./samples";
import { patientFixture } from "./patient-fixture";

vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: () => {}, push: () => {} }) }));

const closedCase = (): CaseSummary => CaseDetail.parse(loadSample("case_evidence_closure.json")).case;

describe("B8: a patient contest holding closure never reads as closed", () => {
  it("says closed with evidence only when the API serves closed", () => {
    const c = closedCase();
    expect(c.closed).toBe(true);
    expect(headerStatusWords(c)).toBe("closed with evidence");
    const held = { ...c, closed: false, canonical_closed: true, closure_hold: "patient_contest" } as unknown as CaseSummary;
    expect(headerStatusWords(held)).not.toMatch(/closed with evidence|^closed/);
    expect(headerStatusWords(held)).toMatch(/held/);
    expect(statusWords(held).status).toBe("Closure held: patient contest open");
  });
});

describe("B2/B3/B4 review desk wording", () => {
  it("tells an eligible senior to claim, and only an ineligible viewer that a third read is not accepted", () => {
    expect(awaitingSeniorWords({ claim: true, renew: false, release: false, takeover: false })).not.toMatch(/not accepted/);
    expect(awaitingSeniorWords({ claim: true, renew: false, release: false, takeover: false })).toMatch(/Claim the senior resolution/);
    expect(awaitingSeniorWords({ claim: false, renew: false, release: false, takeover: false })).toMatch(/A third read from you is not accepted/);
    expect(awaitingSeniorWords(undefined)).toMatch(/not accepted/);
  });

  it("announces a claim once, never 'claim claim'", () => {
    expect(claimAnnouncement("link", "claim")).toBe("Identity question claimed. The current server state is shown.");
    expect(claimAnnouncement("link", "claim")).not.toMatch(/claim claim/i);
  });

  it("re-renders the withheld-history notice after the second read", () => {
    expect(readEndsConcealment("r2")).toBe(true);
    expect(readEndsConcealment("r1")).toBe(false);
    expect(readEndsConcealment("r3")).toBe(false);
  });
});

describe("B5: CRC obligation confirmation agrees with the review desk", () => {
  const need = { id: "crc-need-1", outcome: "gastroenterology", state: "awaiting_confirmation", source_report: "P1", source_version: 1, procedure_report: "C1",
    procedure_version: 1, clock_start: 45.427384259259256, review_item: "rv-713", expected_head: "h", owner: "gastroenterology", blockers: [] as string[] };
  const state = (s: ReviewState["state"]): ReviewState => ({ item: "rv-713", question: "evidence", state: s, head: "", next_read: null, may_submit: false, reads: [], disagreement: false, canonical_unchanged: true });

  it("asks for two independent clinicians, then reports the desk's progress as an overlay", () => {
    expect(obligationConfirmationWords(need, {})).toMatch(/two independent clinicians on the review desk/);
    expect(obligationConfirmationWords(need, { api_review_states: [state("awaiting_r2")] })).toMatch(/One of two/);
    const done = obligationConfirmationWords(need, { api_review_states: [state("resolved_agreed")] });
    expect(done).toMatch(/Independently confirmed/);
    expect(done).toMatch(/canonical record still shows the obligation awaiting/);
    expect(obligationConfirmationWords({ ...need, confirmation: "rv-713", reviewer: "c1" }, {})).toBe("Confirmed by c1, signature rv-713.");
    expect(obligationConfirmationWords({ ...need, state: "blocked", blockers: ["specimen"] }, {})).toMatch(/holds/);
  });

  it("never says 'not waiting for a signature' while an obligation awaits confirmation", () => {
    const base = CaseDetail.parse(loadSample("case_evidence_closure.json"));
    const detail = { ...base, case: { ...base.case, crc: { kind: "screening", screening_complete: true, care_state: "awaiting_confirmation", obligations: [need] } } };
    expect(awaitingObligationConfirmation(detail)).toBe(true);
    expect(settledWords(detail).title).not.toMatch(/not waiting/);
    expect(settledWords(base).title).toMatch(/not waiting/);
  });
});

describe("B7: a past plan review date is marked overdue", () => {
  const now = new Date("2026-11-18T10:00:00Z");
  it("marks only dates before today", () => {
    expect(todayIso(now)).toBe("2026-11-18");
    expect(reviewOverdue("2020-01-01", now)).toBe(true);
    expect(reviewOverdue("2026-11-18", now)).toBe(false);
    expect(reviewDateWords("2020-01-01", now)).toBe("2020-01-01 (overdue)");
    expect(reviewDateWords("2026-12-15", now)).toBe("2026-12-15");
    expect(recordedWords("clinician_disposition", { plan: "repeat_diagnostic", owner: "o", review_date: "2020-01-01" })).toMatch(/2020-01-01 \(overdue\)/);
  });
});

describe("B10: staff copy without raw identifiers", () => {
  it("labels patient rights requests instead of calling them unrecognised", () => {
    expect(ledgerWords({ ledger: "unknown", label: "patient_request/contest_submitted" })).toMatch(/Patient questioned the decision/);
    expect(ledgerWords({ ledger: "unknown", label: "patient_request/explanation_requested" })).toMatch(/explanation/);
    expect(ledgerWords({ ledger: "unknown", label: "patient_request/contest_resolved" })).toMatch(/resolved/);
    expect(ledgerWords({ ledger: "unknown", label: "mystery/x" })).toBe("Unrecognised ledger event (mystery/x)");
  });

  it("names roles in words and rounds run days", () => {
    const row = { receipt: "r", kind: "contest_submitted", decision: "d", submitted_at: "2026-11-18T00:00:00Z", reason: "x", status: "open", owner: "senior_clinician" } as never;
    expect(requestState(row)).toMatch(/^Pending · senior clinician review/);
    expect(dayNumber(31.89244212962963)).toBe("31");
    expect(dayNumber(undefined)).toBe("not recorded");
  });

  it("has no stray comma in the system-identifier disclosure", () => {
    const html = renderToStaticMarkup(createElement(TechTerm, { id: "review_claim/claim", label: "Review question claimed" }));
    expect(html).not.toContain(">, show");
    expect(html).toContain("(show the system identifier)");
  });
});

describe("Booking: an unreservable slot says why", () => {
  it("explains elapsed slots and a past suggestion", () => {
    expect(slotUnavailableWords([{ slot_id: "a", availability: "elapsed" }, { slot_id: "b", availability: "elapsed" }], "a", true)).toMatch(/already past/);
    expect(slotUnavailableWords([{ slot_id: "a", availability: "elapsed" }, { slot_id: "b", availability: "available" }], "a", false)).toMatch(/suggested slot is already past/);
    expect(slotUnavailableWords([{ slot_id: "a", availability: "available" }], "a", true)).toBe("");
  });
});

describe("Returned referrals: bounded, isolated detail reads and accurate copy", () => {
  it("bounds concurrency and isolates a failure to its own item", async () => {
    let inFlight = 0;
    let peak = 0;
    const out = await mapSettledBounded([1, 2, 3, 4, 5, 6, 7, 8, 9, 10], 4, async (n) => {
      inFlight++;
      peak = Math.max(peak, inFlight);
      await new Promise((r) => setTimeout(r, 5));
      inFlight--;
      if (n === 3) throw new Error("socket closed");
      return n * 2;
    });
    expect(peak).toBeLessThanOrEqual(4);
    expect(out[2].ok).toBe(false);
    expect(out.filter((r) => r.ok)).toHaveLength(9);
  });

  it("reads detail only for a canonical packet still waiting on the referrer", () => {
    const c = closedCase();
    const handoff = { referral_id: "x", source: "canonical" as const, reason_code: "r", responsible_clinician: "gp", decided_by: "skmc-endo", decided_at: 1, act_by: 3,
      medical_director_day: 5, status: "awaiting_referrer" as const, event: "e", info_requested: [], destinations: [] };
    const withHandoff = (h: object) => ({ ...c, closed: false, api_overlay: { ...(c.api_overlay ?? { head: "h" }), referral_handoff: h } }) as CaseSummary;
    expect(needsDetail(withHandoff(handoff))).toBe(true);
    expect(needsDetail(withHandoff({ ...handoff, status: "referrer_acted" }))).toBe(false);
    expect(needsDetail(withHandoff({ ...handoff, source: "unit_return" }))).toBe(false);
    expect(returnedPacket(withHandoff(handoff))).not.toBeNull();
  });

  it("does not promise procedure evidence after a close or primary-care answer", () => {
    expect(afterAnswerWords("close")).not.toMatch(/procedure evidence arrives/);
    expect(afterAnswerWords("manage_in_primary_care")).not.toMatch(/procedure evidence arrives/);
    expect(afterAnswerWords("re_refer")).toMatch(/procedure evidence arrives/);
  });
});

describe("Patient phone: closed case and no-offer wording", () => {
  const journey = (over: Partial<PatientJourney> = {}) => ({ ...patientFixture().journey, ...over }) as PatientJourney;

  it("a closed CRC case gets finished-follow-up wording, never the booking call", () => {
    const open = patientScript(journey(), "en");
    expect(open.notice.title).toBe("Your test result needs one more check");
    const closed = patientScript(journey({ case_closed: true }), "en");
    expect(closed.notice.title).toBe(ALL_DICTS.en.notice.closedTitle);
    expect(`${closed.notice.body} ${closed.notice.next}`).not.toMatch(/book a colonoscopy|needs one more check/);
    expect(PatientJourney.parse({ ...patientFixture().journey, case_closed: true }).case_closed).toBe(true);
  });

  it("the CRC no-offer text is not an error message", () => {
    const s = patientScript(journey(), "en");
    expect(s.book.noOffer).not.toMatch(/Nothing is shown or saved/);
    expect(s.book.noOffer).toMatch(/call you to arrange/);
    for (const lang of ["ar", "ur", "fil"] as const) expect(ALL_DICTS[lang].book.noOffer).not.toMatch(/Nothing is shown or saved/);
  });

  it("the translation preview has sample offers and no empty done-elsewhere region", () => {
    expect(PREVIEW_OFFERS.slots.length).toBeGreaterThan(1);
    expect(PREVIEW_OFFERS.procedure).toBe("colonoscopy");
    expect(elsewhereVisible(null, false)).toBe(false);
    expect(elsewhereVisible({ can_report: true } as never, false)).toBe(true);
  });
});

describe("Draft translations stay out of patient client bundles", () => {
  const root = path.join(__dirname, "..", "components", "patient");
  const src = (f: string) => readFileSync(path.join(root, f), "utf8");
  const CLIENT = ["book-flow.tsx", "check-flow.tsx", "outcome-handoff.tsx", "rights-requests.tsx", "elsewhere-report.tsx", "language-sheet.tsx", "patient-status.tsx",
    "parts.tsx", "programme.ts", "programme-copy.tsx", "journey.ts", "outcome-copy.ts", "elsewhere-copy.ts", "use-patient-journey.ts", "i18n/index.ts", "i18n/context.tsx", "i18n/review.ts"];

  it("no module a client component loads imports a draft dictionary", () => {
    for (const f of CLIENT) {
      const text = src(f);
      expect(text, f).not.toMatch(/from "\.\/(ar|ur|fil)"|from "\.\/i18n\/(ar|ur|fil|all|drafts)"|from "\.\/(all|drafts)"|i18n\/(all|drafts)"/);
    }
  });

  it("drafts are reachable only through the server-only module", () => {
    expect(src("i18n/drafts.ts")).toMatch(/^import "server-only";/);
    expect(src("phone-page.tsx")).toMatch(/dictsForSurface\(surface\)/);
  });
});

describe("Recovery board: paged candidates and day-ordered slots", () => {
  it("sorts slots by day and pages candidates", () => {
    expect(slotsByDay([{ day: 129, slot_id: "b" }, { day: 123, slot_id: "a" }, { day: 127, slot_id: "c" }]).map((s) => s.day)).toEqual([123, 127, 129]);
    const candidates = Array.from({ length: 60 }, (_, i) => ({ case_id: `c${i}`, eligible: i < 40 })) as never[];
    const sets = candidateSets({ candidates, backfill: { case_id: "c0", event: "e", status: "simulated_backfill_confirmed" } });
    expect(sets.assigned).toMatchObject({ case_id: "c0" });
    expect([sets.eligible.length, sets.excluded.length]).toEqual([39, 20]);
    const page2 = paginate(sets.eligible, 2, CANDIDATE_PAGE);
    expect([page2.pages, page2.rows.length, page2.rows[0].case_id]).toEqual([2, 14, "c26"]);
  });

  it("keeps the selected slot, candidate and pages in the URL and drops malformed values", () => {
    const view = parseRecoveryView({ slot: "rel_1", status: "filled", page: "2", case: "a/b", set: "excluded", cpage: "3" });
    expect(view).toEqual({ slot: "rel_1", filter: "filled", page: 2, case: "a/b", set: "excluded", cpage: 3 });
    expect(parseRecoveryView(Object.fromEntries(new URL(recoveryHref(view), "http://x").searchParams))).toEqual(view);
    expect(parseRecoveryView({ status: "closed", page: "0", set: "maybe", cpage: "1.5" })).toEqual(NO_VIEW);
    expect(recoveryHref(NO_VIEW)).toBe("/app/recovery");
  });
});
