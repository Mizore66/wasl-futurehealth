import type { DoubleReadView, DoubleReviewEvent, DoubleReviewPhase, DoubleReviewRound, DoubleReviewState, Programme, ReadName } from "@/lib/api/schema";

/*
 * The double-review state of one review item, per read role, as THIS deployment's one credential may see it. The web
 * has no per-person sign-in, so a deployment is one reader per question role (lib/api/double-review.ts): there are no
 * seats to switch between. Pure: the API decides what is hidden and which read this credential may submit
 * (`viewer_may`); this only turns that into what the panel shows. Guards are repeated here so a malformed, older or
 * misconfigured response can never put another person's blinded answer on the page or open the wrong form:
 * `ownView` drops every answer that is not this deployment's own while a round awaits its initial reads, there is no
 * senior form before a disagreement, and no initial-read form once both initial reads are in.
 */

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

/** Phases in which the API reveals both initial answers to every queue reader. Anything else is treated as blinded. */
const revealed = (phase: Phase) => phase === "agreed" || phase === "disagreement" || phase === "resolved";

const same = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase();

/** Whether a read was given by this deployment's own actor (compared as the API does: trimmed, case-insensitive). */
export const isOwn = (actor: string, viewer: string) => viewer.trim() !== "" && same(actor, viewer);

function readView(v: DoubleReadView | null | undefined, phase: Phase, viewer: string): DoubleReadView | null {
  if (!v) return null;
  const who = { read: v.read, actor: v.actor, role: v.role, at: v.at, hash: v.hash };
  if (v.blinded || v.decision === undefined || (!revealed(phase) && !isOwn(v.actor, viewer))) return { ...who, blinded: true };
  return { ...who, blinded: false, decision: v.decision, note: v.note ?? "" };
}

/**
 * The state as this deployment may render it, rebuilt from known fields only (the response schema is loose). While a
 * round is not yet revealed, every read whose author is not this deployment's actor loses its decision and note, and
 * the round loses any `final`, whatever the API sent. Only this result is rendered, so neither the HTML nor the RSC
 * payload can carry another person's blinded answer.
 */
export function ownView(state: DoubleReviewState): DoubleReviewState {
  const viewer = state.viewer.actor;
  return {
    protocol: state.protocol,
    case: state.case,
    viewer: { role: state.viewer.role, actor: viewer },
    rounds: state.rounds.map((r) => {
      const phase = phaseOf(r.phase);
      return {
        item: r.item,
        question: r.question,
        action: r.action,
        phase: r.phase,
        head: r.head,
        seq: r.seq,
        r3_open: r.r3_open,
        viewer_may: r.viewer_may.filter((x) => x === "r1" || x === "r2" || x === "r3"),
        ...(revealed(phase) && r.final !== undefined ? { final: r.final } : {}),
        reads: { r1: readView(r.reads.r1, phase, viewer), r2: readView(r.reads.r2, phase, viewer), r3: readView(r.reads.r3, phase, viewer) },
      };
    }),
  };
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
  /** The read this deployment can submit now (the slot the API offers), with its answers; absent when there is nothing to do. */
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

/**
 * The slot this deployment's credential would fill now, as the API offers it (`viewer_may`), or undefined. Initial:
 * the first open initial slot, only while the round awaits its initial reads. Senior: r3, only in a disagreement.
 */
export function slotFor(round: Pick<DoubleReviewRound, "phase" | "viewer_may" | "reads">, senior: boolean): ReadName | undefined {
  const phase = phaseOf(round.phase);
  if (senior) return phase === "disagreement" && round.viewer_may.includes("r3") ? "r3" : undefined;
  if (phase !== "awaiting_reads") return undefined;
  return (["r1", "r2"] as const).find((r) => !round.reads[r] && round.viewer_may.includes(r));
}

function cellOf(read: ReadName, v: DoubleReadView | null | undefined, viewer: string): ReadCell {
  if (!v) return { read, state: "empty" };
  const who = { actor: v.actor, role: v.role, at: v.at, hash: v.hash };
  if (v.blinded || v.decision === undefined) return { read, state: "hidden", ...who };
  return { read, state: "shown", ...who, decision: v.decision, note: v.note ?? "", mine: isOwn(v.actor, viewer) };
}

/** `round` should come from `ownView`; the model applies the same blinding rule again in case it does not. */
export function roundModel(round: DoubleReviewRound, viewerActor: string, programme: Programme): RoundModel {
  const phase = phaseOf(round.phase);
  const question = round.question === "link" || round.question === "evidence" ? round.question : "other";
  const view = (r: ReadName) => readView(round.reads[r], phase, viewerActor);
  const cells: ReadCell[] = [cellOf("r1", view("r1"), viewerActor), cellOf("r2", view("r2"), viewerActor)];
  if (phase === "disagreement" || phase === "resolved") cells.push(cellOf("r3", view("r3"), viewerActor));
  const model: RoundModel = { item: round.item, question, phase, head: round.head, final: revealed(phase) ? round.final : undefined, cells };

  const choices = choicesFor(question, programme);
  const read = slotFor(round, phase === "disagreement");
  if (read && choices.length > 0) {
    model.form = { read, senior: read === "r3", choices };
    return model;
  }
  model.why = whyNot(round, phase, viewerActor, choices.length > 0);
  return model;
}

function whyNot(round: DoubleReviewRound, phase: Phase, viewer: string, hasChoices: boolean): string {
  if (phase === "unknown") return "The API returned a state this page does not recognise, so no answer is offered.";
  if (phase === "resolved") return "Settled by the senior read. Every read stays in the review log.";
  if (phase === "agreed") return "The two initial reads agree, so no senior read is needed.";
  const mine = [round.reads.r1, round.reads.r2].find((r) => r && isOwn(r.actor, viewer));
  if (phase === "disagreement") {
    if (mine) return `This deployment gave ${mine.read} as ${viewer}. The senior read must come from a third person.`;
    return "This deployment's credential may not give the senior read for this question.";
  }
  if (mine) return `You gave ${mine.read} as ${mine.actor}. The other initial read must come from a different person.`;
  if (!hasChoices) return "This programme's answer labels are accepted by the API but have no form on this page yet.";
  return "This deployment's credential may not answer this question.";
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
  if (f.status === 403 && f.reason === "same_actor") {
    return `Not saved: the API refused because this deployment's credential belongs to someone who already read this item. ${f.error}.`;
  }
  if (f.status === 403) return `Not saved: this deployment's credential is not allowed to answer this question (${f.error}).`;
  if (f.status === 409 && f.conflict === "head") return "Not saved: someone else read this item since the page loaded. The page now shows the current state; answer again.";
  if (f.status === 409 && f.conflict === "read") return "Not saved: this read was already given by someone else.";
  if (f.status === 409 && f.conflict === "phase") return "Not saved: the round has moved on (it is no longer open for this read).";
  if (f.status === 404) return "Not saved: this case has no such review item.";
  return `Not saved: ${f.error}`;
}
