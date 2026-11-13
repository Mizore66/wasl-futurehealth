import { describe, expect, it } from "vitest";
import { CaseList } from "@/lib/api/schema";
import { effectiveOwner } from "@/lib/api/owner";
import { ownerLabel } from "@/components/worklist/model";
import { loadSample } from "./samples";

const base = CaseList.parse(loadSample("cases_crc_page1.json")).cases[0];
const overlay = (owner: string) => ({ head: "h", actions: 1, owner, last_action_at: "", ledger_status: "ok" as const, no_answer_streak: 0 });

describe("effectiveOwner (record header)", () => {
  it("names the saved owner on a case that had none", () => {
    const c = { ...base, owner: "" };
    expect(ownerLabel(effectiveOwner(c, overlay("Hessa Al Ali")))).toBe("Hessa Al Ali");
  });

  it("prefers the saved owner over a different canonical owner", () => {
    const c = { ...base, owner: "Practitioner/ahs-mushrif-nurse-5" };
    expect(effectiveOwner(c, overlay("Dr Al Kaabi"))).toBe("Dr Al Kaabi");
  });

  it("falls back to the canonical owner without an overlay or without an assigned owner", () => {
    const c = { ...base, owner: "Practitioner/ahs-mushrif-nurse-5" };
    expect(effectiveOwner(c, undefined)).toBe(c.owner);
    expect(effectiveOwner(c, overlay(""))).toBe(c.owner);
  });
});
