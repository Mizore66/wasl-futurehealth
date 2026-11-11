import { z } from "zod";
import { Count, Dropoff as DropoffEnum, type Dashboard, type Dropoff, type Programme } from "@/lib/api/schema";
import { filtersHref, type CaseFilters } from "@/components/worklist/filters";
import { DROPOFF_WORDS } from "@/components/worklist/model";
import { result } from "@/lib/results";

export type Stall = { dropoff: Dropoff; label: string; count: number };

export type Rates = { eid: number; wasl: number; truth: number };

/** One source of 31-day rates, with the extra sourced detail each bar's tooltip shows. */
export type KpiSet = { rates: Rates; detail: Record<keyof Rates, string> };

export type ProgrammeRow = {
  key: Programme;
  name: string;
  cases: number;
  closed: number;
  overdue: number;
  /** cases - closed - overdue; null when that is negative, so the bar shows closed and overdue only. */
  open: number | null;
  /** The patient list for this programme (the API filters on `programme`). */
  href: string;
};

export type StallGroup = { pathway: string; stalls: Stall[] };

export type CapacityRegion = { key: string; name: string; slots: number; cases: number };

export type CapacityView = {
  /** "adequate", "misallocated" or "short_overall", as the run states it. */
  condition: string;
  /** The condition in plain words. */
  conditionWords: string;
  poolingNote: string;
  poolingRelevant: boolean;
  slots: number;
  openCases: number;
  regions: CapacityRegion[];
  /** Fractions 0 to 1 for share; whole numbers for the rest. */
  summary: { within31dShare: number; medianWaitDays: number; crossRegion: number; nurseCalls: number; noSlot: number };
  note: string;
};

export type ProgrammeView = {
  /** Percent, 0 to 100. The demo run, which is also `kpi.demo`. */
  rate: Rates;
  /** The two sets of 31-day rates the toggle switches between, percent. */
  kpi: { demo: KpiSet; heldout: KpiSet };
  programmes: ProgrammeRow[];
  /** Every sourced stall, by pathway, largest first within a pathway. */
  allStalls: StallGroup[];
  /** null when the run has no capacity block this page can read. */
  capacity: CapacityView | null;
  /** Provider and origin patterns from the dashboard's small-cell suppressed tables. */
  patterns: Patterns;
  /** Aggregate queue-lever counts from `capacity.levers`; null when the run has none this page can read. */
  levers: LeverView | null;
  /** The DoH acceptable level, read from the KPI definition; null when the definition no longer states it. */
  target: number | null;
  /** N in "1 in N on your chase list is already done"; null when the run has no CRC closure metrics. */
  chaseOneIn: number | null;
  stalls: Stall[];
  /** Colorectal and cervical cases, the programmes the stalls come from. */
  cases: number;
};

const STALL_LABEL: Partial<Record<Dropoff, string>> = {
  never_notified: "Never notified",
  no_acknowledgement: "No acknowledgement",
  not_booked: "Not booked",
  referral_rejected: "Referral rejected",
  referral_unanswered: "Referral unanswered",
  no_show: "Did not attend",
  inadequate: "Repeat needed",
  awaiting_histology: "Awaiting histology",
};

/** Plain words for the stalls in the lung, eye and breast pathways. */
const OTHER_STALL_LABEL: Partial<Record<Dropoff, string>> = {
  no_followup_ct: "No follow-up CT booked",
  escalated: "With the specialist team",
  out_of_scope: "Needs a clinician's decision",
  not_assessable: "Not assessable, repeat needed",
  no_eye_exam: "No eye-clinic exam yet",
  ungradable_no_repeat: "Retinal photo ungradable, no repeat",
  technical_repeat_not_done: "Mammogram repeat not done",
  no_assessment_outcome: "No assessment outcome",
  awaiting_tissue: "Awaiting tissue sampling",
  incomplete_scan_not_completed: "Incomplete LDCT, repeat not done",
  followup_ldct_not_done: "Follow-up LDCT not done",
  no_workup_outcome: "No work-up outcome",
  no_followup_documented: "No follow-up visit documented",
};

