import { describe, expect, it } from "vitest";
import dashboardJson from "@/data/api/dashboard.json";
import { Dashboard } from "@/lib/api/schema";
import { onTimeByProgramme, points, poolingRuns, poolingVerb, programmeView, seenShare, stallScope } from "@/components/programme/model";
import { result } from "@/lib/results";

const dashboard = Dashboard.parse(dashboardJson);

describe("programmeView over the dashboard sample", () => {
  const view = programmeView(dashboard);

  it("shows the same 31-day rates as results.json's demo cohort", () => {
    expect(`${view.rate.eid.toFixed(1)}%`).toBe(result("kpi31.demo.eid_only").display);
    expect(`${view.rate.wasl.toFixed(1)}%`).toBe(result("kpi31.demo.wasl").display);
    expect(`${view.rate.truth.toFixed(1)}%`).toBe(result("kpi31.demo.truth").display);
    expect(points(view.rate.truth, view.rate.wasl)).toBe("1.2 points");
  });

  it("reads the 90% target from the KPI definition, and drops it when the definition stops stating it", () => {
    expect(view.target).toBe(90);
    const kpi = { ...dashboard.metrics.crc_kpi_31d, definition: "31-day completion" };
    expect(programmeView({ ...dashboard, metrics: { ...dashboard.metrics, crc_kpi_31d: kpi } }).target).toBeNull();
  });

  it("derives 1 in 6 from the cases Wasl closed beyond the Emirates-ID match", () => {
    expect(view.chaseOneIn).toBe(6);
  });

  it("lists the three largest stalls, largest first, never closed or in-progress cases", () => {
    expect(view.stalls).toEqual([
      { dropoff: "no_acknowledgement", label: "No acknowledgement", count: 370 },
      { dropoff: "not_booked", label: "Not booked", count: 241 },
      { dropoff: "never_notified", label: "Never notified", count: 196 },
    ]);
  });

  it("counts only the colorectal and cervical cases the stalls come from", () => {
    expect(dashboard.cases).toBeGreaterThan(view.cases);
    expect(view.cases).toBe(dashboard.by_programme.crc.cases + dashboard.by_programme.cervical.cases);
    expect(view.cases).toBe(2_100);
  });

  it("skips small-cell counts the API suppresses as <5", () => {
    const d = { ...dashboard, dropoff: { no_show: "<5", not_booked: 7 } };
    expect(programmeView(d).stalls).toEqual([{ dropoff: "not_booked", label: "Not booked", count: 7 }]);
  });
});

describe("programmeView: the two 31-day sources", () => {
  const view = programmeView(dashboard);

  it("holds the demo run and the five-seed held-out mean side by side", () => {
    expect(view.kpi.demo.rates).toEqual({ eid: 11.3, wasl: 30.7, truth: 31.9 });
    expect(view.kpi.heldout.rates).toEqual({ eid: 12.2, wasl: 29.8, truth: 32.9 });
    expect(`${view.kpi.heldout.rates.wasl.toFixed(1)}%`).toBe(result("kpi31.heldout.wasl").display);
  });

  it("explains each bar from its source: counts for the demo run, the 95% range for the mean", () => {
    expect(view.kpi.demo.detail.wasl).toBe("408 of 1,331 eligible referrals");
    expect(view.kpi.heldout.detail.eid).toBe("Average of 5 test runs, likely range 11.4% to 13.1%");
  });
});

