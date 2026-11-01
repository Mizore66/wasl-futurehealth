/* data/reader.json, built by scripts/build-reader.ts. */

export type ReaderField = "caecum" | "prep" | "polyps" | "histology";

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