const PATHWAYS: { pathway: string; stalls: Dropoff[] }[] = [
  { pathway: "Colorectal and cervical", stalls: ["never_notified", "no_acknowledgement", "not_booked", "referral_rejected", "referral_unanswered", "no_show", "inadequate", "awaiting_histology"] },
  { pathway: "Lung nodule", stalls: ["no_followup_ct", "escalated", "out_of_scope", "not_assessable"] },
  { pathway: "Diabetic retinopathy", stalls: ["no_eye_exam", "ungradable_no_repeat"] },
  { pathway: "Breast", stalls: ["technical_repeat_not_done", "no_assessment_outcome", "awaiting_tissue"] },
  { pathway: "Lung cancer screening (LDCT)", stalls: ["incomplete_scan_not_completed", "followup_ldct_not_done", "no_workup_outcome"] },
  { pathway: "IFHAS cardiometabolic follow-up", stalls: ["no_followup_documented"] },
];

export const PROGRAMME_NAME: Record<Programme, string> = {
  crc: "Colorectal",
  cervical: "Cervical",
  lung_nodule: "Lung nodule",
  diabetic_retinopathy: "Diabetic retinopathy",
  breast: "Breast",
  lung_ldct: "Lung screening (LDCT)",
  ifhas: "IFHAS cardiometabolic",
};

const REGION_NAME: Record<string, string> = { AA: "Al Ain", AD: "Abu Dhabi", DH: "Al Dhafra" };

const CONDITION_WORDS: Record<string, string> = {
  adequate: "Adequate: enough slots, in the right places",
  misallocated: "Misallocated: slots exist, but in the wrong region",
  short_overall: "Short overall: fewer slots than open cases",
};

const Capacity = z.object({
  condition: z.object({
    condition: z.string(),
    pooling_note: z.string(),
    pooling_relevant: z.boolean(),
    slots: z.number(),
    open_cases_needing_slot: z.number(),
    by_region: z.record(z.string(), z.object({ cases: z.number(), slots: z.number() })),
  }),
  note: z.string(),
  summary: z.object({
    within_31d_share: z.number(),
    median_wait_days: z.number(),
    cross_region: z.number(),
    nurse_call: z.number(),
    no_slot: z.number(),
  }),
});

function capacityView(d: Dashboard): CapacityView | null {
  const parsed = Capacity.safeParse(d.capacity);
  if (!parsed.success) return null;
  const { condition: c, summary: s, note } = parsed.data;
  return {
    condition: c.condition,
    conditionWords: CONDITION_WORDS[c.condition] ?? c.condition.replaceAll("_", " "),
    poolingNote: c.pooling_note,
    poolingRelevant: c.pooling_relevant,
    slots: c.slots,
    openCases: c.open_cases_needing_slot,
    regions: Object.entries(c.by_region).map(([key, r]) => ({ key, name: REGION_NAME[key] ?? key, slots: r.slots, cases: r.cases })),
    summary: { within31dShare: s.within_31d_share, medianWaitDays: s.median_wait_days, crossRegion: s.cross_region, nurseCalls: s.nurse_call, noSlot: s.no_slot },
    note,
  };
}

function heldoutSet(): KpiSet {
  const one = (arm: string) => {
    const r = result(`kpi31.heldout.${arm}`);
    const [lo, hi] = r.ci95 ?? [];
    return { rate: pctOf(r.value), detail: lo === undefined || hi === undefined ? "Mean of 5 seeds" : `Mean of 5 seeds, 95% range ${pct(pctOf(lo))} to ${pct(pctOf(hi))}` };
  };
  const [eid, wasl, truth] = [one("eid_only"), one("wasl"), one("truth")];
  return { rates: { eid: eid.rate, wasl: wasl.rate, truth: truth.rate }, detail: { eid: eid.detail, wasl: wasl.detail, truth: truth.detail } };
}

function demoSet(kpi: Dashboard["metrics"]["crc_kpi_31d"]): KpiSet {
  const arm = (a: typeof kpi.eid_only) => ({ rate: pctOf(a.kpi_all_referrals), detail: `${a.within_31d.toLocaleString("en-US")} of ${a.eligible_referrals.toLocaleString("en-US")} eligible referrals` });
  const [eid, wasl, truth] = [arm(kpi.eid_only), arm(kpi.wasl_live), arm(kpi.truth)];
  return { rates: { eid: eid.rate, wasl: wasl.rate, truth: truth.rate }, detail: { eid: eid.detail, wasl: wasl.detail, truth: truth.detail } };
}

/** The stall drop-offs are the colorectal and cervical pathway's; the other programmes have their own. */
const STALL_PROGRAMMES = ["crc", "cervical"] as const;

const pctOf = (x: number) => Math.round(x * 1000) / 10;

