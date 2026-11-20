/*
 * The one chart palette for the staff app (light mode only; chart surface limestone-50 #FFFEF8).
 * Derived from the jade & limestone tokens plus ochre and clay, then snapped to steps that pass the dataviz
 * validator: `validate_palette.js "#0e7a5a,#c2851a,#5a5fb0,#b5503a" --mode light --surface "#FFFEF8"` passes
 * every check (worst adjacent CVD ΔE 12.2, normal-vision ΔE 22.5, all >= 3:1); the first three also pass `--pairs all`.
 * Slots are assigned in this order and never cycled. Text never wears these colours: values and labels use ink tokens.
 */
export const CATEGORICAL = ["#0e7a5a", "#c2851a", "#5a5fb0", "#b5503a"] as const;

/** Roles used across the evidence charts. Colour follows the entity: Wasl is always jade, a comparator always ochre. */
export const SERIES = {
  /** Wasl, and the held-out (v2) evaluation, which is Wasl's honest number. */
  wasl: CATEGORICAL[0],
  /** Whatever Wasl is compared against: Emirates-ID-only matching, a published baseline, one reviewer. */
  comparator: CATEGORICAL[1],
  /** The v1 noise model Wasl was tuned on. */
  tuned: CATEGORICAL[2],
  clay: CATEGORICAL[3],
} as const;

/** Chart furniture: recessive hairlines, a de-emphasis gray, and the ink used for a reference or the truth. */
export const CHROME = {
  grid: "var(--color-limestone-300)",
  baseline: "var(--color-limestone-400)",
  track: "var(--color-limestone-200)",
  deemphasis: "var(--color-limestone-500)",
  reference: "var(--color-limestone-900)",
  surface: "var(--color-limestone-50)",
} as const;
