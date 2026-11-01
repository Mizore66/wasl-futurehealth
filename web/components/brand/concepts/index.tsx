import { join } from "./join";
import { sad } from "./sad";
import { seal } from "./seal";
import { w } from "./w";
import { BRAND } from "../mark";
import { GRID, type Concept } from "./types";

export type { Concept };
export const CONCEPTS: Concept[] = [join, seal, sad, w];

export { BRAND };

const TILE_INSET = 4.5;
const TILE_SCALE = (GRID - TILE_INSET * 2) / GRID;

/**
 * `tile`: the mark knocked out of a rounded teal square (favicon, app icon, sidebar).
 * `mark`: the bare mark in one colour (lockups, one-colour print).
 */
export function BrandSymbol({
  concept,
  size,
  variant = "tile",
  color = BRAND.teal,
  knockout = BRAND.paper,
  title,
}: {
  concept: Concept;
  size: number;
  variant?: "tile" | "mark";
  color?: string;
  knockout?: string;
  title?: string;
}) {
  if (size <= 16 && concept.small) {
    return (
      <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden style={{ display: "block", flex: "none" }}>
        {variant === "tile" && <rect width={16} height={16} rx={3.5} fill={color} />}
        {concept.small(variant === "tile" ? knockout : color)}
      </svg>
    );
  }
  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${GRID} ${GRID}`}
      role={title ? "img" : undefined}
      aria-hidden={title ? undefined : true}
      aria-label={title}
      style={{ display: "block", flex: "none" }}
    >
      {variant === "tile" ? (
        <>
          <rect width={GRID} height={GRID} rx={7.5} fill={color} />
          <g transform={`translate(${TILE_INSET} ${TILE_INSET}) scale(${TILE_SCALE})`}>{concept.draw(knockout)}</g>
        </>
      ) : (
        concept.draw(color)
      )}
    </svg>
  );
}

/** Symbol, Latin name and Arabic name on one line. Arabic is live text in IBM Plex Sans Arabic, so it shapes. */
export function BrandLockup({
  concept,
  height = 32,
  tone = "light",
  oneColour,
}: {
  concept: Concept;
  height?: number;
  tone?: "light" | "dark";
  oneColour?: string;
}) {
  const word = oneColour ?? (tone === "light" ? BRAND.ink : BRAND.paper);
  const accent = oneColour ?? (tone === "light" ? BRAND.teal : BRAND.mint);
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: height * 0.36, color: word, lineHeight: 1 }}>
      <BrandSymbol concept={concept} size={height} variant="mark" color={accent} />
      <span style={{ fontWeight: 700, fontSize: height * 0.86, letterSpacing: "-0.02em", fontFamily: "var(--font-sans)" }}>Wasl</span>
      <span aria-hidden style={{ width: Math.max(1.5, height * 0.07), height: height * 0.72, background: accent }} />
      <span lang="ar" dir="rtl" style={{ fontFamily: "var(--font-arabic)", fontWeight: 600, fontSize: height * 0.8, color: accent }}>
        وصل
      </span>
    </span>
  );
}
