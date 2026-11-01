/* data/reader.json, built by scripts/build-reader.ts. */

export type ReaderField = "caecum" | "prep" | "tissue" | "histology";

/** A field the rules reader in reports43.json was never asked: it answered "polyp removed", not "tissue taken". */
export const NOT_ASKED = "not_asked";

export type ReaderSegment = { text: string; field?: ReaderField };

export type ReaderAnswer = Record<ReaderField, string> & { state: string; reason: string };

export type ReaderCase = {
  /** MTSamples row id in data/evidence/reports43.json. */
  uid: string;
  title: string;
  sha: string;
  /** A fixture case whose recorded evidence quotes this same text, if any. */
  standsFor?: { loop: string; report: string };
  segments: ReaderSegment[];
  rules: ReaderAnswer;
  ai: ReaderAnswer & { downgraded: ReaderField[]; modelVersion: string; latencyMs: number };
};

export type ReaderData = { label: "replay"; source: { reports: string; ledger: string }; cases: ReaderCase[] };
