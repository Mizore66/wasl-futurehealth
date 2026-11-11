import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

const web = path.resolve(import.meta.dirname, "..");
const read = (file: string) => readFileSync(path.join(web, file), "utf8").replace(/\r\n/g, "\n");
const css = read("app/globals.css");

/** The palette hex for `--color-<name>` in the @theme block. */
function palette(name: string): string {
  const m = css.match(new RegExp(`--color-${name}:\\s*(#[0-9a-fA-F]{6});`));
  if (!m) throw new Error(`--color-${name} is not a hex in globals.css`);
  return m[1].toLowerCase();
}

/** The palette hex a semantic `:root` role resolves to, through `var(--color-*)`. */
function role(name: string): string {
  const m = css.match(new RegExp(`\\n\\s*--${name}:\\s*var\\(--color-([a-z-]+)\\);`));
  if (!m) throw new Error(`--${name} does not point at a palette colour`);
  return palette(m[1]);
}

function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** WCAG 2.x contrast ratio. */
function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

// What an input or select boundary sits against in each state. Fields are filled with
// surface (or transparent on a surface panel); hover fills in the app are ground and
// active; a disabled or read-only field sits on raised. The boundary colour itself does
// not change between states, so each state is the same border against a different fill.
const STATES: Record<string, string[]> = {
  normal: ["surface"],
  hover: ["ground", "active"],
  disabled: ["raised"],
};

describe("input boundary token (WCAG 1.4.11)", () => {
  it("is not the control fill any more", () => {
    expect(role("input")).not.toBe(palette("control"));
  });

  it("checks against the WCAG reference values", () => {
    expect(contrast("#000000", "#ffffff")).toBeCloseTo(21, 5);
    // The old boundary: control on surface, the failure reported in #48.
    expect(contrast(palette("control"), palette("surface"))).toBeCloseTo(1.63, 2);
  });

  it.each(Object.entries(STATES))("clears 3:1 in the %s state", (_, fills) => {
    for (const fill of fills) {
      expect(contrast(role("input"), palette(fill)), `--input on ${fill}`).toBeGreaterThanOrEqual(3);
    }
  });

  it("keeps the control fill, the focus ring and the text colours as they were", () => {
    expect(palette("control")).toBe("#d3c8b4");
    expect(role("ring")).toBe(palette("hue"));
    expect(css).toMatch(/:focus-visible \{\n\s*outline: 2px solid var\(--color-hue\);\n\s*outline-offset: 2px;/);
    expect([palette("ink"), palette("ink-soft"), palette("muted")]).toEqual(["#0b2427", "#34494b", "#4f5f63"]);
  });
});

describe("inputs and selects use the input boundary", () => {
  // Where each field's boundary is drawn: the class string on the element that carries the border.
  const FIELDS: [string, string, RegExp][] = [
    ["worklist search (Today, Referrals)", "components/worklist/search-field.tsx", /<label\s+className=\{cx\(\s*"([^"]*)"/],
    ["Patients case-id search", "app/app/patients/page.tsx", /<label className="([^"]*)">\s*<span className="sr-only">Search case id/],
    ["Patients refine selects", "components/patients/refine.tsx", /<select[^>]*?className="([^"]*)"/],
  ];

  it.each(FIELDS)("%s draws its boundary with border-input", (_, file, where) => {
    const m = read(file).match(where);
    expect(m, `boundary element in ${file}`).not.toBeNull();
    const classes = m![1].split(/\s+/);
    expect(classes).toContain("border-input");
    expect(classes).not.toContain("border-control");
    // No opacity on the boundary: it must not fade below 3:1 in any state.
    expect(classes.some((c) => /border-input\//.test(c) || /opacity/.test(c))).toBe(false);
  });

  it("keeps the worklist search's focus indicator", () => {
    expect(read("components/worklist/search-field.tsx")).toContain(
      "focus-within:border-hue focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-hue",
    );
  });
});
