import { CYLINDER, cylinderFigure } from "@/components/story/figure-geometry";

/** Every option is a straight cylinder with a Lego head; they differ only in proportions. */
export const SILHOUETTES = [
  { id: "A", name: "Even: body a little wider than the head", shape: CYLINDER },
  { id: "B", name: "Slim: tall body, head as wide as it", shape: cylinderFigure(0.185, 0.76, 0.19, 0.36) },
  { id: "C", name: "Stocky: short wide body, big head", shape: cylinderFigure(0.29, 0.5, 0.225, 0.42) },
  { id: "D", name: "Column: tall body, wider than the head", shape: cylinderFigure(0.24, 0.8, 0.19, 0.36) },
] as const;
