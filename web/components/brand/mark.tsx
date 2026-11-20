import type { CSSProperties } from "react";
import { cx } from "@/lib/cx";

/** The brand palette. The mark never takes the state colours (positive, held, negative): those mean case status. */
export const BRAND = {
  paper: "#f6f3ee",
  surface: "#fffdf8",
  ink: "#0b2427",
  teal: "#0f4f4c",
  mint: "#dbe8e6",
  sand: "#efe8dc",
} as const;

/**
 * وصل ("connect", "link") in three strokes, redrawn from IBM Plex Sans Arabic's joined وصل as a single weight:
 * the ل with its ascender and descender bowl, the baseline running into the ص bowl, and the non-joining و whose tail
 * tucks under the ص. Condensed to fit the square; the ص tooth is dropped so the bowl stays open at small sizes.
 */
export const MASTER = {
  grid: 32,
  stroke: 3.2,
  d: "M7 3.4V23.2C7 27.2 5.6 28.4 4.2 28.4C2.6 28.4 1.8 27 1.8 24.6M7 20H18.4C19.8 20 20.6 18.8 20.6 16.6C20.6 12.6 15.8 12.2 13.6 15.4L10.6 20M20.4 28.4H23.6C28 28.4 30.2 25.6 30.2 20.6V17C30.2 14.8 29 13.6 27.7 13.6C26.3 13.6 25.2 14.8 25.2 16.6C25.2 18.8 26.3 20 28 20H30.2",
} as const;

/**
 * The 16px drawing: ل stem on x=4, baseline on y=11 and the و stem on x=14, so the 2px strokes land on whole pixels,
 * with a one-pixel gap between ل and ص and a one-pixel hole in the ص bowl. Already inset for the tile.
 */
export const HINTED = {
  grid: 16,
  stroke: 2,
  d: "M4 2.5V12.4C4 13.4 3.5 14 2.8 14C2.3 14 2 13.6 2 13V12M4 11H9C9.6 11 10 10.6 10 10V7.4C10 6.6 9.4 6 8.6 6H8.4C7.6 6 7 6.6 7 7.4V9.2L6.2 11M14 11H13.2C12.6 11 12.5 10.4 12.5 9.6V8.4C12.5 7.8 12.9 7.5 13.3 7.5C13.8 7.5 14 7.8 14 8.4V12.4C14 13.4 13.4 14 12.4 14H10.5",
} as const;

const TILE_INSET = 4.5;
const TILE_RX = { 32: 7.5, 16: 3.5 } as const;

/**
 * `light` sits on paper or sand: teal tile with a paper knockout, or a teal bare mark.
 * `dark` sits on ink: mint tile with an ink knockout, or a mint bare mark. A teal tile vanishes on ink.
 */
export type BrandTone = "light" | "dark";

const TONES = {
  light: { ground: BRAND.teal, knockout: BRAND.paper, word: BRAND.ink },
  dark: { ground: BRAND.mint, knockout: BRAND.ink, word: BRAND.paper },
} as const;

/** A plain function, not a component: Satori (the OG renderer) accepts only intrinsic elements inside <svg>. */
function stroke(geometry: typeof MASTER | typeof HINTED, color: string) {
  return (
    <path d={geometry.d} fill="none" stroke={color} strokeWidth={geometry.stroke} strokeLinecap="round" strokeLinejoin="round" />
  );
}

/**
 * The symbol. `tile` knocks the mark out of a rounded square (favicon, app icon, sidebar, phone);
 * `bare` is the mark alone (lockups). `color` sets a one-colour mark, or the tile ground.
 * At 16px and below it switches to the hinted drawing, which stays sharp where the master blurs.
 */
export function Mark({
  size,
  variant = "tile",
  tone = "light",
  color,
  title,
  className,
  style,
}: {
  size: number | string;
  variant?: "tile" | "bare";
  tone?: BrandTone;
  color?: string;
  title?: string;
  className?: string;
  style?: CSSProperties;
}) {
  const t = TONES[tone];
  const hinted = typeof size === "number" && size <= 16;
  const geometry = hinted ? HINTED : MASTER;
  const g = geometry.grid;
  const svgProps = {
    xmlns: "http://www.w3.org/2000/svg",
    width: size,
    height: size,
    viewBox: `0 0 ${g} ${g}`,
    "data-brand": "mark",
    role: title ? "img" : undefined,
    "aria-label": title,
    "aria-hidden": title ? undefined : true,
    className: cx("block flex-none", className),
    style,
  } as const;

  if (variant === "bare") {
    return (
      <svg {...svgProps}>
        {stroke(geometry, color ?? t.ground)}
      </svg>
    );
  }
  const ground = color ?? t.ground;
  return (
    <svg {...svgProps}>
      <rect width={g} height={g} rx={TILE_RX[g]} fill={ground} />
      {hinted ? (
        stroke(HINTED, t.knockout)
      ) : (
        <g transform={`translate(${TILE_INSET} ${TILE_INSET}) scale(${(g - TILE_INSET * 2) / g})`}>
          {stroke(MASTER, t.knockout)}
        </g>
      )}
    </svg>
  );
}

/**
 * The logo: the وصل symbol beside the Latin wordmark "Wasl". Never set وصل as text beside it; the symbol already says it.
 * Sized by font-size (the symbol is 1em tall), so a fluid text size scales the whole lockup.
 * `color` makes it one colour.
 */
export function Logo({
  tone = "light",
  color,
  className,
  style,
}: {
  tone?: BrandTone;
  color?: string;
  className?: string;
  style?: CSSProperties;
}) {
  const t = TONES[tone];
  return (
    <span
      data-brand="logo"
      className={cx("inline-flex items-center leading-none", className)}
      style={{ gap: "0.36em", color: color ?? t.word, ...style }}
    >
      <Mark size="1em" variant="bare" tone={tone} color={color} />
      <span style={{ fontSize: "0.86em", fontWeight: 700, letterSpacing: "-0.02em" }}>Wasl</span>
    </span>
  );
}
