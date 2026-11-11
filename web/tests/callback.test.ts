import { afterEach, describe, expect, it, vi } from "vitest";
import { callbackChoice, fromNow, WEEKDAY } from "@/components/worklist/callback";
import { dayToDate } from "@/lib/domain/day";
import { postAction } from "@/lib/api/source";

/*
 * Issue #37: the "Call back on…" label and the saved snooze must come from one `until`. The run clock (the simulation's
 * latest decision day) and the wall clock are on different dates here, so a label drawn from the run clock would differ.
 */
const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
const RUN_DAY = 120; // a run day far from the wall date below
const ms = (iso: string) => Date.parse(iso);

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("callbackChoice", () => {
  it("formats the label from the exact `until` it saves, not from the run clock", () => {
    const wall = ms("2026-11-10T09:15:42.678Z");
    for (const k of [1, 2, 3]) {
      const { label, action } = callbackChoice(k, wall);
      expect(action.until).toBe(new Date(wall + k * 86_400_000).toISOString().replace(/\.\d{3}Z$/, "Z"));
      expect(label).toBe(WEEKDAY.format(new Date(action.until)));
      expect(action.reason).toBe(`Call back ${label}`);
      expect(label).not.toBe(WEEKDAY.format(dayToDate(RUN_DAY + k)));
    }
    expect(callbackChoice(1, wall).label).toBe("Wed 11 Nov");
    expect(callbackChoice(3, wall).label).toBe("Fri 13 Nov");
  });

  it("names the UTC date of `until` at a UTC day boundary", () => {
    // One second before midnight UTC: the saved instant is still on the 11th in UTC.
    const late = callbackChoice(1, ms("2026-11-10T23:59:59.999Z"));
    expect([late.action.until, late.label]).toEqual(["2026-11-11T23:59:59Z", "Wed 11 Nov"]);
    // Midnight UTC exactly, across a month and year boundary.
    const midnight = callbackChoice(1, ms("2026-12-31T00:00:00.000Z"));
    expect([midnight.action.until, midnight.label]).toEqual(["2027-01-01T00:00:00Z", "Fri 1 Jan"]);
    // 22:30 UTC is already the next day in the Gulf (UTC+4); the label follows the UTC instant the API stores.
    const evening = callbackChoice(1, ms("2026-11-10T22:30:00Z"));
    expect([evening.action.until, evening.label]).toEqual(["2026-11-11T22:30:00Z", "Wed 11 Nov"]);
  });

  it("uses the mocked wall clock when no instant is given", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-11-10T23:30:00Z"));
    expect(fromNow(1)).toBe("2026-11-11T23:30:00Z");
    expect(callbackChoice(2).label).toBe("Thu 12 Nov");
  });

  it("the POST body carries the same `until` the label was formatted from", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "coordinator-secret-1234567890");
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ meta, error: "stop here" }), { status: 400 }));
    vi.stubGlobal("fetch", fetchMock);
    const choice = callbackChoice(2, ms("2026-11-10T23:59:30Z"));
    await postAction("c", choice.action, "h");
    const body = JSON.parse(fetchMock.mock.calls[0][1].body);
    expect(body).toEqual({ action: "snooze", until: "2026-11-12T23:59:30Z", reason: "Call back Thu 12 Nov", expected_head: "h" });
    expect(WEEKDAY.format(new Date(body.until))).toBe(choice.label);
  });
});
