// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { niceMax } from "@/components/charts/kit";
import { ChartFigure } from "@/components/programme/chart-kit";
import { BoundChart } from "@/components/results/charts";

vi.mock("next/link", () => ({ default: ({ href, children }: Record<string, unknown>) => createElement("a", { href: String(href) }, children as never) }));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let host: HTMLDivElement;
let root: Root;
function mount(el: React.ReactElement) {
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  act(() => root.render(el));
}

afterEach(() => {
  act(() => root.unmount());
  document.body.innerHTML = "";
});

describe("the table toggle", () => {
  it("is an icon-only button named Show as table, with aria-pressed and a title naming what a click does", () => {
    mount(
      createElement(ChartFigure, { caption: "Cap", table: { caption: "T", head: ["A", "B"], rows: [["x", "1"]] } } as Parameters<typeof ChartFigure>[0], createElement("div", { "data-chart": "" })),
    );
    const button = host.querySelector("button")!;
    expect(button.getAttribute("aria-label")).toBe("Show as table");
    expect(button.textContent).toBe("");
    expect(button.querySelector("svg")).not.toBeNull();
    expect(button.getAttribute("aria-pressed")).toBe("false");
    expect(button.title).toBe("Show as table");
    act(() => button.click());
    expect(button.getAttribute("aria-label")).toBe("Show as table");
    expect(button.getAttribute("aria-pressed")).toBe("true");
    expect(button.title).toBe("Show as chart");
    expect(host.querySelector("table")).not.toBeNull();
    expect(host.querySelector("[data-chart]")).toBeNull();
  });
});

describe("zero-event counters", () => {
  it("read as cases checked with none wrong, the bound kept beside the count", () => {
    mount(createElement(BoundChart, { rows: [{ label: "CRC false closures", events: 0, denominator: 3035, upper: 0.00121, testId: "crc" }] }));
    const row = host.querySelector("[data-testid='crc']")!;
    expect(row.textContent).toContain("0 of 3,035");
    expect(row.textContent).toContain("≤ 0.12%");
    expect(row.querySelector("svg")).not.toBeNull(); // the check mark
    expect(host.textContent).toContain("Cases checked");
  });

  it("rounds the count axis up to a round end", () => {
    expect(niceMax(16486)).toBe(20000);
    expect(niceMax(5627)).toBe(8000);
  });
});
