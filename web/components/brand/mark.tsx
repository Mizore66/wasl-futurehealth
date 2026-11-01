import type { CSSProperties } from "react";
import { cn } from "@/lib/utils";

/** The brand palette. The mark never takes the state colours (positive, held, negative): those mean case status. */
export const BRAND = {
  paper: "#f6f3ee",
  surface: "#fffdf8",
  ink: "#0b2427",
  teal: "#0f4f4c",
  mint: "#dbe8e6",
  sand: "#efe8dc",
} as const;

/** صل (صِلْ, "connect") from وصل: the ص bowl, the baseline and the ل as one stroke. */
export const MASTER = {
  grid: 32,
  stroke: 3.2,
  d: "M8 4.5V21C8 27.5 2.5 27.5 2.5 23.5M8 22H15C15 14 28.5 12.5 28.5 18.5C28.5 22 24 22 15 22",
} as const;

/** The 16px drawing: stem on x=5 and baseline on y=11, so both 2px strokes land on whole pixels. Already inset for the tile. */
export const HINTED = {
  grid: 16,
  stroke: 2,
  d: "M5 3V10.5C5 13.5 2.5 13.5 2.5 12.5M5 11H8C8 5 14.5 4.5 14.5 8.5C14.5 11 12 11 8 11",
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
    className: cn("block flex-none", className),
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
 * The logo: the صل symbol beside the Latin wordmark "Wasl". Never set وصل beside it; the symbol already says it.
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
      className={cn("inline-flex items-center leading-none", className)}
      style={{ gap: "0.36em", color: color ?? t.word, ...style }}
    >
      <Mark size="1em" variant="bare" tone={tone} color={color} />
      <span style={{ fontSize: "0.86em", fontWeight: 700, letterSpacing: "-0.02em" }}>Wasl</span>
    </span>
  );
}
