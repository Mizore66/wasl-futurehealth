// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RecoveryBoard } from "@/components/recovery/recovery-board";
import { RecoveryDetail, RecoveryList } from "@/lib/api/recovery-schema";

vi.mock("@/app/app/recovery/actions", () => ({ cancelBackfill: vi.fn(), confirmBackfill: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }), usePathname: () => "/app/recovery" }));
vi.mock("next/link", () => ({ default: (props: Record<string, unknown>) => createElement("a", { href: String(props.href) }, props.children as never) }));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const read = (name: string) => JSON.parse(readFileSync(path.join(__dirname, "..", "data", "api", name), "utf8"));
const list = RecoveryList.parse(read("recovery_slots.json"));
const base = RecoveryDetail.parse(read("recovery_slot.json"));
const [first, ...rest] = base.slot.candidates;
// #173: the API excludes a returned referral with an unresolved hand-off and
// reports the failed procedure_ready hard check; the board must not offer it.
const held = RecoveryDetail.parse({
  ...base,
  slot: {
    ...base.slot,
    candidates: [
      { ...first, eligible: false, rank: null, checks: [...first.checks, { code: "procedure_ready", kind: "hard", result: "fail", reason_code: "returned_referral_unresolved" }] },
      ...rest,
    ],
  },
});

let container: HTMLDivElement | undefined;
let root: Root | undefined;

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  document.body.innerHTML = "";
});

describe("recovery board returned-referral readiness", () => {
  it("shows a held returned referral as excluded without a backfill action", async () => {
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
    await act(async () => root!.render(createElement(RecoveryBoard, { list, detail: held, readOnly: false })));
    const item = document.querySelector(`[data-eligible="false"]`);
    expect(item?.textContent).toContain("Excluded");
    expect(item?.querySelector("[data-testid=excluded-by]")?.textContent).toContain("Returned referral unresolved");
    expect([...(item?.querySelectorAll("button") ?? [])].some((node) => /Confirm backfill|Choose/.test(node.textContent ?? ""))).toBe(false);
    // With the only eligible case held, nobody is offered the slot.
    expect([...document.querySelectorAll("button")].some((node) => /Confirm backfill|Choose/.test(node.textContent ?? ""))).toBe(false);
  });
});
