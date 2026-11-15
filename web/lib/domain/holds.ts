import { Hold, type CaseDetail } from "@/lib/api/schema";

type CaseHold = CaseDetail["holds"][number];
type CaseHolds = { current: CaseHold[]; earlier: CaseHold[] };

/**
 * `holds` accumulates hold_decision blocks from every decision (pipeline/api_export.go caseDetailFor). Only the latest
 * canonical decision's chain says what holds the case now; the rest are resolved or replaced and stay as history.
 */
export function caseHolds(d: Pick<CaseDetail, "holds" | "current">): CaseHolds {
  const current = d.current.chain.filter((e) => e.type === "hold_decision").map((e) => Hold.parse(e.body));
  const now = new Set(current.map(holdKey));
  return { current, earlier: d.holds.filter((h) => !now.has(holdKey(h))) };
}

const holdKey = (h: CaseHold) => JSON.stringify(h, Object.keys(h).sort());
