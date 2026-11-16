import { beforeEach, expect, it, vi } from "vitest";
import { createElement, type ComponentProps } from "react";
import Page from "@/app/app/referrals/page";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { CaseSummary, CurrentDecision } from "@/lib/api/schema";
import synthetic from "@/data/referral-inbox-synthetic-v2.json";
import handoffs from "@/data/referral-handoff-synthetic.json";

const mocks = vi.hoisted(() => ({ all: vi.fn(), detail: vi.fn(), extra: vi.fn(), handoffs: vi.fn() }));
vi.mock("@/lib/api/all-cases", () => ({ getAllCases: mocks.all }));
vi.mock("@/lib/api/source", () => ({ apiMode: () => "fixtures", getCase: mocks.detail, getDashboard: async () => ({ sla: { referral_response_days: 7 } }) }));
vi.mock("@/components/worklist/synthetic-referrals", () => ({ syntheticReferrals: mocks.extra, syntheticHandoffs: mocks.handoffs }));
vi.mock("@/components/worklist/run-clock", () => ({ runClock: async () => 120, hasRecord: () => false }));

beforeEach(() => { vi.clearAllMocks(); mocks.extra.mockResolvedValue([]); mocks.handoffs.mockResolvedValue([]); });

it("uses complete accepted-main acquisition with server q filtering and each returned case's canonical current", async () => {
  const a = CaseSummary.parse(synthetic[3]);
  const b = CaseSummary.parse({ ...synthetic[3], id: "ahs-mushrif/ServiceRequest/AHS-sr-9006" });
  const missing = CaseSummary.parse({ ...synthetic[3], id: "ahs-mushrif/ServiceRequest/AHS-sr-9007" });
  const open = CaseSummary.parse(synthetic[0]);
  mocks.all.mockResolvedValue({ cases: [a, b, missing, open] });
  const packet = (id: string, decided_at: number, act_by: number, status: string) => ({ current: CurrentDecision.parse({ id: `decision-${id}`, loop: id, state: "referral_rejected", handoff: { clinical_priority: "urgent", decided_at, act_by, status } }) });
  mocks.detail.mockImplementation(async (id: string) => id === a.id ? packet(id, 99, 103, "coordinator") : id === b.id ? packet(id, 93, 97, "medical_director") : null);
  // Server component unit test: real page/projection, mocked reads; not served HTTP or browser evidence.
  const result = await Page({ params: Promise.resolve({}), searchParams: Promise.resolve({ q: "  900  " }) });
  expect(result.type).toBe(ReferralInbox);
  const props = result.props as ComponentProps<typeof ReferralInbox>;
  expect(mocks.all).toHaveBeenCalledExactlyOnceWith({ provider: "ahs-mushrif", programme: "crc", q: "900" });
  expect(mocks.extra).toHaveBeenCalledWith("900");
  expect(mocks.detail.mock.calls.map(([id]) => id)).toEqual([a.id, b.id, missing.id]);
  expect(props.referrals.map((r) => r.handoff)).toEqual([{ status: "coordinator", actBy: 103 }, { status: "medical_director", actBy: 97 }, null, null]);
  expect(props.q).toBe("900");
  expect(result.key).toBeNull();
  // Type-level binding keeps the actual returned component's complete required props checked by tsc.
  expect(createElement(ReferralInbox, props).type).toBe(ReferralInbox);
});

it("#155 loads the selected lung view from its canonical packets and keeps the selection in scope", async () => {
  const lung = handoffs.filter((h) => h.case.programme === "lung_nodule").map((h) => ({ case: CaseSummary.parse(h.case), current: CurrentDecision.parse(h.current) }));
  mocks.all.mockResolvedValue({ cases: [] });
  mocks.handoffs.mockResolvedValue(lung);
  const result = await Page({ params: Promise.resolve({}), searchParams: Promise.resolve({ programme: "lung_nodule", provider: "rad-imaging", q: "91" }) });
  const props = result.props as ComponentProps<typeof ReferralInbox>;
  expect(mocks.all).toHaveBeenCalledExactlyOnceWith({ provider: "rad-imaging", programme: "lung_nodule", q: "91" });
  expect(mocks.handoffs).toHaveBeenCalledExactlyOnceWith("lung_nodule", "rad-imaging", "91");
  expect(mocks.extra).not.toHaveBeenCalled();
  expect(mocks.detail).not.toHaveBeenCalled();
  expect(props.referrals.map((r) => [r.id, r.kind, r.state, r.handoff?.status])).toEqual([[lung[0].case.id, "packet", "returned", "awaiting_referrer"]]);
  expect(props.scope).toBe("programme=lung_nodule&provider=rad-imaging");
  expect(props.referrer).toBe("referrer");
  expect(props.respond).toBeUndefined();
});

it("#155 shows an honest empty view for an unknown programme and provider pair", async () => {
  const result = await Page({ params: Promise.resolve({}), searchParams: Promise.resolve({ programme: "lung_nodule", provider: "ahs-mushrif" }) });
  const props = result.props as ComponentProps<typeof ReferralInbox>;
  expect(props.referrals).toEqual([]);
  expect(mocks.all).not.toHaveBeenCalled();
  expect(props.scope).toBe("programme=lung_nodule&provider=ahs-mushrif");
});
