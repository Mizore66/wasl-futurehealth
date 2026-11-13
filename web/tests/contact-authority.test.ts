import { readFileSync } from "node:fs";
import path from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { TodayBoard } from "@/components/worklist/today-board";
import { workItem } from "@/components/worklist/model";
import { CaseList, CaseSummary } from "@/lib/api/schema";

const load = (name: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", name), "utf8"));
const base = CaseList.parse(load("ledger_case_list.json")).cases[0];
const NOW = Date.parse("2026-11-20T12:00:00Z");

// Captured synthetic Go HTTP summaries before and after replay, not reconstructed browser consent history.
export const regranted = (phase: string) => CaseSummary.parse({
  ...base, ...load(`contact_staff_regrant_${phase}.json`), review: base.review,
  owner: "someone", dropoff: "not_booked",
});

describe("authoritative shared contact projection", () => {
  for (const phase of ["live", "restarted"]) {
    it(`${phase}: staff grant supersedes historical patient denial`, () => {
      const summary = regranted(phase);
      expect(summary.api_patient?.contact_permission.status).toBe("denied");
      const item = workItem(summary, 40, true, NOW);
      expect([item.contactBlocked, item.textsAllowed, item.textsStopped]).toEqual([false, true, false]);
      const html = renderToStaticMarkup(createElement(TodayBoard, { items: [item], now: 40, live: true, aside: null }));
      expect(html).toContain("Allowed");
      expect(html).toContain("After the call");
      expect(html).not.toContain("Stopped by patient");
      expect(html).not.toMatch(/<button[^>]* disabled=""[^>]*>(?:(?!<\/button>).)*Log call/);
    });
  }

  it("legacy snapshots without shared projection conservatively retain patient denial and terminal opt-out", () => {
    for (const status of ["denied", "opted_out"] as const) {
      const summary = regranted("live");
      const item = workItem({ ...summary, contact_state: undefined, api_patient: {
        ...summary.api_patient!, contact_permission: { status },
      } }, 40, true, NOW);
      expect([item.contactBlocked, item.textsAllowed, item.textsStopped]).toEqual([true, false, true]);
    }
  });

  it("shared opt-out remains blocked despite a historical patient grant; canonical consent is not invented", () => {
    const summary = regranted("live");
    const optedOut = workItem({ ...summary, contact_state: { status: "opted_out", contact_blocked: true },
      api_patient: { ...summary.api_patient!, contact_permission: { status: "granted" } },
    }, 40, true, NOW);
    expect([optedOut.contactBlocked, optedOut.textsAllowed]).toEqual([true, false]);
    expect(workItem({ ...summary, consent: "unknown" }, 40, true, NOW).textsAllowed).toBe(false);
  });
});