describe("programmeView: on time by programme", () => {
  const view = programmeView(dashboard);
  const byKey = Object.fromEntries(view.programmes.map((p) => [p.key, p.onTime]));

  it("gives every programme in the sample its own on-time share, window and step", () => {
    expect(view.onTimeMeasured).toBe(true);
    expect(view.programmes.every((p) => p.onTime !== null)).toBe(true);
    expect(byKey.crc).toEqual({
      step: "colonoscopy",
      eligible: 1331,
      onTime: 408,
      share: 30.7,
      windows: [{ days: 31, eligible: 1331, onTime: 408, share: 30.7 }],
      within: "within 31 days",
    });
    expect(byKey.diabetic_retinopathy).toMatchObject({ step: "eye-clinic exam", eligible: 94, onTime: 38, share: 40.4, within: "within 42 or 91 days" });
    expect(byKey.breast).toMatchObject({ eligible: 4, onTime: 2, share: 50, within: "within 14 days" });
    expect(byKey.lung_ldct).toMatchObject({ eligible: 19, onTime: 14, share: 73.7, within: "within 30, 121 or 213 days" });
    expect(byKey.ifhas).toMatchObject({ eligible: 237, onTime: 116, share: 48.9, within: "within 10 or 59 days" });
  });

  it("sums a programme's windows and keeps each window's own share", () => {
    expect(byKey.cervical).toMatchObject({ step: "colposcopy", eligible: 454, onTime: 144, share: 31.7, within: "within 14, 28 or 56 days" });
    expect(byKey.cervical?.windows.map((w) => [w.days, w.onTime, w.eligible, w.share])).toEqual([
      [14, 4, 53, 7.5],
      [28, 42, 168, 25],
      [56, 98, 233, 42.1],
    ]);
    expect(byKey.lung_nodule).toMatchObject({ step: "follow-up CT", eligible: 354, onTime: 125, share: 35.3, within: "within 91, 183, 365 or 396 days" });
  });

  it("agrees with the colorectal 31-day figure Wasl measured", () => {
    const arm = dashboard.metrics.crc_kpi_31d.wasl_live;
    expect([byKey.crc?.onTime, byKey.crc?.eligible]).toEqual([arm.within_31d, arm.eligible_referrals]);
    expect(byKey.crc?.share).toBe(view.kpi.demo.rates.wasl);
  });

  it("recomputes the share from the counts and drops rows it cannot use", () => {
    const metrics = {
      ...dashboard.metrics,
      on_time_by_programme: [
        { programme: "breast", window_days: 21, eligible: 3, on_time: 1, share: 0.9 },
        { programme: "breast", window_days: 14, eligible: 0, on_time: 0, share: 0 },
        { programme: "prostate", window_days: 30, eligible: 5, on_time: 5, share: 1 },
      ],
    };
    const got = onTimeByProgramme({ ...dashboard, metrics });
    expect([...got.keys()]).toEqual(["breast"]);
    expect(got.get("breast")).toMatchObject({ eligible: 3, onTime: 1, share: 33.3, windows: [{ days: 21, eligible: 3, onTime: 1, share: 33.3 }] });
  });

  it("says nothing on time for a run that does not serve the metric, and parses that run", () => {
    const old = Dashboard.parse({ ...dashboardJson, metrics: { ...dashboardJson.metrics, on_time_by_programme: undefined } });
    expect(old.metrics.on_time_by_programme).toBeUndefined();
    const v = programmeView(old);
    expect(v.onTimeMeasured).toBe(false);
    expect(v.programmes.every((p) => p.onTime === null)).toBe(true);
  });

  it("rejects a served row whose counts are not counts", () => {
    const bad = { ...dashboardJson, metrics: { ...dashboardJson.metrics, on_time_by_programme: [{ programme: "crc", window_days: 31, eligible: -1, on_time: 0, share: 0 }] } };
    expect(Dashboard.safeParse(bad).success).toBe(false);
  });
});

