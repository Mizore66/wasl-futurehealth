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
 * وصل typed in IBM Plex Sans Arabic Bold: shaped with HarfBuzz and stored as filled glyph outlines (fontTools SVG pen),
 * so no font loads at runtime and Satori draws it as a plain path. Fitted to x 0.5 to 31.5 of the 32-unit grid,
 * centred on y=16. Bold, not SemiBold, so the strokes stay a full pixel wide in the 16px favicon.
 */
export const WORD = {
  grid: 32,
  d: "M4.8 23.49Q2.67 23.49 1.59 22.4Q0.5 21.3 0.5 19.3Q0.5 18.61 0.69 17.75Q0.88 16.89 1.22 16.08L2.69 16.5Q2.38 17.16 2.2 17.83Q2.03 18.51 2.03 19Q2.03 19.96 2.6 20.49Q3.18 21.01 4.18 21.01H5.41Q6 21.01 6.42 20.87Q6.83 20.72 7.08 20.39Q7.33 20.07 7.45 19.54Q7.56 19.01 7.56 18.25V8.51H9.4V17.53H11.09V19.36L10.63 19.82H9.27Q8.97 21.7 7.88 22.6Q6.8 23.49 4.8 23.49ZM14.46 16.46Q14.46 17.12 14.82 17.36L15.5 16.2Q16.03 15.27 16.59 14.61Q17.15 13.95 17.76 13.54Q18.37 13.13 19.05 12.94Q19.73 12.74 20.51 12.74Q22.31 12.74 23.21 13.73Q24.12 14.72 24.12 16.47Q24.12 18.2 23.14 19.01Q22.16 19.82 20.05 19.82H15.68Q14.46 19.82 13.82 19.32Q13.19 18.83 12.97 17.76H12.88Q12.67 18.83 12.15 19.32Q11.63 19.82 10.63 19.82V17.99L11.09 17.53H11.55Q12.77 17.53 12.77 16.46V15.5H14.46ZM20.05 17.53Q20.78 17.53 21.27 17.48Q21.75 17.42 22.05 17.29Q22.34 17.16 22.47 16.93Q22.59 16.7 22.59 16.34Q22.59 15.68 22.16 15.36Q21.73 15.04 20.74 15.04H20.28Q19.59 15.04 19.07 15.09Q18.55 15.14 18.16 15.3Q17.76 15.45 17.44 15.72Q17.12 16 16.83 16.44L16.17 17.44L16.21 17.53ZM25.11 21.2H26.09Q28.09 21.2 29.02 20.5Q29.96 19.81 29.96 18.71V18.31H29.86Q29.64 19.09 29.12 19.45Q28.61 19.82 27.83 19.82Q26.59 19.82 25.9 19.1Q25.2 18.37 25.2 17.07Q25.2 16.28 25.44 15.59Q25.68 14.91 26.1 14.42Q26.52 13.92 27.09 13.64Q27.66 13.36 28.34 13.36Q29.08 13.36 29.67 13.67Q30.26 13.98 30.67 14.57Q31.07 15.16 31.29 16.02Q31.5 16.87 31.5 17.94Q31.5 20.56 30.16 22.02Q28.82 23.49 26.24 23.49H25.11ZM28.31 17.53Q28.9 17.53 29.3 17.48Q29.7 17.42 29.94 17.3Q29.88 16.55 29.38 16.1Q28.87 15.65 28.14 15.65Q27.54 15.65 27.14 15.96Q26.73 16.28 26.73 16.76Q26.73 17.18 27.07 17.35Q27.4 17.53 27.98 17.53Z",
} as const;

/** Padding around the word inside the tile; at 16px and below the word grows to fill the tile so it stays legible. */
const TILE_INSET = { regular: 3, small: 1.5 } as const;
const TILE_RX = 7.5;

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
function word(color: string) {
  return <path d={WORD.d} fill={color} />;
}

/**
 * The symbol. `tile` knocks the mark out of a rounded square (favicon, app icon, sidebar, phone);
 * `bare` is the word alone (lockups). `color` sets a one-colour mark, or the tile ground. One path at every size;
 * at 16px and below the tile padding shrinks so the word is drawn larger.
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
  const g = WORD.grid;
  const inset = typeof size === "number" && size <= 16 ? TILE_INSET.small : TILE_INSET.regular;
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
        {word(color ?? t.ground)}
      </svg>
    );
  }
  const ground = color ?? t.ground;
  return (
    <svg {...svgProps}>
      <rect width={g} height={g} rx={TILE_RX} fill={ground} />
      <g transform={`translate(${inset} ${inset}) scale(${(g - inset * 2) / g})`}>{word(t.knockout)}</g>
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
