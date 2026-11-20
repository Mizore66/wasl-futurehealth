import { describe, expect, it } from "vitest";
import dashboardJson from "@/data/api/dashboard.json";
import { Dashboard } from "@/lib/api/schema";
import { regionGap, regionTakeaway } from "@/components/programme/capacity-chart";
import { byOverdueShare, closedShare, patternsView, programmeView, STATUS_PART } from "@/components/programme/model";

/* The chart-first Programme redesign: the figures the charts print beyond the source cells. */

const dashboard = Dashboard.parse(dashboardJson);
const view = programmeView(dashboard);

describe("programme closed vs provider closed", () => {
  it("by_programme closed is every finished case: evidenced closures plus the no-follow-up outcomes", () => {
    const programmeClosed = Object.values(dashboard.by_programme).reduce((t, p) => t + p.closed, 0);
    expect(programmeClosed).toBe(dashboard.closed);
    const s = dashboard.states as Record<string, number>;
    expect(programmeClosed).toBe(s.closed + s.no_followup_needed + s.no_referral_needed + s.no_recall_needed);
    expect(STATUS_PART.closed).toBe("Finished");
  });

  it("provider evidenced_closed counts state closed only, so it never exceeds the states.closed total", () => {
    const evidenced = (dashboardJson.providers as { evidenced_closed: number | string }[])
      .map((p) => (typeof p.evidenced_closed === "number" ? p.evidenced_closed : 0))
      .reduce((t, v) => t + v, 0);
    expect(evidenced).toBeLessThanOrEqual((dashboard.states as Record<string, number>).closed);
    // The eye-screening provider: 59 closed with evidence, while diabetic retinopathy has 1,120 finished.
    const retina = patternsView(dashboard).providers.find((r) => r.key === "phc-retina")!;
    expect(closedShare(retina.patterns.find((p) => p.kind === "closed"))).toBe(5);
    expect(dashboard.by_programme.diabetic_retinopathy.closed).toBe(1120);
  });
});

describe("chart helpers", () => {
  it("sorts programmes by overdue share, largest first", () => {
    expect(byOverdueShare(view.programmes).map((p) => p.key)).toEqual(["crc", "cervical", "lung_nodule", "ifhas", "diabetic_retinopathy", "lung_ldct", "breast"]);
  });

  it("prints each region's slots minus open cases and a takeaway from the same numbers", () => {
    const regions = view.capacity!.regions;
    expect(regions.map(regionGap)).toEqual(["86 spare", "29 short", "96 spare"]);
    expect(regionTakeaway(regions)).toBe("Abu Dhabi is 29 slots short while Al Ain and Al Dhafra have spare.");
    expect(regionTakeaway([{ key: "a", name: "A", slots: 5, cases: 5 }])).toBe("Every region has a slot for each open case.");
  });

  it("gives no evidenced-closure share for a suppressed cell", () => {
    const bsu = patternsView(dashboard).providers.find((r) => r.key === "bsu-screen")!;
    expect(closedShare(bsu.patterns.find((p) => p.kind === "closed"))).toBeNull();
  });
});