describe("programmeView: by programme", () => {
  it("exposes all seven programmes from the actual committed dashboard fixture", () => {
    const committed = programmeView(dashboard).programmes;
    expect(committed.map((p) => p.key)).toEqual(["crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"]);
    expect(committed.find((p) => p.key === "lung_ldct")).toMatchObject({ cases: 305, closed: 280, overdue: 9, open: 16 });
    expect(committed.find((p) => p.key === "ifhas")).toMatchObject({ cases: 700, closed: 602, overdue: 73, open: 25 });
    expect(committed.reduce((sum, p) => sum + p.cases, 0)).toBe(5280);
    expect(dashboard.cases).toBe(5280);
  });

  const { programmes } = programmeView(dashboard);

  it("lists the seven programmes in order with human names", () => {
    expect(programmes.map((p) => p.name)).toEqual(["Colorectal", "Cervical", "Lung nodule", "Diabetic retinopathy", "Breast", "Lung screening (LDCT)", "IFHAS cardiometabolic"]);
    expect(programmes.map((p) => p.href)).toEqual(
      ["crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"].map((k) => `/app/patients?programme=${k}`),
    );
  });

  it("derives still open as cases minus closed minus overdue", () => {
    expect(programmes.map((p) => [p.cases, p.closed, p.overdue, p.open])).toEqual([
      [1500, 473, 869, 158],
      [600, 205, 267, 128],
      [905, 304, 160, 441],
      [1200, 1120, 56, 24],
      [70, 68, 2, 0],
      [305, 280, 9, 16],
      [700, 602, 73, 25],
    ]);
    expect(programmes.reduce((t, p) => t + p.cases, 0)).toBe(dashboard.cases);
  });

  it("drops still open when closed and overdue exceed the cases", () => {
    const d = { ...dashboard, by_programme: { ...dashboard.by_programme, breast: { cases: 10, closed: 8, overdue: 5 } } };
    expect(programmeView(d).programmes.find((p) => p.key === "breast")?.open).toBeNull();
  });
});

describe("programmeView: every stall", () => {
  const { allStalls } = programmeView(dashboard);
  const labels = allStalls.flatMap((g) => g.stalls.map((s) => s.label));

  it("groups sourced stalls by pathway, largest first, with human labels", () => {
    expect(allStalls.map((g) => g.pathway)).toEqual(["Colorectal and cervical", "Lung nodule", "Diabetic retinopathy", "Breast", "Lung cancer screening (LDCT)", "IFHAS cardiometabolic follow-up"]);
    expect(allStalls[1].stalls[0]).toEqual({ dropoff: "no_followup_ct", label: "No follow-up CT booked", count: 498 });
    expect(allStalls.flatMap((g) => g.stalls)).toHaveLength(20);
    expect(new Set(labels).size).toBe(labels.length);
    for (const g of allStalls) expect(g.stalls.map((s) => s.count)).toEqual(g.stalls.map((s) => s.count).sort((a, b) => b - a));
  });

  it("groups LDCT and IFHAS drop-offs under their own pathways", () => {
    const d = {
      ...dashboard,
      dropoff: {
        ...dashboard.dropoff,
        incomplete_scan_not_completed: 4,
        followup_ldct_not_done: 8,
        no_workup_outcome: 6,
        no_followup_documented: 9,
      },
    };
    const groups = programmeView(d).allStalls;
    expect(groups.map((g) => g.pathway)).toEqual([
      "Colorectal and cervical",
      "Lung nodule",
      "Diabetic retinopathy",
      "Breast",
      "Lung cancer screening (LDCT)",
      "IFHAS cardiometabolic follow-up",
    ]);
    expect(groups[4].stalls).toEqual([
      { dropoff: "followup_ldct_not_done", label: "Follow-up LDCT not done", count: 8 },
      { dropoff: "no_workup_outcome", label: "No work-up outcome", count: 6 },
      { dropoff: "incomplete_scan_not_completed", label: "Incomplete LDCT, repeat not done", count: 4 },
    ]);
    expect(groups[5].stalls).toEqual([{ dropoff: "no_followup_documented", label: "No follow-up visit documented", count: 9 }]);
  });

  it("skips the empty key, in_progress and suppressed counts", () => {
    const keys = allStalls.flatMap((g) => g.stalls.map((s) => s.dropoff));
    expect(keys).not.toContain("");
    expect(keys).not.toContain("in_progress");
    const d = { ...dashboard, dropoff: { ...dashboard.dropoff, no_eye_exam: "<5" } };
    expect(programmeView(d).allStalls.flatMap((g) => g.stalls.map((s) => s.dropoff))).not.toContain("no_eye_exam");
  });
});

