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
 * the ل with its ascender and descender bowl, the baseline running into a narrow ص bowl, and the non-joining و set
 * apart with its tail kept clear of the ص. The ص tooth is dropped so the letters keep visible gaps at small sizes.
 */
export const MASTER = {
  grid: 32,
  stroke: 3.2,
  d: "M6.6 3.4V23.2C6.6 27.2 5.3 28.4 4 28.4C2.5 28.4 1.8 27 1.8 24.6M6.6 20H17.4C18.8 20 19.6 18.8 19.6 16.8C19.6 13 15.6 12.4 13.6 15.4L11 20M24.2 28.4H25.2C28.6 28.4 30.2 26 30.2 21.4V16.6C30.2 14.4 29.2 13 27.9 13C26.6 13 25.6 14.4 25.6 16.4C25.6 18.8 26.6 20 28.2 20H30.2",
} as const;

/**
 * The 16px drawing: every stroke centre sits on a whole pixel (ل stem x=4, ص sides x=7 and x=10, و x=13 to 14,
 * baseline y=11), so the 2px strokes stay crisp, with clean one-pixel columns between ل and ص (x=5) and ص and و (x=11)
 * and a one-pixel hole in the ص bowl. Already inset for the tile.
 */
export const HINTED = {
  grid: 16,
  stroke: 2,
  d: "M4 2.5V12.5C4 13.6 3.4 14 2.8 14C2.3 14 2 13.6 2 13V12M4 11H9C9.6 11 10 10.6 10 10V7C10 6.4 9.6 6 9 6H8C7.4 6 7 6.4 7 7V11M14 11H13V8C13 7.6 13.2 7.4 13.5 7.4C13.8 7.4 14 7.6 14 8V12.6C14 13.5 13.6 14 12.8 14H12",
} as const;

const TILE_INSET = 3.5;
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
