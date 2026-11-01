import type { PersonSeed } from "@/components/avatar/traits";

/**
 * The landing story as tables. One scroll value `p` (0 to 1) drives everything: the beats name the
 * stretches of the road, the copy table says which single line of text owns each stretch, and the
 * motion functions say where Fatima, the thread tip and the receipt are. The stage, the DOM copy, the
 * stills and the tests all read these.
 */

export const BEATS = [
  { id: "clinic", from: 0, to: 0.15 },
  { id: "city", from: 0.15, to: 0.335 },
  { id: "door", from: 0.335, to: 0.42 },
  { id: "gap", from: 0.42, to: 0.545 },
  { id: "thread", from: 0.545, to: 0.64 },
  { id: "join", from: 0.64, to: 0.748 },
  { id: "proof", from: 0.748, to: 0.83 },
  { id: "home", from: 0.83, to: 0.91 },
  { id: "end", from: 0.91, to: 1 },
] as const;

export type BeatId = (typeof BEATS)[number]["id"];

export function beatAt(p: number): BeatId {
  return (BEATS.find((b) => p < b.to) ?? BEATS[BEATS.length - 1]).id;
}

/** The track is this many viewport heights tall. */
export const TRACK_VH = { desktop: 1300, mobile: 1050 } as const;

/** Things in the world a line of copy can point at. The stage projects each once per resize. */
export type AnchorId = "fatima-door" | "clinic-door" | "walker" | "slab" | "report" | "nurse" | "gap" | "tip" | "receipt";

/**
 * Where a line sits. Zones are fixed screen regions; `anchor` sits beside a projected world point
 * while the camera holds still around it.
 */
export type Place = { zone: "tl" | "tc" | "c" | "bl" | "br" | "bc" | "r" } | { anchor: AnchorId };

export type TextId =
  | "hero"
  | "her-test"
  | "her-clinic"
  | "booked"
  | "wait"
  | "written"
  | "nobody-knew"
  | "calling"
  | "never-hears"
  | "missed"
  | "wasl"
  | "follows"
  | "spellings"
  | "join"
  | "quote"
  | "none-wrong"
  | "receipt-word"
  | "receipt"
  | "who"
  | "close";

export interface TextCue {
  id: TextId;
  from: number;
  to: number;
  place: Place;
  /** On a phone every cue sits in a band, above or below the subject. */
  band: "top" | "bottom";
  /** The hero is on screen at load and the close stays to the end. */
  enter?: false;
  exit?: false;
}

/**
 * The copy, one cue at a time: no two windows overlap, so at most one line of story text is on
 * screen. Windows sit inside the stretch whose action they describe.
 */
export const CUES: readonly TextCue[] = [
  { id: "hero", from: 0, to: 0.06, place: { zone: "tl" }, band: "bottom", enter: false },
  { id: "her-test", from: 0.066, to: 0.104, place: { anchor: "fatima-door" }, band: "bottom" },
  { id: "her-clinic", from: 0.11, to: 0.148, place: { anchor: "clinic-door" }, band: "bottom" },
  { id: "booked", from: 0.158, to: 0.205, place: { anchor: "walker" }, band: "bottom" },
  { id: "wait", from: 0.215, to: 0.3, place: { zone: "br" }, band: "bottom" },
  { id: "written", from: 0.34, to: 0.374, place: { anchor: "slab" }, band: "bottom" },
  { id: "nobody-knew", from: 0.38, to: 0.415, place: { zone: "c" }, band: "top" },
  { id: "calling", from: 0.44, to: 0.47, place: { anchor: "nurse" }, band: "bottom" },
  { id: "never-hears", from: 0.475, to: 0.502, place: { anchor: "gap" }, band: "bottom" },
  { id: "missed", from: 0.507, to: 0.542, place: { zone: "bl" }, band: "bottom" },
  { id: "wasl", from: 0.552, to: 0.592, place: { zone: "tl" }, band: "top" },
  { id: "follows", from: 0.597, to: 0.634, place: { anchor: "tip" }, band: "bottom" },
  { id: "spellings", from: 0.64, to: 0.664, place: { zone: "tc" }, band: "top" },
  { id: "join", from: 0.668, to: 0.746, place: { zone: "tc" }, band: "bottom" },
  { id: "quote", from: 0.752, to: 0.786, place: { anchor: "report" }, band: "bottom" },
  { id: "none-wrong", from: 0.791, to: 0.826, place: { zone: "br" }, band: "bottom" },
  { id: "receipt-word", from: 0.832, to: 0.866, place: { anchor: "receipt" }, band: "bottom" },
  { id: "receipt", from: 0.872, to: 0.908, place: { zone: "r" }, band: "bottom" },
  { id: "who", from: 0.916, to: 0.952, place: { zone: "tl" }, band: "bottom" },
  { id: "close", from: 0.958, to: 1, place: { zone: "tc" }, band: "bottom", exit: false },
];

