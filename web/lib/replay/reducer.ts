import { COUNTERS, type CasePhase, type Counters, type Outcome, type Replay, type ReplayEvent } from "./types";

export type CaseProgress = { phase: CasePhase; gatePassed?: boolean; outcome?: Outcome };

export type ReplayState = {
  cursor: number;
  order: string[];
  cases: Record<string, CaseProgress>;
  counters: Counters;
};

const zero = Object.fromEntries(COUNTERS.map((k) => [k, 0])) as Counters;

export const initialState: ReplayState = { cursor: 0, order: [], cases: {}, counters: zero };

function addCounters(counters: Counters, add: Partial<Counters>): Counters {
  const next = { ...counters };
  for (const k of COUNTERS) next[k] += add[k] ?? 0;
  return next;
}

/** Applies one event. Pure; the UI owns the timer. */
export function step(state: ReplayState, event: ReplayEvent): ReplayState {
  if (event.type === "loop_end") return initialState;

  const prev = state.cases[event.caseId];
  const progress: CaseProgress = { ...prev, phase: event.type };
  if (event.type === "gate") progress.gatePassed = event.passed;
  if (event.type === "outcome") progress.outcome = event.outcome;

  return {
    cursor: state.cursor,
    order: prev ? state.order : [...state.order, event.caseId],
    cases: { ...state.cases, [event.caseId]: progress },
    counters: "add" in event ? addCounters(state.counters, event.add) : state.counters,
  };
}

/** Applies the event at the cursor and advances it; the loop's last event returns to state 0. */
export function next(replay: Replay, state: ReplayState): ReplayState {
  const event = replay.events[state.cursor];
  const after = step(state, event);
  return event.type === "loop_end" ? after : { ...after, cursor: state.cursor + 1 };
}
