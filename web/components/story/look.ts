import { CLOTH, SKIN, portrait, topFill, type PersonSeed, type Top } from "@/components/avatar/traits";

/** What a figure wears: the same seed-derived features the 2D avatar draws, so one person has one identity. */
export interface FigureLook {
  skin: string;
  cloth: string;
  top: Top;
  topColor: string;
  eyes: number;
  mouth: number;
}

export function lookOf(seed: PersonSeed): FigureLook {
  const f = portrait(seed);
  return { skin: SKIN[f.skin], cloth: CLOTH[f.cloth], top: f.top, topColor: topFill(f.top, f.topColor), eyes: f.eyes, mouth: f.mouth };
}
