import { describe, expect, it } from "vitest";
import dashboardJson from "@/data/api/dashboard.json";
import { Dashboard } from "@/lib/api/schema";
import { cellText, leverView, loopsText, patternsView, patternText, programmeView, readCell, type Pattern, type PatternRow } from "@/components/programme/model";
import { parseCaseFilters } from "@/components/worklist/filters";

/* Issue #45: provider/origin patterns and queue-lever counts on the Programme dashboard. */

const dashboard = Dashboard.parse(dashboardJson);
const raw = dashboardJson as unknown as {
  providers: Record<string, unknown>[];
  origins: Record<string, unknown>[];
  capacity: { levers: { pathway: Record<string, number>; prep_risk_band: Record<string, number> } };
};

/** The largest plain-number entry, computed straight from the fixture JSON, independently of the model. */
function rawLargest(m: Record<string, unknown>, skip: string[] = []): [string, number] | null {
  const counted = Object.entries(m).filter((e): e is [string, number] => typeof e[1] === "number" && e[1] > 0 && !skip.includes(e[0]));
  counted.sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  return counted[0] ?? null;
}

const pattern = (row: PatternRow, kind: Pattern["kind"]) => row.patterns.find((p) => p.kind === kind);
const query = (href: string) => Object.fromEntries(new URL(href, "http://x").searchParams);

describe("patternsView over the dashboard fixture", () => {
  const view = programmeView(dashboard).patterns;

  it("projects every provider and origin row the API serves, in its order", () => {
    expect(view.providers.map((r) => r.key)).toEqual(raw.providers.map((r) => r.provider));
    expect(view.origins.map((r) => r.key)).toEqual(raw.origins.map((r) => r.origin));
    expect(view.providers.map((r) => r.key)).toEqual(["ahs-mushrif", "bsu-screen", "ifh-screen", "lcs-centre", "medlab", "phc-retina", "rad-imaging"]);
    expect(view.origins.map((r) => r.name)).toEqual(["Arab", "Filipino", "South Asian", "Western"]);
    expect(view.smallCellRule).toBe("counts 1-4 shown as <5");
  });

  it("shows every row's values exactly as the fixture states them, out of the row's cases", () => {
    for (const [group, rows, key] of [
      ["providers", view.providers, "provider"],
      ["origins", view.origins, "origin"],
    ] as const) {
      for (const row of rows) {
        const src = raw[group].find((r) => r[key] === row.key)!;
        expect(row.loops, `${row.key} loops`).toEqual(readCell(src.loops));
        const of = src.loops as number;
        expect(typeof of).toBe("number");
        for (const p of row.patterns) expect(p.of, `${row.key} ${p.kind} denominator`).toBe(of);

        expect(pattern(row, "closed")?.cell).toEqual(readCell(src.evidenced_closed));
        expect(pattern(row, "notified")?.cell).toEqual(readCell(src.notified_within_21d));

        const stall = rawLargest(src.dropoff as Record<string, unknown>, ["", "in_progress"]);
        const shownStall = pattern(row, "stall")!;
        if (stall) expect(shownStall.cell, `${row.key} largest stall`).toEqual({ kind: "count", value: stall[1] });
        else expect(shownStall.what).toBeNull();

        const esc = rawLargest(src.escalations as Record<string, unknown>);
        const shownEsc = pattern(row, "escalation")!;
        if (esc) expect(shownEsc.cell, `${row.key} top escalation`).toEqual({ kind: "count", value: esc[1] });
        else expect(shownEsc.what).toBeNull();
      }
    }
  });

  it("prints the Al Mushrif row from its fixture numbers", () => {
    const row = view.providers.find((r) => r.key === "ahs-mushrif")!;
    expect(row.name).toBe("Al Mushrif");
    expect(loopsText(row.loops)).toBe("1,606 cases");
    expect(row.patterns.map(patternText)).toEqual([
      "Closed with evidence: 523 of 1,606",
      "Notified within 21 days: 1,404 of 1,606",
      "Largest stall: No acknowledgement, 293 of 1,606",
      "Most frequent escalation: Past the colorectal deadline, 591 of 1,606",
    ]);
  });

  it("links provider rows, closure and the largest stall to exactly the matching Patients filters", () => {
    const row = view.providers.find((r) => r.key === "rad-imaging")!;
    expect(row.href).toBe("/app/patients?provider=rad-imaging");
    expect(pattern(row, "closed")!.href).toBe("/app/patients?state=closed&provider=rad-imaging");
    expect(pattern(row, "stall")!.href).toBe("/app/patients?dropoff=no_followup_ct&provider=rad-imaging");
    expect(pattern(row, "stall")!.cell).toEqual({ kind: "count", value: 498 });
    for (const r of view.providers) {
      for (const href of [r.href, ...r.patterns.map((p) => p.href)].filter((h): h is string => h !== null)) {
        // Every parameter survives the Patients parser, so the list is filtered to the same set the count describes.
        expect(parseCaseFilters(query(href)), href).toEqual(query(href));
        expect(query(href).provider).toBe(r.key);
      }
      expect(pattern(r, "notified")!.href, "notification has no case-list filter").toBeNull();
      expect(pattern(r, "escalation")!.href, "escalations are not on case summaries").toBeNull();
    }
  });

  it("never links an origin: case records carry no origin", () => {
    for (const r of view.origins) {
      expect(r.href).toBeNull();
      expect(r.patterns.every((p) => p.href === null)).toBe(true);
    }
  });
});

