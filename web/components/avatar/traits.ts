/**
 * Seed → features for the product avatar, a flat portrait. Pure and deterministic: the same person key
 * gives the same features on every screen. Rendering lives in avatar.tsx.
 */

import type { PersonSeed, Sex } from "@/lib/people/types";

export type { PersonSeed, Sex };

/** FNV-1a, 32-bit. Small and stable across runtimes. */
export function hash(text: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0;
}

/** murmur3 finaliser over (seed, slot): each feature draws from its own independent stream. */
export function draw(seed: number, slot: number, n: number): number {
  let h = (seed ^ Math.imul(slot + 1, 0x9e3779b9)) >>> 0;
  h ^= h >>> 16;
  h = Math.imul(h, 0x85ebca6b);
  h ^= h >>> 13;
  h = Math.imul(h, 0xc2b2ae35);
  h ^= h >>> 16;
  return (h >>> 0) % n;
}

/** Six tones, light to deep. */
export const SKIN = ["#f4d9c3", "#e5b793", "#c98f67", "#a06b45", "#774c2f", "#4c3020"] as const;
export const HAIR = ["#1d1815", "#3a281e", "#5b3b25", "#7d4a2a", "#8e8983"] as const;
export const BG = ["#e8dccb", "#d6e5e1", "#ead2cc", "#dbe1ec", "#efe0bf", "#d7e1cc", "#e3d6e7", "#f0d8c0", "#cfe0e7", "#e4dfd3"] as const;
export const CLOTH = ["#0f4f4c", "#2e4a6c", "#8c3b2d", "#b0863a", "#5d6b39", "#6c4b7c", "#33393b", "#b8ab92"] as const;
export const COVER = ["#f8f6f1", "#24302f", "#c8b598", "#0f4f4c", "#b77870", "#2e4a6c", "#7a8a5a"] as const;
export const GHUTRA = ["#f8f6f1", "#efe9dd", "#b2383a"] as const;

/** Tops a record without a sex field may draw: hair only, no gendered headwear. */
const NEUTRAL_TOPS = ["short", "crop", "curly", "bald", "wavy"] as const;
const FEMALE_TOPS = ["short", "curly", "wavy", "long", "bun", "shayla", "hijab"] as const;
const MALE_TOPS = ["short", "crop", "curly", "bald", "ghutra", "kufi"] as const;
export type Top = (typeof NEUTRAL_TOPS | typeof FEMALE_TOPS | typeof MALE_TOPS)[number];

const ACCENTS = {
  neutral: ["none", "blush"],
  female: ["none", "earrings", "blush"],
  male: ["none", "beard", "moustache"],
} as const;
export type Accent = (typeof ACCENTS)[keyof typeof ACCENTS][number];

export const COVERINGS: ReadonlySet<Top> = new Set(["shayla", "hijab", "ghutra", "kufi"]);

export function topFill(top: Top, color: number): string {
  if (top === "ghutra") return GHUTRA[color];
  if (top === "shayla" || top === "hijab" || top === "kufi") return COVER[color];
  return HAIR[color];
}

export function topsFor(sex?: Sex): readonly Top[] {
  return sex === "female" ? FEMALE_TOPS : sex === "male" ? MALE_TOPS : NEUTRAL_TOPS;
}

export interface PortraitFeatures {
  skin: number;
  top: Top;
  /** Index into HAIR for hair tops, COVER or GHUTRA for coverings. */
  topColor: number;
  bg: number;
  cloth: number;
  face: number;
  eyes: number;
  mouth: number;
  accent: Accent;
}

export function portrait(p: PersonSeed): PortraitFeatures {
  const s = hash(p.key);
  const tops = topsFor(p.sex);
  const top = tops[draw(s, 1, tops.length)];
  const colors = top === "ghutra" ? GHUTRA.length : COVERINGS.has(top) ? COVER.length : HAIR.length;
  const accents = ACCENTS[p.sex ?? "neutral"];
  return {
    skin: draw(s, 0, SKIN.length),
    top,
    topColor: draw(s, 2, colors),
    bg: draw(s, 3, BG.length),
    cloth: draw(s, 4, CLOTH.length),
    face: draw(s, 5, 3),
    eyes: draw(s, 6, 4),
    mouth: draw(s, 7, 3),
    accent: accents[draw(s, 8, accents.length)],
  };
}

/** Features a viewer can tell apart at 24px: the big colour fields and the silhouette. */
export const SALIENT = ["skin", "top", "topColor", "bg", "cloth"] as const;

export function signature(f: object, keys?: readonly string[]): string {
  const entries = Object.entries(f).filter(([k]) => !keys || keys.includes(k));
  return entries.map(([k, v]) => `${k}=${v}`).join(";");
}
