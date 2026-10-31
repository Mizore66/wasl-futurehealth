/** How true a thing on screen is. Every route renders at least one. */
export const HONESTY = {
  built: "Built",
  simulated: "Simulated",
  designed: "Designed, not built",
  replay: "Replay of a recorded run",
  illustrative: "Illustrative",
} as const;

export type HonestyKind = keyof typeof HONESTY;