describe("programmeView: capacity", () => {
  const { capacity } = programmeView(dashboard);

  it("reads regions, condition and summary from the sample", () => {
    expect(capacity?.condition).toBe("misallocated");
    expect(capacity?.poolingNote).toContain("sharing slots across regions");
    expect(capacity?.regions).toEqual([
      { key: "AA", name: "Al Ain", slots: 340, cases: 254 },
      { key: "AD", name: "Abu Dhabi", slots: 532, cases: 561 },
      { key: "DH", name: "Al Dhafra", slots: 96, cases: 0 },
    ]);
    expect(capacity?.summary).toEqual({ within31dShare: 0.973, medianWaitDays: 17, crossRegion: 85, nurseCalls: 102, noSlot: 0 });
  });

  it("is null when the run has no capacity block", () => {
    expect(programmeView({ ...dashboard, capacity: { status: "none", data_mode: "simulated" } }).capacity).toBeNull();
  });
});

describe("poolingRuns", () => {
  it("reads the deterministic pooling test from results.json: 21.5% without pooling, 41.7% with, at 1.2 slots per FIT+", () => {
    const [enough, short] = poolingRuns();
    expect(enough.slotsPerCase).toBe("1.2");
    expect(enough.rows.map((r) => r.display)).toEqual(["40.8%", "21.5%", "36.9%", "41.7%"]);
    expect(short.rows.map((r) => r.display)).toEqual(["7.3%", "18.5%", "7.1%", "8.2%"]);
    expect(result("pool.1_2.skewed_pool_60").source).toBe("booking/sim/pool_test.json");
    expect(enough.rows.map((r) => r.label)).toEqual(["Right regions", "Wrong regions, no sharing", "Wrong regions, 30% travel", "Wrong regions, sharing"]);
  });

  it("words the sharing effect from the measured shares: nearly doubles with enough slots, no gain when short", () => {
    const [enough, short] = poolingRuns();
    const share = (run: typeof enough, key: string) => run.rows.find((r) => r.id.endsWith(key))!.share;
    expect(poolingVerb(share(enough, "skewed_no_travel"), share(enough, "skewed_pool_60"))).toBe("nearly doubles");
    expect(poolingVerb(share(short, "skewed_no_travel"), share(short, "skewed_pool_60"))).toBe("does not raise");
    expect(poolingVerb(10, 25)).toBe("more than doubles");
    expect(poolingVerb(10, 15)).toBe("raises");
  });
});

describe("seenShare", () => {
  it("is each method's 31-day rate over the truth's, per source tab: 96% and 35% in the demo run, 91% and 37% over 5 runs", () => {
    const { demo, heldout } = programmeView(dashboard).kpi;
    expect([seenShare(demo.rates.wasl, demo.rates.truth), seenShare(demo.rates.eid, demo.rates.truth)]).toEqual([96, 35]);
    expect([seenShare(heldout.rates.wasl, heldout.rates.truth), seenShare(heldout.rates.eid, heldout.rates.truth)]).toEqual([91, 37]);
    expect(seenShare(5, 0)).toBe(0);
  });
});

describe("stallScope", () => {
  it("names the run and the colorectal and cervical cases, with the 196 never-notified stall among the largest", () => {
    const s = stallScope(dashboard);
    expect(s.run).toBe(dashboard.meta.run);
    expect(s.cases).toBe(2100);
    expect(s.top.map((x) => [x.label, x.count])).toEqual([["No acknowledgement", 370], ["Not booked", 241], ["Never notified", 196]]);
    expect(s.smaller.map((x) => [x.label, x.count])).toEqual([["Referral unanswered", 46], ["Referral rejected", 10]]);
  });
});