export function programmeView(d: Dashboard): ProgrammeView {
  const kpi = d.metrics.crc_kpi_31d;
  const demo = demoSet(kpi);
  const target = /acceptable >(\d+)%/.exec(kpi.definition);

  const crc = d.metrics.closure.crc;
  const chaseList = crc ? crc.loops - crc.baseline_eid_only.closed_pred : 0;
  const alreadyDone = crc ? crc.wasl_live_with_review.closed_pred - crc.baseline_eid_only.closed_pred : 0;

  const stalls = Object.entries(d.dropoff)
    .flatMap(([key, count]) => {
      const label = STALL_LABEL[key as Dropoff];
      return label && typeof count === "number" ? [{ dropoff: key as Dropoff, label, count }] : [];
    })
    .sort((a, b) => b.count - a.count)
    .slice(0, 3);


  const programmes = (Object.keys(PROGRAMME_NAME) as Programme[]).flatMap((key) => {
    const p = d.by_programme[key];
    if (!p) return [];
    const open = p.cases - p.closed - p.overdue;
    return [{ key, name: PROGRAMME_NAME[key], cases: p.cases, closed: p.closed, overdue: p.overdue, open: open >= 0 ? open : null, href: `/app/patients?programme=${key}` }];
  });

  const label = (k: Dropoff) => STALL_LABEL[k] ?? OTHER_STALL_LABEL[k];
  const allStalls = PATHWAYS.map(({ pathway, stalls: keys }) => ({
    pathway,
    stalls: keys
      .flatMap((k) => {
        const count = d.dropoff[k];
        const l = label(k);
        return l && typeof count === "number" ? [{ dropoff: k, label: l, count }] : [];
      })
      .sort((a, b) => b.count - a.count),
  })).filter((g) => g.stalls.length > 0);

  return {
    rate: demo.rates,
    kpi: { demo, heldout: heldoutSet() },
    programmes,
    allStalls,
    capacity: capacityView(d),
    patterns: patternsView(d),
    levers: leverView(d),
    target: target ? Number(target[1]) : null,
    chaseOneIn: chaseList > 0 && alreadyDone > 0 ? Math.round(chaseList / alreadyDone) : null,
    stalls,
    cases: STALL_PROGRAMMES.reduce((n, p) => n + (d.by_programme[p]?.cases ?? 0), 0),
  };
}

/**
 * The colorectal and cervical stalls with the scope they were counted over, for any page that quotes them.
 * `top` is the three largest; `smaller` is the named referral stalls that are not in the top three.
 * The dashboard's `dropoff` counts every programme in the run, but these labels belong to the colorectal and cervical pathway.
 */
export function stallScope(d: Dashboard): { run: string; cases: number; top: Stall[]; smaller: Stall[] } {
  const v = programmeView(d);
  const top = v.stalls;
  const group = v.allStalls.find((g) => g.pathway === "Colorectal and cervical")?.stalls ?? [];
  const smaller = group.filter((s) => (s.dropoff === "referral_unanswered" || s.dropoff === "referral_rejected") && !top.some((t) => t.dropoff === s.dropoff));
  return { run: d.meta.run, cases: v.cases, top, smaller };
}

/* ---------------- Provider links and queue levers (issue #45) ---------------- */

/**
 * One count as the API serves it. The backend suppresses small cells at source (pipeline/audit.go `supp`: 1 to 4 become
 * the string "<5"), so a suppressed cell carries only its cut-off and never a value.
 */
export type Cell = { kind: "count"; value: number } | { kind: "suppressed"; below: number };

/** A number, or a small-cell string such as "<5"; anything else is unreadable (null). */
export function readCell(v: unknown): Cell | null {
  const parsed = Count.safeParse(v);
  if (!parsed.success) return null;
  const c = parsed.data;
  if (typeof c === "number") return Number.isFinite(c) && c >= 0 ? { kind: "count", value: c } : null;
  return { kind: "suppressed", below: Number(c.slice(1)) };
}

const valueOf = (c: Cell) => (c.kind === "count" ? c.value : 0);

/**
 * The two provider patterns the case list can filter to exactly: evidenced closure (`state=closed`) and the largest stall
 * (`dropoff`). Notification timing, escalations and origin have no case-list filter, so the dashboard does not show them.
 */
export type PatternKind = "closed" | "stall";

export type Pattern = {
  kind: PatternKind;
  /** The stall's plain words; null for closure, and when no stall is recorded or every one is suppressed. */
  what: string | null;
  cell: Cell;
  /** The provider's cases, the denominator of every pattern on its line. */
  of: number;
  /** Patients filtered to the same provider and state or stall; null when the cell is suppressed or zero. */
  href: string | null;
};

