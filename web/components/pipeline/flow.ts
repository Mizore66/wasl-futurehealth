import { initialState, next, type CaseProgress, type ReplayState } from "@/lib/replay/reducer";
import type { Outcome, ReaderRead, Replay, ReplayEvent, ScriptedCase } from "@/lib/replay/types";
import type { PersonSeed } from "@/lib/people/types";
import type { Track } from "./playback";

export type Mark = "queued" | "working" | "done" | "held" | "failed";

export type RowId = "received" | "matching" | "reading" | "gate" | "closed" | "review" | "open" | "ledger";

/** The eight stage rows of the case flow (pipeline-brief "The state vocabulary"), with the backend term small and muted. */
export const ROWS: { id: RowId; label: string; tech: string; branch?: boolean }[] = [
  { id: "received", label: "Received", tech: "HL7 v2 · FHIR R4" },
  { id: "matching", label: "Matching identity", tech: "Arabic and Latin names" },
  { id: "reading", label: "Reading the report", tech: "rules reader + Gemma 3 4B" },
  { id: "gate", label: "Safety gate", tech: "readers agree, p ≥ 0.99" },
  { id: "closed", label: "Closed with evidence", tech: "closure_decision", branch: true },
  { id: "review", label: "Waiting for two reviewers", tech: "needs_human", branch: true },
  { id: "open", label: "Kept open", tech: "not_closed", branch: true },
  { id: "ledger", label: "Recorded in the ledger", tech: "hash-chained" },
];

const OUTCOME_ROW: Record<Outcome, RowId> = {
  closed_with_evidence: "closed",
  waiting_for_reviewers: "review",
  kept_open: "open",
};

/**
 * Which row a case sits in, and the shape of its mark there. A case stays in its outcome row once recorded,
 * so the outcome rows add up over the loop; the ledger row counts recordings instead of holding chips.
 */
export function place(p: CaseProgress): { row: RowId; mark: Mark } {
  switch (p.phase) {
    case "received":
      return { row: "received", mark: "queued" };
    case "matching":
      return { row: "matching", mark: "working" };
    case "matched":
      return { row: "matching", mark: "done" };
    case "reading":
      return { row: "reading", mark: "working" };
    case "read":
      return { row: "reading", mark: "done" };
    case "gate":
      return { row: "gate", mark: p.gatePassed ? "done" : "held" };
    case "outcome":
    case "recorded":
      return { row: OUTCOME_ROW[p.outcome!], mark: p.outcome === "closed_with_evidence" ? "done" : "held" };
  }
}

export const SLOTS = 4;

/** What a chip's avatar shows: "?" until the report is matched, the joining moment, then the face. */
export type ChipFace = "unknown" | "resolving" | "known";

export type Chip = {
  id: string;
  who: Who;
  mark: Mark;
  active: boolean;
  /** The two readers: both at work, or both answered. Only while the case sits in the reading row. */
  readers?: "reading" | "read";
};

function faceOf(p: CaseProgress): ChipFace {
  if (p.phase === "received" || p.phase === "matching") return "unknown";
  return p.phase === "matched" ? "resolving" : "known";
}
export type FlowRow = (typeof ROWS)[number] & { count: number; chips: Chip[]; more: number };

/** Who a chip or line is about, as the system knows them at that moment: the report's own spelling and "?" until it is matched. */
export type Who = { name: string; seed: PersonSeed; face: ChipFace };

export type LedgerLine = { who: Who; closed: boolean } | null;

export type FeedLine = { key: number; who: Who; text: string; tech: string; mark: Mark; step: number };

export type FlowView = {
  rows: FlowRow[];
  feed: FeedLine[];
  activeCase: string | null;
  /** The case most recently recorded in the ledger. */
  latestRecorded: LedgerLine;
  step: number;
  steps: number;
  decisions: number;
  chains: number;
};

const seedOf = (c: ScriptedCase): PersonSeed => ({ key: c.person.key, sex: c.person.sex });

function who(c: ScriptedCase, face: ChipFace): Who {
  return { name: face === "unknown" ? c.person.reportName : c.person.name, seed: seedOf(c), face };
}

const EVENT_FACE: Partial<Record<ReplayEvent["type"], ChipFace>> = { received: "unknown", matching: "unknown" };

const PREP_WORD: Record<string, string> = { adequate: "prep adequate", inadequate: "prep inadequate", not_stated: "prep not stated" };
const CAECUM_WORD: Record<string, string> = { yes: "caecum reached", no: "caecum not reached", not_stated: "caecum not stated" };

/** The reader's answer in plain words, from the ledger fields. */
export function readWords(r: ReaderRead): string {
  return `${CAECUM_WORD[r.caecumReached] ?? `caecum ${r.caecumReached}`}, ${PREP_WORD[r.prep] ?? `prep ${r.prep}`}`;
}