export const clamp01 = (x: number) => Math.min(1, Math.max(0, x));
export const span = (s: number, a: number, b: number) => clamp01((s - a) / (b - a));
export const ease = (t: number) => t * t * (3 - 2 * t);
export const smoother = (t: number) => t * t * t * (t * (t * 6 - 15) + 10);

/**
 * How far a cue is in, for scroll position `p`. `reveal` runs 0 to 1 over the first 30% of the window
 * (word by word); `fade` is 1 while shown and falls to 0 over the last 15%. Outside the window both are 0.
 */
export function cueAt(c: TextCue, p: number): { reveal: number; fade: number } {
  if (p < c.from || (p > c.to && c.exit !== false)) return { reveal: 0, fade: 0 };
  const len = c.to - c.from;
  const reveal = c.enter === false ? 1 : span(p, c.from, c.from + len * 0.3);
  const fade = c.exit === false ? 1 : 1 - span(p, c.to - len * 0.15, c.to);
  return { reveal, fade };
}

/** The cue on screen at `p`, if any. */
export function cueOn(p: number): TextCue | undefined {
  return CUES.find((c) => cueAt(c, p).fade > 0);
}

/** Fatima's walk, as a share of the road (0 at her clinic door, 1 at the hospital slab). */
export const WALK = { from: 0.15, to: 0.335 } as const;
/** The thread's run along the same road, to the first ring at the hospital. */
export const RUN = { from: 0.556, to: 0.636 } as const;
/** The receipt's ride home, from the hospital back to the clinic window. */
export const RIDE = { from: 0.83, to: 0.872 } as const;
/** The join at the hospital door: name ring, birth-date ring, the face turns back. */
export const JOIN = { from: 0.672, to: 0.73 } as const;

export function walkAt(p: number): number {
  return smoother(span(p, WALK.from, WALK.to));
}

/**
 * The Day counter that runs during the walk. It counts the published city-wide median wait, not
 * Fatima's own: the copy beside it says so.
 */
export function dayAt(p: number, waitDays: number): number {
  return Math.round(walkAt(p) * waitDays);
}

/** Illustrative people for the hands-on join. Each has three possible endings. */
export type Outcome = "joined" | "apart" | "review";

export const CAST = {
  fatima: { first: "Fatima", seed: { key: "p-fatima-zaabi", sex: "female" }, outcome: "joined" },
  khalid: { first: "Khalid", seed: { key: "p-khalid-kitbi", sex: "male" }, outcome: "apart" },
  aisha: { first: "Aisha", seed: { key: "p-aisha-mazroui", sex: "female" }, outcome: "review" },
} as const satisfies Record<string, { first: string; seed: PersonSeed; outcome: Outcome }>;

export type CastKey = keyof typeof CAST;
export const CAST_ORDER: CastKey[] = ["fatima", "khalid", "aisha"];

/** The other made-up people: the nurse who keeps calling, and the man the hospital wrote down as "Khalid". */
export const SUPPORT = {
  nurse: { key: "p-nurse-mariam", sex: "female" },
  stranger: { key: "p-khalid-kaabi-hospital", sex: "male" },
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
