import { z } from "zod";
import type { CaseDetail } from "@/lib/api/schema";

/*
 * One clinician disposition workflow for both issues: the case action route (`POST /api/cases/{id}/actions`) with one
 * permission (`record_clinician_disposition`) and a typed `action` kind.
 *   clinician_disposition (#180): a plan for a hold that leaves the next step to a clinician (no fundal view; outside
 *     Fleischner). No review item is raised for these holds, so this workflow never depends on one.
 *   decline_disposition (#115): the informed-refusal answer to a coordinator-recorded patient decline.
 * Both are overlay records. Neither books anything, records a procedure, signs evidence or closes the screening loop.
 */

export const CLINICIAN_HOLDS = {
  clinician_decision_no_fundal_view: { programme: "diabetic_retinopathy", label: "No fundal view: the eye clinic could not assess the retina" },
  clinician_decision_outside_fleischner: { programme: "lung_nodule", label: "Outside Fleischner: the guideline does not apply to this patient" },
} as const;
export type ClinicianHold = keyof typeof CLINICIAN_HOLDS;

export const HOLD_PLANS = {
  repeat_diagnostic: "Repeat the diagnostic test",
  alternative_diagnostic: "A different diagnostic test",
  specialist_pathway: "Specialist pathway",
  primary_care_pathway: "Primary-care pathway",
} as const;

export const REFUSAL_REASONS = {
  patient_preference: "Patient preference",
  procedure_anxiety: "Anxiety about the procedure",
  logistics_or_access: "Logistics or access",
  comorbidity_or_frailty: "Comorbidity or frailty",
  prefers_alternative_test: "Prefers an alternative test",
  other: "Other (say why in counselling)",
} as const;

export const FIT_REOFFERS = {
  offered_accepted: "FIT re-offered and accepted",
  offered_declined: "FIT re-offered and declined",
  not_appropriate: "FIT not appropriate for this patient",
} as const;

const keys = <T extends object>(o: T) => Object.keys(o) as [keyof T & string, ...(keyof T & string)[]];
const text = z.string().trim().min(1).max(1000);
const day = z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Use a YYYY-MM-DD date");

/** What the browser may send. No role, actor, hold or decline event: the API binds those from the token and the case. */
export const DispositionInput = z.discriminatedUnion("action", [
  z.strictObject({
    action: z.literal("clinician_disposition"),
    reason: text,
    plan: z.enum(keys(HOLD_PLANS)),
    details: text,
    owner: z.string().trim().min(1).max(64),
    review_date: day,
  }),
  z.strictObject({
    action: z.literal("decline_disposition"),
    reason: z.enum(keys(REFUSAL_REASONS)),
    counselling: text,
    safety_net: text,
    plan: z.enum(["follow_up", "closed_declined"]),
    follow_up_owner: z.string().trim().min(1).max(64).optional(),
    next_review: day.optional(),
    fit_reoffer: z.enum(keys(FIT_REOFFERS)).optional(),
  }).superRefine((d, ctx) => {
    if (d.plan === "follow_up" && (!d.follow_up_owner || !d.next_review)) ctx.addIssue({ code: "custom", message: "A follow-up needs an owner and a review date" });
    if (d.plan === "closed_declined" && (d.follow_up_owner || d.next_review)) ctx.addIssue({ code: "custom", message: "The non-procedure end state takes no follow-up owner or review date" });
  }),
]);
export type DispositionInput = z.infer<typeof DispositionInput>;
export type DispositionKind = DispositionInput["action"];

type Contact = { at?: string; event?: string; outcome?: string; channel?: string };
type Recorded = Record<string, unknown> & { at?: string; event?: string; actor?: string; role?: string };

export type DispositionContext = {
  caseId: string;
  programme: string;
  expectedHead: string;
  /** The current clinician-decision hold, or null. */
  hold: ClinicianHold | null;
  /** The plan recorded for that hold, while the hold persists. */
  holdPlan: Recorded | null;
  /** The latest contact outcome when it is a decline, with whether a disposition answers it. */
  decline: (Contact & { answered: boolean }) | null;
  /** The latest decline disposition (it may answer an earlier decline). */
  declinePlan: Recorded | null;
  /** The kinds a clinician may record now. */
  open: DispositionKind[];
};

/** What the case needs from a clinician now, from the served detail only. */
export function dispositionContext(detail: CaseDetail): DispositionContext {
  const c = detail.case;
  const ov = (detail.api_overlay ?? c.api_overlay ?? {}) as Record<string, unknown>;
  /* The served escalations, else the latest decision's. The API refuses a plan when the current decision has no hold. */
  const escalations = c.escalations ?? detail.decisions?.at(-1)?.escalations ?? [];
  const hold = c.closed || c.lifecycle === "retained"
    ? null
    : ((escalations.find((e) => e in CLINICIAN_HOLDS && CLINICIAN_HOLDS[e as ClinicianHold].programme === c.programme) as ClinicianHold | undefined) ?? null);
  const contacts = (ov.contact_outcomes as Contact[] | undefined) ?? (ov.last_contact ? [ov.last_contact as Contact] : []);
  const last = contacts.at(-1);
  const declinePlan = (ov.decline_disposition as Recorded | undefined) ?? null;
  const decline = last?.outcome === "declined" ? { ...last, answered: !!declinePlan && declinePlan.decline_event === last.event } : null;
  const holdPlan = hold ? ((ov.clinician_disposition as Recorded | undefined) ?? null) : null;
  const open: DispositionKind[] = [];
  if (hold) open.push("clinician_disposition");
  if (decline && !decline.answered && c.lifecycle !== "retained") open.push("decline_disposition");
  return { caseId: c.id, programme: c.programme, expectedHead: String(ov.head ?? ""), hold, holdPlan, decline, declinePlan, open };
}

/** Plain words for a recorded disposition. Never says the procedure was done or the loop closed. */
export function recordedWords(kind: DispositionKind, d: Recorded): string {
  if (kind === "clinician_disposition") {
    const plan = HOLD_PLANS[d.plan as keyof typeof HOLD_PLANS] ?? String(d.plan);
    return `Plan recorded, not completed care: ${plan} · ${String(d.owner ?? "owner not recorded")} to review by ${String(d.review_date ?? "a date not recorded")}`;
  }
  if (d.plan === "closed_declined") return "Informed refusal recorded · non-procedure end state: no procedure was done and no screening result exists";
  return `Informed refusal recorded · ${String(d.follow_up_owner ?? "the GP")} reviews again on ${String(d.next_review ?? "a date not recorded")}`;
}