const OUTCOME_LINE: Record<Outcome, { text: string; mark: Mark }> = {
  closed_with_evidence: { text: "Closed with evidence", mark: "done" },
  waiting_for_reviewers: { text: "Held · waiting for two reviewers", mark: "held" },
  kept_open: { text: "Kept open · not complete yet", mark: "held" },
};

/** A decision feed line for the events that report a result. Start-of-work events stay off the feed. */
function line(e: ReplayEvent, c: ScriptedCase): Omit<FeedLine, "key" | "who" | "step"> | null {
  const verdict = `${c.read.state} · ${c.read.reason}`;
  switch (e.type) {
    case "received":
      return { text: "Received · colonoscopy report", tech: c.source === "hl7" ? "HL7 v2" : "FHIR R4", mark: "queued" };
    case "matched":
      return { text: "Matched to a referral", tech: "link_decision", mark: "done" };
    case "read":
      return { text: `Read · ${readWords(c.read)}`, tech: `${verdict} · ${(c.read.latencyMs / 1000).toFixed(1)} s`, mark: "done" };
    case "gate":
      return e.passed
        ? { text: "Safety gate passed", tech: verdict, mark: "done" }
        : { text: "Safety gate held · it will not guess", tech: verdict, mark: "held" };
    case "outcome":
      return { ...OUTCOME_LINE[e.outcome], tech: e.outcome === "closed_with_evidence" ? "closure_decision" : verdict };
    case "recorded":
      return {
        text: "Recorded in the ledger",
        tech: c.outcome === "closed_with_evidence" ? "link_decision → evidence → closure_decision" : "decision recorded",
        mark: "done",
      };
    default:
      return null;
  }
}

export const FEED_ROWS = 6;

export function flowView(replay: Replay, state: ReplayState): FlowView {
  const byId = new Map(replay.cases.map((c) => [c.id, c]));
  const played = replay.events.slice(0, state.cursor);
  const lastMoved = new Map<string, number>();
  played.forEach((e, i) => "caseId" in e && lastMoved.set(e.caseId, i));
  const last = played.at(-1);
  const activeCase = last && "caseId" in last ? last.caseId : null;

  const placed = state.order.map((id) => ({ id, ...place(state.cases[id]) }));
  const recorded = state.order.filter((id) => state.cases[id].phase === "recorded");
  const lastRecorded = played.findLast((e) => e.type === "recorded");
  const rows = ROWS.map((r) => {
    if (r.id === "ledger") return { ...r, count: recorded.length, chips: [], more: 0 };
    const here = placed.filter((p) => p.row === r.id).sort((a, b) => lastMoved.get(b.id)! - lastMoved.get(a.id)!);
    const shown = here.length > SLOTS ? here.slice(0, SLOTS - 1) : here;
    return {
      ...r,
      count: here.length,
      chips: shown.map((p) => {
        const progress = state.cases[p.id];
        return {
          id: p.id,
          who: who(byId.get(p.id)!, faceOf(progress)),
          mark: p.mark,
          active: p.id === activeCase,
          readers: progress.phase === "reading" ? "reading" : progress.phase === "read" ? "read" : undefined,
        } satisfies Chip;
      }),
      more: here.length - shown.length,
    };
  });

  const feed: FeedLine[] = [];
  for (let i = played.length - 1; i >= 0 && feed.length < FEED_ROWS; i--) {
    const e = played[i];
    if (!("caseId" in e)) continue;
    const c = byId.get(e.caseId)!;
    const l = line(e, c);
    if (l) feed.push({ ...l, key: i, who: who(c, EVENT_FACE[e.type] ?? "known"), step: i + 1 });
  }

  return {
    rows,
    feed,
    activeCase,
    latestRecorded:
      lastRecorded && "caseId" in lastRecorded
        ? { who: who(byId.get(lastRecorded.caseId)!, "known"), closed: byId.get(lastRecorded.caseId)!.outcome === "closed_with_evidence" }
        : null,
    step: state.cursor,
    steps: replay.events.length - 1,
    decisions: state.counters.outputs,
    chains: state.counters.chainsVerified,
  };
}

/** The resting still: the first frame of the loop with the most stage rows occupied, so a screenshot shows the whole flow. */
export function restFrame(replay: Replay): ReplayState {
  let best = initialState;
  let bestScore = -1;
  let s = initialState;
  for (let i = 0; i < replay.events.length - 1; i++) {
    s = next(replay, s);
    const occupied = new Set(s.order.map((id) => place(s.cases[id]).row)).size;
    if (occupied > bestScore) {
      best = s;
      bestScore = occupied;
    }
  }
  return best;
}

export function replayTrack(replay: Replay): Track<ReplayState> {
  return { rest: restFrame(replay), advance: (s) => next(replay, s) };
}
