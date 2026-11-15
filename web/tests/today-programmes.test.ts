import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { byUrgency, clockWords, overdueClockDays } from "@/components/worklist/days";
import { parseTodaySelection, TODAY_DEFAULT } from "@/components/worklist/filters";
import { workItem } from "@/components/worklist/model";
import { CaseList, CaseSummary, type Programme } from "@/lib/api/schema";

const page = (p: Programme) => CaseList.parse(JSON.parse(readFileSync(path.join(__dirname, "..", "data", "api", `cases_${p}_page1.json`), "utf8"))).cases;
const open = (p: Programme) => page(p).find((c) => !c.closed)!;
const annual = CaseSummary.parse({ ...page("lung_ldct").find((c) => c.ldct?.rec?.action === "annual")!, closed: false, state: "open", overdue: false });
const at = (c: CaseSummary, days: number) => workItem(c, c.referral_day + days + 0.5, true);

describe("Today programme and provider selection", () => {
  it("defaults to the CRC queue at Al Mushrif only when nothing is chosen", () => {
    expect(parseTodaySelection({})).toEqual(TODAY_DEFAULT);
    expect(TODAY_DEFAULT).toEqual({ programme: "crc", provider: "ahs-mushrif" });
  });

  it("keeps an explicit choice, and an explicit empty choice means every programme or provider", () => {
    expect(parseTodaySelection({ programme: "breast", provider: "" })).toEqual({ programme: "breast" });
    expect(parseTodaySelection({ programme: "", provider: "lcs-centre" })).toEqual({ provider: "lcs-centre" });
    expect(parseTodaySelection({ programme: "", provider: "" })).toEqual({});
  });
});

describe("Today clocks follow each programme's rule", () => {
  it("measures each case against its own deadline, not 31 days", () => {
    expect(overdueClockDays(open("crc"))).toBe(31);
    expect(overdueClockDays(open("cervical"))).toBe(14);
    expect(overdueClockDays(open("ifhas"))).toBe(59);
    expect(at(open("breast"), 20)).toMatchObject({ days: 20, deadline: 14 });
  });

  it("gives an annual LDCT case no overdue clock, however long it waits", () => {
    expect(overdueClockDays(annual)).toBeNull();
    const row = at(annual, 400);
    expect(row.deadline).toBeNull();
    expect(clockWords(row)).toEqual({ over: false, short: "routine interval", long: "Routine interval · no overdue clock" });
  });

  it("words the countdown against the case's deadline", () => {
    expect(clockWords({ days: 108, deadline: 31 })).toEqual({ over: true, short: "77 over", long: "77 days over 31" });
    expect(clockWords({ days: 10, deadline: 14 })).toEqual({ over: false, short: "4 left", long: "4 days left of 14" });
  });

  it("orders overdue first, then fewest days left, and routine intervals last", () => {
    const rows = [
      { ...at(open("crc"), 25), id: "crc-6-left" },
      { ...at(open("breast"), 12), id: "breast-2-left" },
      { ...at(annual, 900), id: "annual" },
      { ...at(open("ifhas"), 60), id: "ifhas-1-over" },
      { ...at(open("crc"), 40), id: "crc-9-over" },
    ];
    expect(rows.sort(byUrgency).map((r) => r.id)).toEqual(["crc-9-over", "ifhas-1-over", "breast-2-left", "crc-6-left", "annual"]);
  });
});
