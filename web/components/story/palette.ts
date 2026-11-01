/**
 * The scene's palette. The page tokens (ground, teal, ink) match globals.css; everything else exists
 * only in the 3D world and the stills. Chroma is a hierarchy: the thread first, then figures and the
 * hospital, then a muted city on a warm floor.
 */
export const COLOR = {
  ground: "#f6f3ee",
  floor: "#e8dcc6",
  road: "#90877a",
  kerb: "#f7f1e5",
  surface: "#fffdf8",
  stone: "#d8cbb4",
  recess: "#4d4a44",
  ink: "#0b2427",
  unknown: "#8b9598",
  unknownInk: "#f6f3ee",
  grey: "#98a19f",
  teal: "#0f4f4c",
  tealBright: "#04b8a6",
  thread: "#00c2ae",
  held: "#e8920a",
  differs: "#c8473b",
  hospital: "#fdfcf8",
  hospitalShade: "#dfe6e3",
  hospitalFrame: "#c3d3cf",
  glass: "#1c6f78",
  glassDeep: "#124f58",
  hospitalTeal: "#0a9c8e",
  forecourt: "#f2ebdc",
  clinic: "#efd9b8",
  clinicShade: "#d9b78f",
  awning: "#c4552f",
  lattice: "#7a4a2b",
  leaf: "#4f8f5a",
  leafDark: "#3a7a58",
  trunk: "#86653f",
} as const;

/** The city's blocks: muted, low to mid saturation, picked with the seeded `rand`. */
export const CITY = ["#cf9a7c", "#ddb595", "#b4c0a2", "#a8bac6", "#e3d2ac", "#f3ecdf", "#d4aa9e", "#dcc08a"] as const;