describe("patternsView: small cells", () => {
  const view = patternsView(dashboard);
  const breast = view.providers.find((r) => r.key === "bsu-screen")!;

  it("shows a suppressed cell as fewer than the cut-off, without a value or a link", () => {
    const closed = pattern(breast, "closed")!;
    expect(closed.cell).toEqual({ kind: "suppressed", below: 5 });
    expect(closed.href).toBeNull();
    expect(patternText(closed)).toBe("Closed with evidence: fewer than 5 of 70");
  });

  it("names no largest stall or escalation when every recorded one is suppressed", () => {
    const stall = pattern(breast, "stall")!;
    expect(stall.what).toBeNull();
    expect(stall.href).toBeNull();
    expect(patternText(stall)).toBe("Largest stall: each one fewer than 5 of 70");
    expect(patternText(pattern(breast, "escalation")!)).toBe("Most frequent escalation: each one fewer than 5 of 70");
  });

  it("never ranks a suppressed cell above a counted one, and never prints a suppressed value anywhere", () => {
    const d = { ...dashboard, providers: [{ provider: "p", loops: 40, evidenced_closed: "<5", notified_within_21d: 9, dropoff: { no_show: "<5", not_booked: 6 }, escalations: { unowned: "<5" } }] };
    const [row] = patternsView(d).providers;
    expect(pattern(row, "stall")).toMatchObject({ what: "Not booked", cell: { kind: "count", value: 6 }, href: "/app/patients?dropoff=not_booked&provider=p" });
    const text = row.patterns.map(patternText).join(" | ");
    expect(text).not.toMatch(/<5/);
    expect(text.match(/\d+/g)).toEqual(["5", "40", "21", "9", "40", "6", "40", "5", "40"]);
    expect(row.patterns.filter((p) => p.cell.kind === "suppressed").every((p) => p.href === null)).toBe(true);
  });

  it("shows a row whose case count is suppressed with no patterns and no link", () => {
    const d = { ...dashboard, providers: [{ provider: "tiny", loops: "<5", evidenced_closed: "<5", dropoff: { no_show: "<5" } }] };
    const [row] = patternsView(d).providers;
    expect(row).toEqual({ key: "tiny", name: "tiny", loops: { kind: "suppressed", below: 5 }, href: null, patterns: [] });
    expect(loopsText(row.loops)).toBe("fewer than 5 cases");
  });
});

