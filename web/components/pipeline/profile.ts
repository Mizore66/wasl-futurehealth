import { caseHref } from "@/lib/domain/ids";
import { personFor } from "@/lib/people";
import type { ReplayState } from "@/lib/replay/reducer";
import type { Replay, ReplayEvent, ReplayPerson } from "@/lib/replay/types";
import { line, OUTCOME_LINE, place, ROWS } from "./flow";
import type { ReaderAnswer } from "./reader-types";

/** The rules reader's answer for a report, where data/reader.json has the same report text. Keyed by report sha256. */
export type RulesBySha = Record<string, ReaderAnswer>;

export type ProfileField = { name: string; key: string; value: string; quote: string; rules?: string };

/** One person in the replay, as the profile sheet shows them: who, where now, what happened to their report, and the decision. */
export type Profile = {
  caseId: string;
  person: ReplayPerson;
  /** Where the person's report sits in the current frame, or when it arrives. */
  now: string;
  /** Whether the report is matched to its referral in the current frame; until then the face shows "?". */
  matched: boolean;
  history: { step: number; text: string; tech: string; played: boolean }[];
  fields: ProfileField[];
  latencyS: string;
  model: string;
  /** Whether the rules reader's answer for this report is on record (only the demo cast's reports). */
  rulesOnRecord: boolean;
  decision: { word: string; label: string; gatePassed: boolean };
  /** The patient record, for a demo cast member the API also serves. Generated people have none. */
  record: string | null;
};

/** The ledger keeps punctuation fragments where the reader quoted nothing useful; those show as no quote. */
const quoteOf = (q: string) => (/[A-Za-z]{3}/.test(q) ? q.trim() : "");

/** Start-of-work events the decision feed leaves out; the profile lists every step. */
const START_LINE: Partial<Record<ReplayEvent["type"], { text: string; tech: string }>> = {
  matching: { text: "Matching identity", tech: "Arabic and Latin names" },
  reading: { text: "Reading the report", tech: "rules reader + Gemma 3 4B" },
};

export function profileOf(replay: Replay, state: ReplayState, caseId: string, rules: RulesBySha): Profile | null {
  const c = replay.cases.find((x) => x.id === caseId);
  if (!c) return null;

  const history = replay.events.flatMap((e, i) => {
    if (!("caseId" in e) || e.caseId !== caseId) return [];
    const l = line(e, c) ?? START_LINE[e.type];
    return l ? [{ step: i + 1, text: l.text, tech: l.tech, played: i < state.cursor }] : [];
  });

  const progress = state.cases[caseId];
  const now = progress
    ? (ROWS.find((r) => r.id === place(progress).row)?.label ?? progress.phase)
    : `Not in this loop yet. The report arrives at step ${history[0]?.step ?? "?"}.`;

  const r = c.read;
  const rulesAnswer = rules[r.reportSha];
  const fields: ProfileField[] = [
    { name: "Caecum reached", key: "caecum_reached", value: r.caecumReached, quote: quoteOf(r.caecumQuote), rules: rulesAnswer?.caecum },
    { name: "Bowel prep graded", key: "prep", value: r.prep, quote: quoteOf(r.prepQuote), rules: rulesAnswer?.prep },
    { name: "Tissue taken", key: "tissue_taken", value: r.tissueTaken, quote: quoteOf(r.tissueQuote), rules: rulesAnswer?.tissue },
    { name: "Histology back", key: "histology_present", value: r.histologyPresent, quote: quoteOf(r.histologyQuote), rules: rulesAnswer?.histology },
  ];

  return {
    caseId,
    person: c.person,
    now,
    matched: !!progress && progress.phase !== "received" && progress.phase !== "matching",
    history,
    fields,
    latencyS: `${(r.latencyMs / 1000).toFixed(1)} s`,
    model: r.modelVersion.split("/")[0],
    rulesOnRecord: !!rulesAnswer,
    decision: { word: OUTCOME_LINE[c.outcome].text, label: `${r.state} · ${r.reason}`, gatePassed: c.gatePassed },
    record: recordOf(c.person),
  };
}

function recordOf(person: ReplayPerson): string | null {
  if (person.source !== "canonical-cast") return null;
  const loop = personFor(person.key)?.loop;
  return loop ? caseHref("/app/patients", loop) : null;
}
