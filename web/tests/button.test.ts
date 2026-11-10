import { describe, expect, it } from "vitest";
import { buttonVariants } from "@/components/ui/button";

type Variant = NonNullable<Parameters<typeof buttonVariants>[0]>["variant"];
const VARIANTS: Variant[] = ["default", "destructive", "outline", "secondary", "ghost", "link", "cta", "quiet"];
const PRESS = "motion-safe:not-disabled:not-aria-disabled:not-aria-busy:active:scale-[0.96]";
const classes = (...args: Parameters<typeof buttonVariants>) => buttonVariants(...args).split(" ");

describe("shared Button pressed feedback", () => {
  it.each(VARIANTS)("%s squeezes to 0.96 on press, only with motion allowed and only when it can act", (variant) => {
    const c = classes({ variant });
    expect(c).toContain(PRESS);
    // One squeeze, no deeper or competing scale.
    expect(c.filter((k) => /scale-/.test(k))).toEqual([PRESS]);
  });

  it.each(VARIANTS)("%s transitions paint-only properties, within 100 to 160ms", (variant) => {
    const c = classes({ variant });
    const transition = c.filter((k) => k.startsWith("transition"));
    expect(transition).toHaveLength(1);
    const props = transition[0].replace(/^transition-\[|\]$/g, "").split(",");
    expect(props).toEqual(expect.arrayContaining(["transform", "scale"]));
    // Never a layout property, so the squeeze cannot shift surrounding content.
    for (const p of props) expect(["color", "background-color", "border-color", "box-shadow", "transform", "scale"]).toContain(p);
    expect(c).toContain("duration-150");
  });

  it("keeps the shadcn ring on the stock variants", () => {
    for (const variant of VARIANTS.filter((v) => v !== "cta" && v !== "quiet")) {
      const c = classes({ variant });
      expect(c).toEqual(expect.arrayContaining(["outline-none", "focus-visible:ring-[3px]", "font-medium", "whitespace-nowrap", "shrink-0"]));
    }
  });
});

describe("product action variants", () => {
  // The recipes they replace, from the worklist and patients screens.
  const PRIMARY = "h-11 cursor-pointer rounded-control bg-cta px-4 text-body font-medium text-white";
  const SECONDARY = "h-11 cursor-pointer rounded-control bg-active px-4 text-body text-ink hover:bg-control";

  it.each([
    ["cta", PRIMARY],
    ["quiet", SECONDARY],
  ] as const)("%s at size touch carries the old local recipe and none of the stock look", (variant, recipe) => {
    const c = classes({ variant, size: "touch" });
    expect(c).toEqual(expect.arrayContaining(recipe.split(" ")));
    for (const stock of ["outline-none", "focus-visible:ring-[3px]", "rounded-md", "text-sm", "whitespace-nowrap", "shrink-0"]) expect(c).not.toContain(stock);
  });

  it("the secondary keeps the body's regular weight", () => {
    expect(classes({ variant: "quiet", size: "touch" }).some((k) => k.startsWith("font-"))).toBe(false);
  });

  it("merges call-site placement classes", () => {
    expect(classes({ variant: "cta", size: "touch", className: "relative z-10" })).toEqual(expect.arrayContaining(["relative", "z-10", "h-11"]));
  });
});
