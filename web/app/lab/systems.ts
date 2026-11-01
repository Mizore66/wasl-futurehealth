import { BG, CLOTH, COVER, COVERINGS, GHUTRA, HAIR, SALIENT, SKIN, draw, hash, portrait, topsFor, type PersonSeed, type Top } from "@/components/avatar/traits";

/* The two avatar systems the lab keeps for comparison. The product draws only the portrait (system A). */

export interface TokenFeatures {
  skin: number;
  bg: number;
  ring: number;
  segments: number;
  rotation: number;
  orbit: number;
  orbitSpan: number;
  glyph: number;
}

export function token(p: PersonSeed): TokenFeatures {
  const s = hash(p.key);
  return {
    skin: draw(s, 0, SKIN.length),
    bg: draw(s, 3, BG.length),
    ring: draw(s, 4, CLOTH.length),
    segments: 1 + draw(s, 1, 4),
    rotation: draw(s, 2, 8),
    orbit: draw(s, 5, CLOTH.length),
    orbitSpan: draw(s, 6, 3),
    glyph: draw(s, 7, 4),
  };
}

export const MOTIFS = ["triangles", "diamonds", "chevrons", "checks", "stripes", "dots"] as const;

export interface SaduFeatures {
  skin: number;
  top: Top;
  topColor: number;
  bg: number;
  band: number;
  thread: number;
  motif: number;
}

/** A faceless bust over a band woven in an Al Sadu motif: identity is carried by colour and pattern, not by a drawn face. */
export function sadu(p: PersonSeed): SaduFeatures {
  const s = hash(p.key);
  const tops = topsFor(p.sex);
  const top = tops[draw(s, 1, tops.length)];
  const colors = top === "ghutra" ? GHUTRA.length : COVERINGS.has(top) ? COVER.length : HAIR.length;
  const band = draw(s, 4, CLOTH.length);
  return {
    skin: draw(s, 0, SKIN.length),
    top,
    topColor: draw(s, 2, colors),
    bg: draw(s, 3, BG.length),
    band,
    thread: (band + 1 + draw(s, 5, CLOTH.length - 1)) % CLOTH.length,
    motif: draw(s, 6, MOTIFS.length),
  };
}

export const SYSTEMS = {
  portrait: { name: "A · Portrait", features: portrait },
  token: { name: "B · Token", features: token },
  sadu: { name: "C · Sadu bust", features: sadu },
} as const;
export type SystemId = keyof typeof SYSTEMS;

/** Features a viewer can tell apart at 24px: the big colour fields and the silhouette. */
export const LAB_SALIENT: Record<SystemId, readonly string[]> = {
  portrait: SALIENT,
  token: ["skin", "bg", "ring", "segments", "orbit"],
  sadu: ["skin", "top", "bg", "band", "thread", "motif"],
};
