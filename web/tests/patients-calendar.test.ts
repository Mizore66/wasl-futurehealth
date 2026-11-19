import { describe, expect, it } from "vitest";
import {
  byDate,
  calendarHref,
  chooseMonth,
  dateLabel,
  monthCells,
  monthLabel,
  monthsWith,
  parseCalendarView,
  referralDate,
  searchMatches,
} from "@/components/patients/calendar";

describe("Referral calendar state", () => {
  it("reads month, files scope and open case from the URL and drops malformed values", () => {
    expect(parseCalendarView({ month: "2026-05", files: "2026-05-02", open: "ahs/ServiceRequest/AHS-sr-1" })).toEqual({
      month: "2026-05",
      files: "2026-05-02",
      open: "ahs/ServiceRequest/AHS-sr-1",
    });
    expect(parseCalendarView({ files: "2026-05" })).toEqual({ files: "2026-05" });
    expect(parseCalendarView({ month: "2026-13", files: "May", open: " " })).toEqual({});
  });

  it("writes the filters first, then the calendar state, so old filter links keep their shape", () => {
    expect(calendarHref({ programme: "crc", q: "1011" }, { files: "2026-05-27", open: "a/b/c" })).toBe(
      "/app/patients?programme=crc&q=1011&files=2026-05-27&open=a%2Fb%2Fc",
    );
    expect(calendarHref({}, {})).toBe("/app/patients");
    expect(calendarHref({ closed: true }, { files: "2026-05", page: 2 })).toBe("/app/patients?closed=true&files=2026-05&page=2");
    expect(calendarHref({}, { files: "2026-05", page: 1 })).toBe("/app/patients?files=2026-05");
  });

  it("dates a referral by the UTC day it was received, as formatDay does", () => {
    // Day 0 is 1 May 2026; 26.28 is the morning of 27 May.
    expect(referralDate({ referral_day: 0 })).toBe("2026-05-01");
    expect(referralDate({ referral_day: 26.2784 })).toBe("2026-05-27");
    expect(referralDate({ referral_day: -63.02 })).toBe("2026-02-26");
    expect(monthLabel("2026-05")).toBe("May 2026");
    expect(dateLabel("2026-05-02")).toBe("2 May 2026");
  });

  it("lays a month out Monday first with padding", () => {
    const may = monthCells("2026-05");
    // 1 May 2026 is a Friday: four pads, then 31 days, padded to whole weeks.
    expect(may.slice(0, 5)).toEqual([null, null, null, null, "2026-05-01"]);
    expect(may.length % 7).toBe(0);
    expect(may.filter(Boolean)).toHaveLength(31);
    expect(monthCells("2027-02").filter(Boolean)).toHaveLength(28);
  });

  it("opens on the asked month when it holds referrals, else the scope's month, else the latest", () => {
    const months = monthsWith(["2026-07-01", "2026-05-02", "2026-05-27"]);
    expect(months).toEqual(["2026-05", "2026-07"]);
    expect(chooseMonth(months, { month: "2026-05" })).toBe("2026-05");
    expect(chooseMonth(months, { month: "2026-06" })).toBe("2026-07");
    expect(chooseMonth(months, { files: "2026-05-02" })).toBe("2026-05");
    expect(chooseMonth([], {})).toBeUndefined();
  });

  it("groups thousands of referrals by date without losing any", () => {
    const items = Array.from({ length: 5000 }, (_, i) => ({ id: i, date: `2026-05-${String((i % 31) + 1).padStart(2, "0")}` }));
    const groups = byDate(items, (t) => t.date);
    expect(groups.size).toBe(31);
    expect([...groups.values()].reduce((n, g) => n + g.length, 0)).toBe(5000);
    expect(groups.get("2026-05-01")![0].id).toBe(0);
  });

  it("finds a case by id, task or owner, every word, ignoring case", () => {
    const fields = ["ahs-mushrif/ServiceRequest/AHS-sr-1011", "AHS-sr-1011", "Report incomplete", "Doctors to decide", "Nurse 3", "Aisha Al Mansoori"];
    expect(searchMatches(undefined, fields)).toBe(true);
    expect(searchMatches("1011", fields)).toBe(true);
    expect(searchMatches("nurse 3", fields)).toBe(true);
    expect(searchMatches("incomplete aisha", fields)).toBe(true);
    expect(searchMatches("nurse 4", fields)).toBe(false);
  });
});