export type PatternRow = {
  /** The provider id the API uses, which is also the Patients `provider` filter value and the name shown. */
  key: string;
  /** The cases (loops) the provider counts. */
  loops: Cell;
  /** Patients filtered to this provider; null when the case count is suppressed or zero. */
  href: string | null;
  /** Empty when the case count is suppressed or zero: there is no denominator to show a pattern against. */
  patterns: Pattern[];
};

export type Patterns = { providers: PatternRow[]; smallCellRule: string | null };

const words = (code: string) => {
  const s = code.replaceAll("_", " ").trim();
  return s.charAt(0).toUpperCase() + s.slice(1);
};

const stallLabel = (k: string) => STALL_LABEL[k as Dropoff] ?? OTHER_STALL_LABEL[k as Dropoff] ?? DROPOFF_WORDS[k as Exclude<Dropoff, "">]?.status ?? words(k);

/**
 * The largest readable count in a breakdown. A suppressed cell is never ranked (its value is unknown): when every recorded
 * entry is suppressed the result names none and carries the suppression; when nothing is recorded it is a zero.
 * null when the breakdown itself is missing.
 */
function largest(m: Record<string, unknown> | undefined, skip: (k: string) => boolean): { key: string | null; cell: Cell } | null {
  if (!m) return null;
  const cells = Object.entries(m).flatMap(([key, v]) => {
    const cell = skip(key) ? null : readCell(v);
    return cell ? [{ key, cell }] : [];
  });
  const counted = cells.filter((c) => valueOf(c.cell) > 0).sort((a, b) => valueOf(b.cell) - valueOf(a.cell) || a.key.localeCompare(b.key));
  if (counted.length > 0) return counted[0];
  const suppressed = cells.find((c) => c.cell.kind === "suppressed");
  return { key: null, cell: suppressed ? suppressed.cell : { kind: "count", value: 0 } };
}

const PatternRowData = z.looseObject({
  loops: z.unknown(),
  evidenced_closed: z.unknown().optional(),
  dropoff: z.record(z.string(), z.unknown()).optional(),
});

function patternRow(raw: unknown, provider: string): PatternRow | null {
  const r = PatternRowData.safeParse(raw);
  if (!r.success) return null;
  const loops = readCell(r.data.loops);
  if (!loops) return null;
  const of = valueOf(loops);
  if (of === 0) return { key: provider, loops, href: null, patterns: [] };

  // Only a positive, unsuppressed count links. The API's /api/cases filters on provider, state and dropoff.
  const link = (cell: Cell, f: Omit<CaseFilters, "provider">) => (valueOf(cell) > 0 ? filtersHref("/app/patients", { ...f, provider }) : null);
  const patterns: Pattern[] = [];

  // Evidenced closure counts decisions in state "closed" (pipeline/analyze.go dashboard), which is the list's state filter.
  const closed = readCell(r.data.evidenced_closed);
  if (closed) patterns.push({ kind: "closed", what: null, cell: closed, of, href: link(closed, { state: "closed" }) });

  // Drop-offs count only cases not closed, and a closed case has no drop-off, so provider + dropoff is the same set.
  // "" (no drop-off) and in_progress (an appointment is in the record) are not stalls.
  const stall = largest(r.data.dropoff, (k) => k === "" || k === "in_progress");
  if (stall) {
    const code = stall.key === null ? null : DropoffEnum.exclude([""]).safeParse(stall.key);
    patterns.push({
      kind: "stall",
      what: stall.key === null ? null : stallLabel(stall.key),
      cell: stall.cell,
      of,
      // An unknown code would be dropped by the Patients filter parser and show the wrong list, so it never links.
      href: code?.success ? link(stall.cell, { dropoff: code.data }) : null,
    });
  }

  return { key: provider, loops, href: filtersHref("/app/patients", { provider }), patterns };
}

/**
 * Provider rows only. Origin is an attribute of the synthetic cohort's ground truth, not of the case record, so it cannot
 * open a filtered list and the dashboard does not break it down.
 */
export function patternsView(d: Dashboard): Patterns {
  return {
    providers: d.providers.flatMap((p) => patternRow(p, p.provider) ?? []),
    smallCellRule: d.small_cell_rule ?? null,
  };
}

const num = (v: number) => v.toLocaleString("en-US");

