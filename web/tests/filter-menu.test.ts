// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FilterMenu } from "@/components/ui/filters/filter-menu";
import type { FilterOption, FilterSpec } from "@/components/ui/filters/types";

const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
vi.mock("next/link", () => ({ default: ({ href, children, className, "aria-label": label }: Record<string, unknown>) => createElement("a", { href: String(href), className, "aria-label": label }, children as never) }));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const option = (value: string, label: string, count: number): FilterOption => ({ value, label, count, href: `/list?programme=${value}` });
const PROGRAMMES = [option("crc", "Colorectal screening", 42), option("breast", "Breast screening", 7), option("cervical", "Cervical screening", 0)];
const spec = (over: Partial<FilterSpec> = {}): FilterSpec => ({ label: "Programme", options: PROGRAMMES, clearHref: "/list?closed=true", ...over });

let host: HTMLDivElement;
let root: Root;
function mount(props: Parameters<typeof FilterMenu>[0]) {
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  act(() => root.render(createElement(FilterMenu, props)));
}

afterEach(() => {
  act(() => root.unmount());
  document.body.innerHTML = "";
  push.mockReset();
});

const chip = () => host.querySelector<HTMLButtonElement>('button[aria-haspopup="listbox"]')!;
const listbox = () => document.querySelector<HTMLElement>('[role="listbox"]');
const options = () => [...document.querySelectorAll<HTMLElement>('[role="option"]')];
const key = (el: Element, k: string) => act(() => void el.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true })));
/** Radix moves focus back to the trigger a task after the popover unmounts. */
const settle = () => act(() => new Promise<void>((r) => setTimeout(r, 0)));
const activeOption = () => document.getElementById(listbox()!.getAttribute("aria-activedescendant")!);

describe("FilterMenu", () => {
  it("reads as the attribute with a chevron when unset, and opens on click into a listbox", () => {
    mount(spec());
    expect(chip().textContent).toBe("Programme");
    expect(chip().getAttribute("aria-expanded")).toBe("false");
    expect(listbox()).toBeNull();

    act(() => chip().click());
    expect(chip().getAttribute("aria-expanded")).toBe("true");
    expect(listbox()!.getAttribute("aria-label")).toBe("Programme");
    expect(document.activeElement).toBe(listbox());
    // The count sits in its own cell beside the label, never inside it.
    expect(options().map((o) => [o.querySelector(".truncate")!.textContent, o.lastElementChild!.textContent])).toEqual([
      ["Colorectal screening", "42"],
      ["Breast screening", "7"],
      ["Cervical screening", "0"],
    ]);
    expect(options().every((o) => o.getAttribute("aria-selected") === "false")).toBe(true);
  });

  it("opens on Down Arrow and selects by keyboard: Enter opens the option's URL, closes and returns focus to the chip", async () => {
    mount(spec());
    chip().focus();
    key(chip(), "ArrowDown");
    expect(listbox()).not.toBeNull();
    expect(activeOption()!.textContent).toContain("Colorectal");

    key(listbox()!, "ArrowDown");
    expect(activeOption()!.textContent).toContain("Breast");
    key(listbox()!, "End");
    expect(activeOption()!.textContent).toContain("Cervical");
    key(listbox()!, "Home");
    expect(activeOption()!.textContent).toContain("Colorectal");
    key(listbox()!, "b");
    expect(activeOption()!.textContent).toContain("Breast");

    key(listbox()!, "Enter");
    expect(push).toHaveBeenCalledExactlyOnceWith("/list?programme=breast", { scroll: false });
    expect(listbox()).toBeNull();
    await settle();
    expect(document.activeElement).toBe(chip());
  });

  it("an option with no cases is still selectable", () => {
    mount(spec());
    act(() => chip().click());
    act(() => options()[2].click());
    expect(push).toHaveBeenCalledExactlyOnceWith("/list?programme=cervical", { scroll: false });
  });

  it("when set, reads as attribute and value, marks the option, and clears by its remove link or by choosing it again", () => {
    mount(spec({ value: "crc" }));
    expect(chip().textContent).toBe("Programme: Colorectal screening");
    const remove = host.querySelector<HTMLAnchorElement>('a[aria-label="Remove Programme filter"]')!;
    expect(remove.getAttribute("href")).toBe("/list?closed=true");

    act(() => chip().click());
    expect(options().map((o) => o.getAttribute("aria-selected"))).toEqual(["true", "false", "false"]);
    expect(activeOption()!.textContent).toContain("Colorectal");
    key(listbox()!, " ");
    expect(push).toHaveBeenCalledExactlyOnceWith("/list?closed=true", { scroll: false });
  });

  it("Escape closes without choosing and returns focus to the chip", async () => {
    mount(spec());
    chip().focus();
    act(() => chip().click());
    expect(listbox()).not.toBeNull();
    key(listbox()!, "Escape");
    expect(listbox()).toBeNull();
    expect(chip().getAttribute("aria-expanded")).toBe("false");
    await settle();
    expect(document.activeElement).toBe(chip());
    expect(push).not.toHaveBeenCalled();
  });

  it("past 8 options, a field narrows the list and keeps arrow and Enter selection", () => {
    const many = Array.from({ length: 9 }, (_, i) => option(`p${i}`, `Provider ${i}`, i));
    mount(spec({ label: "Provider", options: many }));
    act(() => chip().click());
    const field = document.querySelector<HTMLInputElement>('input[role="combobox"]')!;
    expect(field.getAttribute("aria-label")).toBe("Filter Provider options");
    expect(document.activeElement).toBe(field);
    expect(options()).toHaveLength(9);

    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(field, "8");
      field.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(options().map((o) => o.querySelector(".truncate")!.textContent)).toEqual(["Provider 8"]);
    expect(field.getAttribute("aria-activedescendant")).toBe(options()[0].id);
    key(field, "Enter");
    expect(push).toHaveBeenCalledExactlyOnceWith("/list?programme=p8", { scroll: false });
  });

  it("does not open when disabled", () => {
    mount({ ...spec(), disabled: true });
    expect(chip().getAttribute("aria-disabled")).toBe("true");
    act(() => chip().click());
    expect(listbox()).toBeNull();
  });
});
