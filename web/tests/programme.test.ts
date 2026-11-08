import { describe, expect, it } from "vitest";
import dashboardJson from "@/data/api/dashboard.json";
import { Dashboard } from "@/lib/api/schema";
import { points, poolingRuns, programmeView, stallScope } from "@/components/programme/model";
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
    expect(view.kpi.heldout.detail.eid).toBe("Mean of 5 seeds, 95% range 11.4% to 13.1%");
  });
});

describe("programmeView: by programme", () => {
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
    expect(capacity?.poolingNote).toContain("network pooling");
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
  it("reads the measured pooling test from results.json: 22.2% without pooling, 42.1% with, at 1.2 slots per FIT+", () => {
    const [enough, short] = poolingRuns();
    expect(enough.slotsPerCase).toBe("1.2");
    expect(enough.rows.map((r) => r.display)).toEqual(["40.4%", "22.2%", "37.2%", "42.1%"]);
    expect(short.rows.map((r) => r.display)).toEqual(["6.9%", "18.6%", "7.3%", "7.8%"]);
    expect(result("pool.1_2.skewed_pool_60").source).toBe("booking/sim/pool_test.json");
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
