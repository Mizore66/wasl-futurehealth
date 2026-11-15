// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { IdentityBridge } from "@/components/review/identity-bridge";
import { CaseDetail } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

// Software inputs built in memory from the untouched legacy fixture, not
// original guard snapshots or invented metadata written back into fixtures.
const projection = hold.identity_review[0];
const unavailableReason = "original_guard_context_unavailable";
const withGuard = (guard: unknown, source_scope?: unknown) => ({
  ...hold,
  identity_review: [{ ...projection, source_scope, guard }],
});
const availableProjection = (input: unknown) => {
  const entry = CaseDetail.parse(input).identity_review?.[0];
  if (!entry?.available) throw new Error("software control lost its recorded registration comparison");
  return entry;
};

describe("identifier source and original guard authority", () => {
  it("types materialized records without promoting visible checks to the original guard", () => {
    const input = withGuard({ authoritative: false, reason: unavailableReason, visible_checks: { met: true, failures: ["name_or_shared_id_check"] } }, "materialized_registration");
    const before = JSON.stringify(input);
    const entry = availableProjection(input);
    expect(entry.source_scope).toBe("materialized_registration");
    expect(entry.guard).toMatchObject({ authoritative: false, reason: unavailableReason, visible_checks: { met: true, failures: [] } });
    expect(entry.guard).not.toHaveProperty("met");
    expect(entry.guard).not.toHaveProperty("failures");
    expect(entry.records[0].eid.value).toBe("784*********123");
    expect(JSON.stringify(input)).toBe(before);
  });

  it.each([true, false])("keeps an unqualified legacy met=%s result non-authoritative", (met) => {
    const entry = availableProjection(withGuard({ met, failures: met ? ["name_or_shared_id_check"] : ["dob_missing", "name_or_shared_id_check"] }));
    expect(entry.source_scope).toBeUndefined();
    expect(entry.guard).toMatchObject({ authoritative: false, reason: unavailableReason, visible_checks: { met, failures: met ? [] : ["dob_missing"] } });
    expect(entry.guard).not.toHaveProperty("met");
    expect(entry.guard).not.toHaveProperty("failures");
  });

  it("treats missing guard qualifiers as unavailable, not as a successful evaluation", () => {
    const entry = availableProjection(withGuard({ visible_checks: { met: true, failures: [] } }));
    expect(entry.guard).toMatchObject({ authoritative: false, reason: unavailableReason });
    expect(entry.guard).not.toHaveProperty("met");
  });

  it.each([
    { authoritative: true, reason: unavailableReason, visible_checks: { met: true, failures: [] } },
    { authoritative: true, met: true, failures: [] },
  ])("does not add an authoritative:true capability for input %#", (guard) => {
    expect(CaseDetail.safeParse(withGuard(guard, "materialized_registration")).success).toBe(false);
  });

  it("keeps an omitted guard unknown without inventing an original outcome", () => {
    const entry = availableProjection(withGuard(undefined));
    expect(entry.guard).toEqual({ authoritative: false, reason: unavailableReason });
    expect(entry.source_scope).toBeUndefined();
  });
});

describe("rendered identifier availability", () => {
  let root: Root | undefined;
  let container: HTMLDivElement | undefined;
  afterEach(async () => {
    if (root) await act(async () => root!.unmount());
    container?.remove();
    root = undefined;
  });
  const mount = async (input: unknown) => {
    const view = reviewView(CaseDetail.parse(input));
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
    await act(async () => root!.render(createElement(IdentityBridge, { view })));
    return { element: container, view };
  };

  it.each(["materialized_registration", undefined])("shows original guard unavailability for source scope %s even when visible checks pass", async (scope) => {
    const { element, view } = await mount(withGuard({ authoritative: false, reason: unavailableReason, visible_checks: { met: true, failures: [] } }, scope));
    expect(view.identityAvailable).toBe(true);
    expect(view.identityGuard).toEqual([]);
    const note = element.querySelector('[aria-label="Identifier source context"]');
    expect(note?.textContent).toMatch(/original configured guard result is unavailable/i);
    if (scope) expect(note?.textContent).toMatch(/materialized across recorded events/i);
    else expect(note?.textContent).toMatch(/source scope.*unavailable/i);
    expect(element.textContent).toContain("1965-03-14");
    expect(element.textContent).toContain("784*********123");
    expect(element.textContent).not.toContain("784199012345676");
    expect(element.textContent).not.toMatch(/original guard (?:passed|failed)|shared-ID check did not/);
    expect(element.querySelector('[aria-label="Why the second identifier is not met"]')).toBeNull();
  });

  it("keeps legacy field contrasts under neutral limitation wording, not an original guard failure", async () => {
    const { element, view } = await mount(withGuard({ met: false, failures: ["dob_missing", "name_or_shared_id_check"] }));
    expect(view.identityAvailable).toBe(true);
    expect(view.identityGuard).toEqual(["Date of birth is not recorded on one or both records."]);
    expect(element.querySelector('[aria-label="Visible identifier field limitations"]')?.textContent).toContain("Date of birth is not recorded");
    expect(element.querySelector('[aria-label="Identifier source context"]')?.textContent).toMatch(/source scope.*unavailable.*original configured guard result is unavailable/i);
    expect(element.querySelector('[aria-label="Why the second identifier is not met"]')).toBeNull();
    expect(element.textContent).not.toMatch(/shared-ID check did not|name-agreement.*did not/);
  });

  it("distinguishes missing registration records from original guard unavailability", async () => {
    const { element, view } = await mount({ ...hold, identity_review: [{ item: projection.item, pair: projection.pair, available: false, reason: "records_not_found" }] });
    expect(view.identityAvailable).toBe(false);
    expect(view.identityGuard).toEqual([]);
    expect(element.textContent).toMatch(/identifier comparison is not available/i);
    expect(element.textContent).toMatch(/original configured guard result is unavailable/i);
    expect(element.textContent).not.toContain("1965-03-14");
    expect(element.textContent).not.toContain("784*********123");
  });

  it("does not turn the expanded context into an unevaluated original rule explanation", async () => {
    const { element } = await mount(withGuard({ met: true, failures: ["name_or_shared_id_check"] }));
    const button = element.querySelector("button");
    expect(button).not.toBeNull();
    await act(async () => button!.click());
    const context = element.querySelector('[id^="why-held-"]');
    expect(context).not.toBeNull();
    expect(context?.textContent).toMatch(/original configured guard result is unavailable/i);
    expect(context?.textContent).not.toMatch(/Held anyway|rule asks for a|name-agreement.*did not|shared-ID check did not/);
  });
});
