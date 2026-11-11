import type { DoubleReadView, DoubleReviewEvent, DoubleReviewPhase, DoubleReviewRound, Programme, ReadName } from "@/lib/api/schema";

/*
 * The double-review state of one review item, per read role, as one seat may see it. Pure: the API decides what is
 * hidden and what this seat may submit (`viewer_may`); this only turns that into what the panel shows. Two guards are
 * repeated here so a malformed or older response can never open the wrong form: no senior form before a
 * disagreement, and no initial-read form once both initial reads are in.
 */

export const SEAT_LABEL: Record<ReadName, string> = { r1: "First reader", r2: "Second reader", r3: "Senior resolver" };
export const READ_LABEL: Record<ReadName, string> = { r1: "First read (r1)", r2: "Second read (r2)", r3: "Senior read (r3)" };

export type Phase = DoubleReviewPhase | "unknown";

export const PHASE_WORDS: Record<Phase, { pill: string; tone: "positive" | "negative" | "warning" | "muted" }> = {
  awaiting_reads: { pill: "Waiting for both initial reads", tone: "muted" },
  agreed: { pill: "Initial reads agree", tone: "positive" },
  disagreement: { pill: "Initial reads disagree: senior read open", tone: "warning" },
  resolved: { pill: "Resolved by a senior read", tone: "positive" },
  unknown: { pill: "State not recognised", tone: "muted" },
};

export function phaseOf(raw: string): Phase {
  return raw === "awaiting_reads" || raw === "agreed" || raw === "disagreement" || raw === "resolved" ? raw : "unknown";
}

export type ReadCell =
  | { read: ReadName; state: "empty" }
  | { read: ReadName; state: "hidden"; actor: string; role: string; at: string; hash: string }
  | { read: ReadName; state: "shown"; actor: string; role: string; at: string; hash: string; decision: string; note: string; mine: boolean };

export type Choice = { value: string; label: string };

export type RoundModel = {
  item: string;
  question: "link" | "evidence" | "other";
  phase: Phase;
  head: string;
  final?: string;
  /** r1 and r2 always; r3 only once a disagreement opened it. */
  cells: ReadCell[];
  /** The read this seat can submit now, with its answers; absent when the seat has nothing to do. */
  form?: { read: ReadName; senior: boolean; choices: Choice[] };
  /** Why there is no form, in a sentence. */
  why?: string;
};

const LINK_CHOICES: Choice[] = [
  { value: "same", label: "Same patient" },
  { value: "different", label: "Not the same patient" },
];
const CRC_EVIDENCE_CHOICES: Choice[] = [
  { value: "adequate", label: "Report adequate" },
  { value: "inadequate", label: "Report not adequate" },
];

/** The answers the panel offers. Other programmes' evidence labels exist in the API but have no form here yet. */
export function choicesFor(question: RoundModel["question"], programme: Programme): Choice[] {
  if (question === "link") return LINK_CHOICES;
  if (question === "evidence" && programme === "crc") return CRC_EVIDENCE_CHOICES;
  return [];
}

const same = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase();

function cellOf(read: ReadName, v: DoubleReadView | null | undefined, viewer: string): ReadCell {
  if (!v) return { read, state: "empty" };
  const who = { actor: v.actor, role: v.role, at: v.at, hash: v.hash };
  if (v.blinded || v.decision === undefined) return { read, state: "hidden", ...who };
  return { read, state: "shown", ...who, decision: v.decision, note: v.note ?? "", mine: viewer !== "" && same(v.actor, viewer) };
}

export function roundModel(round: DoubleReviewRound, seat: ReadName, viewerActor: string, programme: Programme): RoundModel {
  const phase = phaseOf(round.phase);
  const question = round.question === "link" || round.question === "evidence" ? round.question : "other";
  const cells: ReadCell[] = [cellOf("r1", round.reads.r1, viewerActor), cellOf("r2", round.reads.r2, viewerActor)];
  if (phase === "disagreement" || phase === "resolved") cells.push(cellOf("r3", round.reads.r3, viewerActor));
  const model: RoundModel = { item: round.item, question, phase, head: round.head, final: round.final, cells };

  const senior = seat === "r3";
  const stateAllows = senior ? phase === "disagreement" : phase === "awaiting_reads";
  const choices = choicesFor(question, programme);
  if (stateAllows && round.viewer_may.includes(seat) && choices.length > 0) {
    model.form = { read: seat, senior, choices };
    return model;
  }
  model.why = whyNot(round, phase, seat, viewerActor, choices.length > 0);
  return model;
}

function whyNot(round: DoubleReviewRound, phase: Phase, seat: ReadName, viewer: string, hasChoices: boolean): string {
  const own = round.reads[seat];
  if (phase === "unknown") return "The API returned a state this page does not recognise, so no answer is offered.";
  if (phase === "resolved") return "Settled by the senior read. Every read stays in the review log.";
  if (phase === "agreed") return "The two initial reads agree, so no senior read is needed.";
  if (seat === "r3") {
    if (phase === "awaiting_reads") return "Senior resolution opens only if the two initial reads disagree.";
    const initial = [round.reads.r1, round.reads.r2].find((r) => r && same(r.actor, viewer));
    if (initial) return `This seat's credential is bound to ${viewer}, who gave ${initial.read}. The senior read must come from a third person.`;
    return "This seat's credential may not give the senior read for this question.";
  }
  if (phase === "disagreement") return "Both initial reads are in. Only the senior resolver can answer now.";
  if (own) return same(own.actor, viewer) ? `You gave ${seat} as ${own.actor}.` : `${seat} was given by ${own.actor}.`;
  const other = round.reads[seat === "r1" ? "r2" : "r1"];
  if (other && viewer && same(other.actor, viewer)) {
    return `This seat's credential is bound to ${viewer}, who already gave ${other.read}. One person cannot give both initial reads.`;
  }
  if (!hasChoices) return "This programme's answer labels are accepted by the API but have no form on this page yet.";
  return "This seat's credential may not answer this question.";
}

/** The double-review reads in a case's ledger, grouped by item in ledger order (case detail `api_reviews`). */
export function readsByItem<E extends Pick<DoubleReviewEvent, "item" | "kind">>(events: E[]): Map<string, E[]> {
  const out = new Map<string, E[]>();
  for (const e of events) {
    if (e.kind !== "read") continue;
    out.set(e.item, [...(out.get(e.item) ?? []), e]);
  }
  return out;
}

/** Plain words for the API's refusal codes. */
export function refusalWords(f: { status: number; error: string; reason?: string; conflict?: string }): string {
  if (f.status === 403 && f.reason === "same_actor") return `Not saved: the API refused because this credential belongs to someone who already read this item. ${f.error}.`;
  if (f.status === 403) return `Not saved: this seat's credential is not allowed to answer this question (${f.error}).`;
  if (f.status === 409 && f.conflict === "head") return "Not saved: someone else read this item since the page loaded. The page now shows the current state; answer again.";
  if (f.status === 409 && f.conflict === "read") return "Not saved: this read was already given by someone else.";
  if (f.status === 409 && f.conflict === "phase") return "Not saved: the round has moved on (it is no longer open for this read).";
  if (f.status === 404) return "Not saved: this case has no such review item.";
  if (f.status === 0) return `Not saved: ${f.error}`;
  return `Not saved: ${f.error}`;
}
