// @vitest-environment happy-dom

import { act, createElement, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { IfhasLabelComposer } from "@/components/review/ifhas-label-composer";
import { ReviewDesk } from "@/components/review/review-desk";
import { GUIDED_EVIDENCE_LABELS, type CaseDetail } from "@/lib/api/schema";
import { programmeDetail } from "./programme-cases";

vi.mock("@/app/app/review/[id]/actions", () => ({ submitReviewRead: vi.fn() }));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

async function mount(element: React.ReactElement) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(element));
}

afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  container?.remove();
  root = undefined;
  container = undefined;
});

const radio = (label: string) => {
  const input = [...container!.querySelectorAll<HTMLInputElement>("input[type=radio]")].find((i) => i.closest("label")?.textContent === label);
  expect(input, label).toBeDefined();
  return input!;
};
const select = (domain: string) => container!.querySelector<HTMLSelectElement>(`select[name$="-${domain}"]`)!;
const labelText = () => container!.querySelector('[data-testid="ifhas-label"]')?.textContent ?? "";

async function click(input: HTMLInputElement) {
  await act(async () => input.click());
}
async function choose(domain: string, value: string) {
  await act(async () => {
    const s = select(domain);
    s.value = value;
    s.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

function ifhasEvidenceDetail(): CaseDetail {
  const detail = programmeDetail("ifhas");
  detail.case.review = "open";
  detail.review_items = [
    { item: "rv-ifhas", loop: detail.case.id, kinds: ["evidence"], pair: [], p: 0, report: detail.evidence[0].report, verdict: "disagree", quote: "", chain: [] },
  ];
  return detail;
}

describe("IFHAS evidence on the review desk", () => {
  it("builds the label one domain at a time instead of listing 304 strings", async () => {
    await mount(
      createElement(ReviewDesk, { view: reviewView(ifhasEvidenceDetail()), mode: "fixtures", next: { href: "/app/review", label: "Back to Review", last: true } }),
    );
    expect(container!.querySelector('select[aria-label="Evidence label"]')).toBeNull();
    expect(container!.querySelectorAll("option")).toHaveLength(0);
    expect(container!.textContent).toContain("Answer the evidence question to sign.");

    await click(radio("Screening result"));
    expect(container!.querySelectorAll("select")).toHaveLength(4);
    expect([...select("glyc").options].map((o) => o.value)).toEqual(["", "N", "P", "D", "S"]);
    await choose("bp", "H");
    await choose("glyc", "P");
    await choose("lipid", "N");
    expect(labelText()).toBe("Choose a result for every domain.");
    expect(container!.textContent).not.toContain("Preview:");
    await choose("weight", "O");
    expect(labelText()).toBe("Evidence label bp=H;glyc=P;lipid=N;weight=O");
    expect(container!.textContent).toContain("Preview:");

    await click(radio("Follow-up document"));
    expect([...select("bp").options].map((o) => o.value)).toEqual(["", "managed", "not_confirmed", "pending"]);
    expect(select("bp").value).toBe("");
    expect(container!.textContent).not.toContain("Preview:");
    await choose("weight", "pending");
    await choose("glyc", "managed");
    expect(labelText()).toBe("Evidence label glyc=managed;weight=pending");
    expect(GUIDED_EVIDENCE_LABELS.ifhas).toContain("glyc=managed;weight=pending");
    await choose("glyc", "");
    expect(labelText()).toBe("Evidence label weight=pending");

    await click(radio("Did not attend"));
    expect(container!.querySelectorAll("select")).toHaveLength(0);
    expect(labelText()).toBe("Evidence label dna");
    expect(container!.textContent).toContain("Preview:");
  });

  it("follows the parent when it clears or replaces the answer", async () => {
    const picked: string[] = [];
    let setValue: (v: string | undefined) => void = () => {};
    function Harness() {
      const [value, set] = useState<string | undefined>("bp=managed;lipid=not_confirmed");
      setValue = set;
      return createElement(IfhasLabelComposer, {
        name: "q",
        value,
        disabled: false,
        onPick: (label: string) => {
          picked.push(label);
          set(label);
        },
      });
    }
    await mount(createElement(Harness));
    expect(radio("Follow-up document").checked).toBe(true);
    expect(select("bp").value).toBe("managed");
    expect(select("lipid").value).toBe("not_confirmed");
    expect(labelText()).toBe("Evidence label bp=managed;lipid=not_confirmed");

    await choose("bp", "pending");
    expect(picked).toEqual(["bp=pending;lipid=not_confirmed"]);

    await act(async () => setValue(undefined)); // a submitted read clears its draft
    expect(radio("Follow-up document").checked).toBe(false);
    expect(container!.querySelectorAll("select")).toHaveLength(0);

    await act(async () => setValue("bp=S;glyc=D;lipid=H;weight=N"));
    expect(radio("Screening result").checked).toBe(true);
    expect(["bp", "glyc", "lipid", "weight"].map((d) => select(d).value)).toEqual(["S", "D", "H", "N"]);
  });
});
