import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { byUrgency } from "@/components/worklist/days";
import { BOOKING_NEEDED, DECLINED_STATUS, workItem } from "@/components/worklist/model";
import { CaseList, type ApiOverlay } from "@/lib/api/schema";

const base = CaseList.parse(JSON.parse(readFileSync(path.join(__dirname, "fixtures", "ledger_case_list.json"), "utf8"))).cases[0];
const NOW = Date.parse("2026-11-20T12:00:00Z");
const UNTIL = "2026-11-21T12:00:00Z";
const outcome = (o: string, at: string) => ({ at, event: "e", outcome: o, channel: "phone" as const });
const item = (ov: Partial<ApiOverlay>, id = base.id) =>
  workItem(
    { ...base, id, owner: "someone", dropoff: "not_booked", api_overlay: { head: "h", actions: 2, owner: "", last_action_at: "", ledger_status: "ok", no_answer_streak: 0, ...ov } },
    40,
    true,
    NOW,
  );

describe("a newer contact outcome supersedes an earlier snooze's queue demotion", () => {
  const snooze = { until: UNTIL, reason: "Call back Sat 21 Nov", at: "2026-11-20T10:00:00Z" };

  it("an unexpired snooze followed by an agreed appointment is Waiting, booking needed and not demoted", () => {
    const it = item({ snooze, last_contact: outcome("appointment_agreed", "2026-11-20T11:00:00Z") });
    expect([it.view, it.status, it.snoozed]).toEqual(["waiting", BOOKING_NEEDED, false]);
  });

  it("a later declined or reached outcome is not demoted by the earlier snooze", () => {
    expect(item({ snooze, last_contact: outcome("declined", "2026-11-20T11:00:00Z") })).toMatchObject({ status: DECLINED_STATUS, snoozed: false });
    expect(item({ snooze, last_contact: outcome("reached", "2026-11-20T11:00:00Z") }).snoozed).toBe(false);
  });

  it("holds at timestamp precision: a zero-nanosecond snooze time against a later fractional outcome", () => {
    const s = { ...snooze, at: "2026-11-20T10:00:00Z" };
    expect(item({ snooze: s, last_contact: outcome("reached", "2026-11-20T10:00:00.5Z") }).snoozed).toBe(false);
    expect(item({ snooze: s, last_contact: outcome("reached", "2026-11-20T10:00:00Z") }).snoozed).toBe(false);
    expect(item({ snooze: { ...s, at: "2026-11-20T10:00:00.000000001Z" }, last_contact: outcome("reached", "2026-11-20T10:00:00Z") }).snoozed).toBe(true);
  });

  it("a snooze logged after the latest contact still sets the status and demotes until it expires", () => {
    const later = item({ snooze: { ...snooze, at: "2026-11-20T11:30:00Z" }, last_contact: outcome("appointment_agreed", "2026-11-20T11:00:00Z") });
    expect([later.status, later.snoozed]).toEqual(["Call back Sat 21 Nov", true]);
    const expired = item({ snooze: { ...snooze, until: "2026-11-20T11:45:00Z", at: "2026-11-20T11:30:00Z" }, last_contact: outcome("reached", "2026-11-20T11:00:00Z") });
    expect(expired.snoozed).toBe(false);
  });

  it("a recent latest no-answer still demotes on its own and an old one does not", () => {
    expect(item({ snooze, last_contact: outcome("no_answer", "2026-11-20T11:00:00Z"), no_answer_streak: 1 }).snoozed).toBe(true);
    expect(item({ snooze: { ...snooze, at: "2026-11-18T10:00:00Z" }, last_contact: outcome("no_answer", "2026-11-18T11:00:00Z"), no_answer_streak: 1 }).snoozed).toBe(false);
  });

  it("the superseded row sorts by urgency among non-snoozed rows after a reload", () => {
    const agreed = item({ snooze, last_contact: outcome("appointment_agreed", "2026-11-20T11:00:00Z") }, "a");
    const plain = { ...item({}, "b"), days: agreed.days - 1 };
    const stillSnoozed = item({ snooze: { ...snooze, at: "2026-11-20T11:30:00Z" } }, "c");
    expect([agreed, stillSnoozed, plain].sort(byUrgency).map((x) => x.id)).toEqual(["a", "b", "c"]);
  });
});
