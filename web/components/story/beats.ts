import type { PersonSeed } from "@/components/avatar/traits";

/**
 * The landing story as one table. Each beat owns a slice of the scroll track (`from`..`to`, 0 to 1);
 * the DOM copy, the camera path, the 3D scene groups, the stills and the tests all read this table.
 */
export const BEATS = [
  { id: "lost", from: 0, to: 0.1 },
  { id: "unknown", from: 0.1, to: 0.24 },
  { id: "wait", from: 0.24, to: 0.32 },
  { id: "join", from: 0.32, to: 0.5 },
  { id: "proof", from: 0.5, to: 0.64 },
  { id: "receipt", from: 0.64, to: 0.76 },
  { id: "who", from: 0.76, to: 0.9 },
  { id: "start", from: 0.9, to: 1 },
] as const;

export type BeatId = (typeof BEATS)[number]["id"];

/** Scroll progress inside one beat, clamped to 0..1. */
export function local(p: number, id: BeatId): number {
  const b = BEATS.find((x) => x.id === id)!;
  return Math.min(1, Math.max(0, (p - b.from) / (b.to - b.from)));
}

export function beatAt(p: number): BeatId {
  return (BEATS.find((b) => p < b.to) ?? BEATS[BEATS.length - 1]).id;
}

/** The track is this many viewport heights tall; each beat's section gets its share. */
export const TRACK_VH = { desktop: 1100, mobile: 900 } as const;

/** Illustrative people for the live join. Fatima follows the canonical record; Aisha's two records are two people. */
export const CAST = {
  fatima: { first: "Fatima", seed: { key: "p-fatima-zaabi", sex: "female" }, joined: true },
  khalid: { first: "Khalid", seed: { key: "p-khalid-kitbi", sex: "male" }, joined: true },
  aisha: { first: "Aisha", seed: { key: "p-aisha-mazroui", sex: "female" }, joined: false },
} as const satisfies Record<string, { first: string; seed: PersonSeed; joined: boolean }>;

export type CastKey = keyof typeof CAST;
export const CAST_ORDER: CastKey[] = ["fatima", "khalid", "aisha"];

/** The other made-up people in the room: the nurse who keeps calling and the auditor. */
export const SUPPORT = {
  nurse: { key: "p-nurse-mariam", sex: "female" },
  auditor: { key: "p-auditor-omar", sex: "male" },
} as const satisfies Record<string, PersonSeed>;

export type FaceKey = CastKey | keyof typeof SUPPORT;
export const FACE_SEEDS: Record<FaceKey, PersonSeed> = {
  fatima: CAST.fatima.seed,
  khalid: CAST.khalid.seed,
  aisha: CAST.aisha.seed,
  ...SUPPORT,
};

/** Every number the story shows, read from results.json by the server page. */
export interface StoryFigures {
  /** Of five people who had their check, how many looked as if they had not under ID-only matching (rounded). */
  missedOfFive: number;
  waitDays: number;
  standardDays: number;
  closuresDone: number;
  closuresWrong: number;
}

type V3 = readonly [number, number, number];

/**
 * Camera keys along the whole track: where the camera stands and what it looks at. Between two keys
 * the camera eases (smootherstep); two keys with the same pose make a hold.
 */
export const CAMERA_KEYS: readonly { p: number; pos: V3; look: V3 }[] = [
  { p: 0, pos: [0, 9, 27], look: [0, 0, -4] },
  { p: 0.08, pos: [1.2, 2.6, 9], look: [0.7, 0.7, 0] },
  { p: 0.12, pos: [28.4, 4.4, 26], look: [28.4, 1, 0] },
  { p: 0.17, pos: [28.4, 4.2, 25], look: [28.4, 1, 0] },
  { p: 0.2, pos: [28.6, 5.6, -1.5], look: [28.6, 0.6, -22] },
  { p: 0.23, pos: [28.6, 5.4, -2], look: [28.6, 0.6, -22] },
  { p: 0.255, pos: [41, 3.4, -5], look: [43, 0.5, -20] },
  { p: 0.31, pos: [47, 3.4, -5], look: [47, 0.5, -20] },
  { p: 0.35, pos: [28.4, 3.6, 25], look: [28.4, 1.1, 0] },
  { p: 0.48, pos: [28.4, 3.2, 24], look: [28.4, 1.1, 0] },
  { p: 0.52, pos: [44.6, 3, 15], look: [44.6, 1.2, 0] },
  { p: 0.57, pos: [44.6, 2.8, 14.5], look: [44.6, 1.2, 0] },
  { p: 0.61, pos: [56, 3.6, 11], look: [56, 3.2, -3] },
  { p: 0.63, pos: [56, 3.6, 10.5], look: [56, 3.2, -3] },
  { p: 0.67, pos: [62, 2, 10], look: [62, 1.8, 0] },
  { p: 0.74, pos: [62, 2, 9.6], look: [62, 1.8, 0] },
  { p: 0.79, pos: [72, 11, 11], look: [72, 0, 0.4] },
  { p: 0.88, pos: [72, 10.5, 10.5], look: [72, 0, 0.4] },
  { p: 0.96, pos: [0, 11, 31], look: [0, 0, -4] },
  { p: 1, pos: [0, 11, 31], look: [0, 0, -4] },
];

/** Where the story's fixed props stand in the room (world units, y up). */
export const PLACES = {
  clinic: [23, 0, 0],
  ring1: [26.4, 0, 0],
  ring2: [29.6, 0, 0],
  hospital: [33, 0, 0],
  ledge: [28, 0, -22],
  beam: [44, 0, -20],
  readers: [44, 0, 0],
  wall: [56, 3.2, -3],
  receipt: [62, 1.9, 0],
  who: [72, 0, 0],
} as const satisfies Record<string, V3>;
