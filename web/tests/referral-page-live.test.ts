import { beforeEach, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import Page from "@/app/app/referrals/page";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { CaseSummary, CurrentDecision } from "@/lib/api/schema";
import synthetic from "@/data/referral-inbox-synthetic-v2.json";
import handoffs from "@/data/referral-handoff-synthetic.json";

// #155: in live mode the list's hand-off marker (pipeline handoffMarker) decides which cases need a detail read.
const mocks = vi.hoisted(() => ({ all: vi.fn(), detail: vi.fn() }));
vi.mock("@/lib/api/all-cases", () => ({ getAllCases: mocks.all }));
vi.mock("@/lib/api/source", () => ({ apiMode: () => "live", getCase: mocks.detail, getDashboard: async () => ({ sla: { referral_response_days: 7 } }) }));
vi.mock("@/components/worklist/synthetic-referrals", () => ({ syntheticReferrals: async () => [], syntheticHandoffs: async () => [] }));
vi.mock("@/components/worklist/run-clock", () => ({ runClock: async () => 120, hasRecord: () => false }));
vi.mock("@/app/app/referrals/actions", () => ({ respondToReferral: vi.fn() }));

beforeEach(() => vi.clearAllMocks());

it("reads the canonical packet of marked lung and DR cases only", async () => {
  const dr = handoffs.filter((h) => h.case.programme === "diabetic_retinopathy");
  const marked = dr.map((h) => CaseSummary.parse(h.case));
  expect(marked.every((c) => c.handoff?.referral_id)).toBe(true);
  const rest: Record<string, unknown> = { ...dr[0].case };
  delete rest.handoff;
  const unmarked = CaseSummary.parse({ ...rest, id: "phc-retina/ServiceRequest/PHC-sr-9999" });
  mocks.all.mockResolvedValue({ cases: [...marked, unmarked] });
  mocks.detail.mockImplementation(async (id: string) => {
    const hit = dr.find((h) => h.case.id === id);
    return hit ? { current: CurrentDecision.parse(hit.current) } : null;
  });
  const result = await Page({ params: Promise.resolve({}), searchParams: Promise.resolve({ programme: "diabetic_retinopathy", provider: "phc-retina" }) });
  const props = result.props as ComponentProps<typeof ReferralInbox>;
  const read = mocks.detail.mock.calls.map(([id]) => id);
  expect(read).toEqual(marked.filter((c) => !c.closed).map((c) => c.id));
  expect(read.length).toBeGreaterThan(3);
  expect(read).not.toContain(unmarked.id);
  expect(props.referrals.map((r) => r.handoff?.status).sort()).toEqual(marked.filter((c) => !c.closed).map((c) => c.handoff!.status).sort());
});

it("reads a CRC case's packet only when the list marks it returned", async () => {
  const returned = CaseSummary.parse({ ...synthetic[3], handoff: { referral_id: "ev-1", status: "coordinator", clinical_priority: "urgent", decided_by: "skmc-endo", decided_at: 99, act_by: 103 } });
  const legacy = CaseSummary.parse({ ...synthetic[3], id: "ahs-mushrif/ServiceRequest/AHS-sr-9008" });
  const open = CaseSummary.parse(synthetic[0]);
  mocks.all.mockResolvedValue({ cases: [returned, legacy, open] });
  mocks.detail.mockResolvedValue({ current: CurrentDecision.parse({ id: "d", loop: returned.id, state: "referral_rejected", handoff: { clinical_priority: "urgent", decided_at: 99, act_by: 103, status: "coordinator" } }) });
  const result = await Page({ params: Promise.resolve({}), searchParams: Promise.resolve({}) });
  const props = result.props as ComponentProps<typeof ReferralInbox>;
  expect(mocks.detail.mock.calls.map(([id]) => id)).toEqual([returned.id]);
  expect(props.referrals.find((r) => r.id === returned.id)?.handoff).toEqual({ status: "coordinator", actBy: 103 });
  expect(props.respond).toBeDefined();
});
