import { describe, expect, it } from "vitest";
import dashboardJson from "@/data/api/dashboard.json";
import { Dashboard } from "@/lib/api/schema";
import { points, programmeView } from "@/components/programme/model";
import { result } from "@/lib/results";

const dashboard = Dashboard.parse(dashboardJson);

describe("programmeView over the dashboard sample", () => {
  const view = programmeView(dashboard);

  it("shows the same 31-day rates as results.json's demo cohort", () => {
    expect(`${view.rate.eid.toFixed(1)}%`).toBe(result("kpi31.demo.eid_only").display);
    expect(`${view.rate.wasl.toFixed(1)}%`).toBe(result("kpi31.demo.wasl").display);
    expect(`${view.rate.truth.toFixed(1)}%`).toBe(result("kpi31.demo.truth").display);
    expect(points(view.rate.truth, view.rate.wasl)).toBe("1 point");
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
      { dropoff: "no_acknowledgement", label: "No acknowledgement", count: 390 },
      { dropoff: "not_booked", label: "Not booked", count: 252 },
      { dropoff: "never_notified", label: "Never notified", count: 183 },
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
