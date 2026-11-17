import type { CaseSummary } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";

/*
 * Care still owed after a screening episode closed. The episode's own state, `overdue` and drop-off stay as the API
 * served them; this is a separate projection over the run clock (`now`, never the browser's wall clock).
 */

export type RecallState = "upcoming" | "due" | "overdue";

export type Obligation =
  | { kind: "dr_recall"; state: RecallState; contact: "rescreen" | "clinic_review"; fromDay: number; dueDay: number }
  | { kind: "ldct_tracking"; followupReport: string; child: string | null; childDueDay?: number };

/** Follow-up Lung-RADS categories that need no action of their own (ACR 2022: return to annual screening). */
const LDCT_ANNUAL = new Set(["LR1", "LR2"]);

/** Follow-up scan report → the id of the case that tracks it (the generated child loop), from a case list. */
export function ldctChildren(cases: readonly CaseSummary[]): ReadonlyMap<string, string> {
  const out = new Map<string, string>();
  for (const c of cases) if (c.programme === "lung_ldct" && c.ldct?.scan_report) out.set(c.ldct.scan_report, c.id);
  return out;
}

export function openObligation(c: CaseSummary, now: number, children: ReadonlyMap<string, string> = new Map()): Obligation | null {
  if (c.lifecycle === "retained" || !c.closed) return null;
  const next = c.dr?.next_due;
  if (next && !next.seen) {
    const day = Math.floor(now);
    const due = Math.floor(next.due_day);
    const state: RecallState = next.recall_overdue || day > due ? "overdue" : day === due ? "due" : "upcoming";
    return { kind: "dr_recall", state, contact: next.kind, fromDay: next.from_day, dueDay: next.due_day };
  }
  // The API links the closed parent to its child tracking request (`ldct_tracking`, matched on provider and scan). When
  // it is served, it is the whole answer: an untracked parent never borrows a report-id-only match from the list, which
  // could be another provider's loop. Only payloads without the field fall back to the list.
  const served = c.ldct_tracking;
  if (served) {
    const child = served.child_case_id ?? null;
    return { kind: "ldct_tracking", followupReport: served.followup_report, child, ...(served.child_due_day !== undefined ? { childDueDay: served.child_due_day } : {}) };
  }
  const { followup_report: report, followup_label: label } = c.ldct ?? {};
  if (report && label && !LDCT_ANNUAL.has(label)) return { kind: "ldct_tracking", followupReport: report, child: children.get(report) ?? null };
  return null;
}

/** A coordinator has something to do now on the closed case itself. A tracked LDCT follow-up is the child's task. */
export const isActionable = (o: Obligation | null): o is Extract<Obligation, { kind: "dr_recall" }> => o?.kind === "dr_recall" && o.state !== "upcoming";

const CONTACT = { rescreen: "diabetic eye rescreen", clinic_review: "eye-clinic review" } as const;
const DUE_LABEL = { rescreen: "Next screen", clinic_review: "Clinic review" } as const;

export function obligationWords(o: Obligation): { status: string; next: string } {
  if (o.kind === "ldct_tracking") {
    return o.child
      ? {
          status: `Closed · follow-up tracked as ${shortId(o.child)}`,
          next: `Act on ${shortId(o.child)}${o.childDueDay !== undefined ? ` (due ${formatDay(o.childDueDay)})` : ""}, not this case`,
        }
      : { status: "Closed · follow-up needs its own tracking request", next: "Open a tracking request for the follow-up scan" };
  }
  const what = CONTACT[o.contact];
  const when = { overdue: `overdue since ${formatDay(o.dueDay)}`, due: "due today", upcoming: `due ${formatDay(o.dueDay)}` }[o.state];
  return { status: `Closed · ${what} ${when}`, next: o.state === "upcoming" ? `Nothing until ${formatDay(o.dueDay)}` : `Arrange the ${what}` };
}

/** The Patients due column for a DR recall: the served next-due day, never referral + interval. */
export const recallDueWords = (o: Extract<Obligation, { kind: "dr_recall" }>) => `${DUE_LABEL[o.contact]} ${formatDay(o.dueDay)}`;