describe("patternsView: zero counts and missing data", () => {
  it("prints zero counts out of the row and never links them", () => {
    const d = { ...dashboard, providers: [{ provider: "z", loops: 12, evidenced_closed: 0, notified_within_21d: 0, dropoff: { "": 12 }, escalations: {} }] };
    const [row] = patternsView(d).providers;
    expect(row.patterns.map(patternText)).toEqual(["Closed with evidence: 0 of 12", "Notified within 21 days: 0 of 12", "Largest stall: none recorded", "Most frequent escalation: none recorded"]);
    expect(row.patterns.every((p) => p.href === null)).toBe(true);
    expect(row.href).toBe("/app/patients?provider=z");
  });

  it("reads a missing or null providers or origins array as empty", () => {
    const { providers, origins, ...rest } = dashboardJson as Record<string, unknown>;
    expect(providers).toBeDefined();
    expect(origins).toBeDefined();
    const missing = Dashboard.parse(rest);
    expect(patternsView(missing)).toEqual({ providers: [], origins: [], smallCellRule: "counts 1-4 shown as <5" });
    const nulls = Dashboard.parse({ ...rest, providers: null, origins: null });
    expect(patternsView(nulls).providers).toEqual([]);
    expect(patternsView(nulls).origins).toEqual([]);
  });

  it("omits a pattern whose field is missing and skips a row it cannot read", () => {
    const d = { ...dashboard, providers: [{ provider: "a", loops: 30 }, { provider: "b", loops: "lots" }], origins: [] };
    const v = patternsView(d);
    expect(v.providers).toHaveLength(1);
    expect(v.providers[0].patterns).toEqual([]);
  });

  it("never links an unknown drop-off code, which the Patients filter would drop", () => {
    const d = { ...dashboard, providers: [{ provider: "q", loops: 30, dropoff: { some_new_stall: 12 } }] };
    const stall = pattern(patternsView(d).providers[0], "stall")!;
    expect(stall).toMatchObject({ what: "Some new stall", cell: { kind: "count", value: 12 }, href: null });
  });
});

describe("leverView", () => {
  it("reads the direct-pathway and high prep-risk counts out of every case they were computed for", () => {
    const v = leverView(dashboard)!;
    const { pathway, prep_risk_band } = raw.capacity.levers;
    expect(v.direct).toEqual({ cell: { kind: "count", value: pathway.direct }, of: pathway.direct + pathway.clinic });
    expect(v.highPrepRisk).toEqual({ cell: { kind: "count", value: prep_risk_band.high }, of: prep_risk_band.high + prep_risk_band.low });
    expect(cellText(v.direct!.cell, v.direct!.of)).toBe("683 of 815");
    expect(cellText(v.highPrepRisk!.cell, v.highPrepRisk!.of)).toBe("238 of 815");
    expect(v.requiresClinicianOrder).toBe(true);
    expect(programmeView(dashboard).levers).toEqual(v);
  });

  it("is null without levers, and drops a lever whose denominator is not exact", () => {
    const capacity = dashboard.capacity;
    expect(leverView({ ...dashboard, capacity: { ...capacity, levers: undefined } })).toBeNull();
    expect(leverView({ ...dashboard, capacity: { status: "not_generated", data_mode: "simulated" } })).toBeNull();
    const suppressed = leverView({ ...dashboard, capacity: { ...capacity, levers: { pathway: { direct: 40, clinic: "<5" }, prep_risk_band: { low: 30, high: 14 } } } })!;
    expect(suppressed.direct).toBeNull();
    expect(suppressed.highPrepRisk).toEqual({ cell: { kind: "count", value: 14 }, of: 44 });
  });

  it("counts a missing bucket as zero", () => {
    const capacity = dashboard.capacity;
    const v = leverView({ ...dashboard, capacity: { ...capacity, levers: { pathway: { clinic: 20 } } } })!;
    expect(v.direct).toEqual({ cell: { kind: "count", value: 0 }, of: 20 });
    expect(v.highPrepRisk).toBeNull();
  });
});
