import type { Dashboard, Dropoff } from "@/lib/api/schema";

export type Stall = { dropoff: Dropoff; label: string; count: number };

export type ProgrammeView = {
  /** Percent, 0 to 100. */
  rate: { eid: number; wasl: number; truth: number };
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

/** The stall drop-offs are the colorectal and cervical pathway's; the other programmes have their own. */
const STALL_PROGRAMMES = ["crc", "cervical"] as const;

const pctOf = (x: number) => Math.round(x * 1000) / 10;

export function programmeView(d: Dashboard): ProgrammeView {
  const kpi = d.metrics.crc_kpi_31d;
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

  return {
    rate: { eid: pctOf(kpi.eid_only.kpi_all_referrals), wasl: pctOf(kpi.wasl_live.kpi_all_referrals), truth: pctOf(kpi.truth.kpi_all_referrals) },
    target: target ? Number(target[1]) : null,
    chaseOneIn: chaseList > 0 && alreadyDone > 0 ? Math.round(chaseList / alreadyDone) : null,
    stalls,
    cases: STALL_PROGRAMMES.reduce((n, p) => n + (d.by_programme[p]?.cases ?? 0), 0),
  };
}

export const pct = (v: number) => `${v.toFixed(1)}%`;

/** "1 point", "1.5 points": the gap between two percents, to one decimal. */
export function points(a: number, b: number): string {
  const d = Math.round(Math.abs(a - b) * 10) / 10;
  return `${d} ${d === 1 ? "point" : "points"}`;
}
