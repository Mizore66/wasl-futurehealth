/*
 * replay.json: a deterministic script over the recorded showcase run (pipeline/results/showcase).
 * Built by scripts/build-replay.ts. Counters tick toward the run's real totals and never past them.
 */

export const COUNTERS = ["fhirMessages", "readerReports", "outputs", "chainsVerified"] as const;
export type CounterKey = (typeof COUNTERS)[number];
export type Counters = Record<CounterKey, number>;

export type ReaderField = "yes" | "no" | "not_stated" | (string & {});

/** One report from reader_ledger.jsonl, as the live Gemma 3 4B reader answered it. */
export type ReaderRead = {
  reportSha: string;
  caecumReached: ReaderField;
  caecumQuote: string;
  prep: ReaderField;
  prepQuote: string;
  tissueTaken: ReaderField;
  tissueQuote: string;
  histologyPresent: ReaderField;
  histologyQuote: string;
  state: string;
  reason: string;
  modelVersion: string;
  latencyMs: number;
};

export type Outcome = "closed_with_evidence" | "waiting_for_reviewers" | "kept_open";

export type ScriptedCase = {
  id: string;
  source: "fhir" | "hl7";
  read: ReaderRead;
  gatePassed: boolean;
  outcome: Outcome;
};

export type ReplayEvent =
  | { type: "received"; caseId: string; add: Partial<Counters> }
  | { type: "matching"; caseId: string }
  | { type: "matched"; caseId: string }
  | { type: "reading"; caseId: string }
  | { type: "read"; caseId: string; add: Partial<Counters> }
  | { type: "gate"; caseId: string; passed: boolean }
  | { type: "outcome"; caseId: string; outcome: Outcome }
  | { type: "recorded"; caseId: string; add: Partial<Counters> }
  | { type: "loop_end" };

export type CasePhase = Exclude<ReplayEvent["type"], "loop_end">;

export type Replay = {
  label: "replay";
  source: { run: string; files: string[] };
  totals: Counters;
  facts: {
    loops: number;
    duplicates: number;
    lost: number;
    throughputEvS: number;
    forgedEvents: number;
    forgedWithEffect: number;
    tamperAttacks: number;
    tamperDetected: number;
    readerModel: string;
    readerStates: Record<string, number>;
  };
  cases: ScriptedCase[];
  events: ReplayEvent[];
};
