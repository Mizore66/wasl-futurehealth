import { readFileSync } from "node:fs";
import path from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { TodayBoard } from "@/components/worklist/today-board";
import { workItem } from "@/components/worklist/model";
import { CaseList, CaseSummary } from "@/lib/api/schema";

const fullList = CaseList.parse(JSON.parse(readFileSync(path.join(__dirname, "fixtures", "ledger_case_list.json"), "utf8")));
const NOW = Date.parse("2026-11-20T12:00:00Z");
const granted: CaseSummary = { ...fullList.cases[0], consent: "granted", owner: "someone", dropoff: "not_booked", api_overlay: undefined };
const withContact = (status: "none" | "granted" | "denied" | "opted_out", contact_blocked: boolean): CaseSummary => ({
  ...granted,
  contact_state: { status, contact_blocked },
});
const board = (c: CaseSummary) => renderToStaticMarkup(createElement(TodayBoard, { items: [workItem(c, 40, true, NOW)], now: 40, aside: null, live: true }));

describe("Today follows the effective contact state", () => {
  it("canonical-granted consent with a recorded denial or opt-out is not texts-allowed", () => {
    for (const status of ["denied", "opted_out"] as const) {
      const it = workItem(withContact(status, true), 40, true, NOW);
      expect([it.textsAllowed, it.contactBlocked]).toEqual([false, true]);
    }
  });

  it("canonical-granted consent with no blocking overlay keeps texts allowed, including after a grant", () => {
    for (const c of [granted, withContact("none", false), withContact("granted", false)]) {
      const it = workItem(c, 40, true, NOW);
      expect([it.textsAllowed, it.contactBlocked]).toEqual([true, false]);
    }
  });

  it("a blocked patient is not shown as Allowed and has no contact-outcome controls", () => {
    const html = board(withContact("opted_out", true));
    expect(html).not.toContain("Allowed");
    expect(html).toMatch(/<button[^>]* disabled=""[^>]*>(?:(?!<\/button>).)*Log call/);
    expect(html).not.toContain("After the call");
    expect(html).toContain("Contact blocked");
  });

  it("an unblocked patient keeps Allowed and the Log call control", () => {
    const html = board(withContact("none", false));
    expect(html).toContain("Allowed");
    expect(html).not.toMatch(/<button[^>]* disabled=""[^>]*>(?:(?!<\/button>).)*Log call/);
    expect(html).toContain("After the call");
    expect(html).not.toContain("Contact blocked");
  });
});
