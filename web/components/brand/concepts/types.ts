import type { ReactNode } from "react";

/** A logo concept drawn on a 32-unit grid. `draw` paints the mark in one colour; the tile and lockup are derived. */
export type Concept = {
  id: "join" | "seal" | "sad" | "w";
  name: string;
  idea: string;
  draw: (ink: string) => ReactNode;
  /** Optional pixel-hinted master on a 16-unit grid, already inset for the tile. Used at 16px and below. */
  small?: (ink: string) => ReactNode;
};

export const GRID = 32;
