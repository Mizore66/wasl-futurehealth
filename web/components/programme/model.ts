import { z } from "zod";
import type { Dashboard, Dropoff, Programme } from "@/lib/api/schema";
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
