// @vitest-environment happy-dom
import { readFileSync } from "node:fs";
import path from "node:path";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { TodayBoard } from "@/components/worklist/today-board";
import { workItem } from "@/components/worklist/model";
import { CaseList } from "@/lib/api/schema";

const base = CaseList.parse(JSON.parse(readFileSync(path.join(__dirname, "fixtures/ledger_case_list.json"), "utf8"))).cases[0];
let root: ReturnType<typeof createRoot> | null = null;
afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  root = null;
  vi.unstubAllGlobals();
  document.body.innerHTML = "";
});

it("rendered Waiting board places superseded snoozes by urgency but preserves later snoozes", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const now = Date.parse("2026-11-20T12:00:00Z");
  const summary = (id: string, contactAt: string, snoozeAt: string) => ({
    ...base, id, owner: "someone", dropoff: "not_booked" as const,
    api_overlay: {
      head: "h", actions: 2, owner: "", last_action_at: "", ledger_status: "ok" as const, no_answer_streak: 0,
      last_contact: { at: contactAt, event: "c", outcome: "appointment_agreed", channel: "phone" as const },
      snooze: { at: snoozeAt, until: "2026-11-21T12:00:00Z", reason: "Call back later" },
    },
  });
  const a = workItem(summary("a", "2026-11-20T10:00:00.000000002Z", "2026-11-20T10:00:00.000000001Z"), 40, true, now);
  const b = { ...workItem(summary("b", "2026-11-20T11:00:00Z", "2026-11-20T10:00:00Z"), 39, true, now), days: a.days - 1 };
  const c = { ...workItem(summary("c", "2026-11-20T11:00:00Z", "2026-11-20T11:30:00Z"), 40, true, now), days: a.days + 1 };
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, json: async () => ({ status: "ok", detail: "", applied: 0, entries: [] }) }));
  const host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  await act(async () => root!.render(createElement(TodayBoard, { items: [b, c, a], now: 40, aside: null, live: true })));
  const tabs = Array.from(host.querySelectorAll<HTMLButtonElement>("button[role=tab]"));
  await act(async () => tabs.find((t) => t.textContent?.startsWith("Waiting"))!.click());
  const rows = () => Array.from(host.querySelectorAll("[role=tabpanel] > div > button"));
  expect(rows().map((r) => r.querySelector(".font-semibold")?.textContent)).toEqual([a.name, b.name, c.name]);
  await act(async () => tabs.find((t) => t.textContent?.startsWith("To call"))!.click());
  expect(rows().map((r) => r.querySelector(".font-semibold")?.textContent)).toEqual([]);
});