/** "523 of 1,606", or "fewer than 5 of 70" for a suppressed cell: never the suppressed value itself. */
export function cellText(c: Cell, of: number): string {
  return c.kind === "count" ? `${num(c.value)} of ${num(of)}` : `fewer than ${c.below} of ${num(of)}`;
}

/** One pattern as its provider line prints it: "523 of 1,606 closed", "largest stall: Not booked, 195 of 1,606". */
export function patternText(p: Pattern): string {
  if (p.kind === "closed") return `${cellText(p.cell, p.of)} closed`;
  if (p.what !== null) return `largest stall: ${p.what}, ${cellText(p.cell, p.of)}`;
  if (p.cell.kind === "suppressed") return `largest stall: each one ${cellText(p.cell, p.of)}`;
  return "no stall recorded";
}

/** The provider's case count: "1,606 cases", "fewer than 5 cases". */
export function loopsText(c: Cell): string {
  if (c.kind === "suppressed") return `fewer than ${c.below} cases`;
  return `${num(c.value)} ${c.value === 1 ? "case" : "cases"}`;
}

/** An aggregate lever count out of every case the lever was computed for. */
export type LeverCount = { cell: Cell; of: number };

export type LeverView = {
  /** Cases suggested for the direct-to-colonoscopy pathway. */
  direct: LeverCount | null;
  /** Cases in the high prep-failure risk band. */
  highPrepRisk: LeverCount | null;
  requiresClinicianOrder: boolean;
};

const Levers = z.looseObject({
  pathway: z.record(z.string(), z.unknown()).optional(),
  prep_risk_band: z.record(z.string(), z.unknown()).optional(),
  requires_clinician_order: z.boolean().optional(),
});

/**
 * `key`'s count out of the whole breakdown (booking/suggest.py counts every case with a booking suggestion, so a missing
 * key is zero). null when the breakdown is missing or empty, or any part of it is unreadable or suppressed: the
 * denominator would not be exact.
 */
function leverCount(m: Record<string, unknown> | undefined, key: string): LeverCount | null {
  if (!m) return null;
  const cells = Object.values(m).map(readCell);
  if (cells.some((c) => c === null || c.kind === "suppressed")) return null;
  const of = cells.reduce((n, c) => n + valueOf(c!), 0);
  if (of === 0) return null;
  return { cell: readCell(m[key] ?? 0)!, of };
}

export function leverView(d: Dashboard): LeverView | null {
  const parsed = Levers.safeParse((d.capacity as Record<string, unknown>).levers);
  if (!parsed.success) return null;
  const direct = leverCount(parsed.data.pathway, "direct");
  const highPrepRisk = leverCount(parsed.data.prep_risk_band, "high");
  if (!direct && !highPrepRisk) return null;
  return { direct, highPrepRisk, requiresClinicianOrder: parsed.data.requires_clinician_order === true };
}

export const pct = (v: number) => `${v.toFixed(1)}%`;

/** "1 point", "1.5 points": the gap between two percents, to one decimal. */
export function points(a: number, b: number): string {
  const d = Math.round(Math.abs(a - b) * 10) / 10;
  return `${d} ${d === 1 ? "point" : "points"}`;
}

export type PoolRow = { id: string; label: string; short: string; share: number; display: string };
export type PoolRun = { slotsPerCase: string; enough: boolean; rows: PoolRow[] };

const POOL_ROWS = [
  ["balanced_30", "Balanced network", "Balanced"],
  ["skewed_no_travel", "Misallocated, no cross-site booking", "No pooling"],
  ["skewed_30", "Misallocated, 30% willing to travel", "30% travel"],
  ["skewed_pool_60", "Misallocated, network pooling (60% willing)", "Pooling"],
] as const;

/**
 * The measured pooling test (booking/sim/pool_test.json, via results.json): share of all FIT+ with a colonoscopy within 31 days.
 * Shares are percent, 0 to 100. At 1.2 slots per FIT+ there are enough slots in total; at 1.0 there are not.
 */
export function poolingRuns(): PoolRun[] {
  return [["1.2", true], ["1.0", false]].map(([slots, enough]) => ({
    slotsPerCase: slots as string,
    enough: enough as boolean,
    rows: POOL_ROWS.map(([key, label, short]) => {
      const r = result(`pool.${(slots as string).replace(".", "_")}.${key}`);
      return { id: r.id, label, short, share: pctOf(r.value), display: r.display };
    }),
  }));
}
