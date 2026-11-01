// Builds data/reader.json: three real MTSamples report texts with the rules reader's answer (reports43.json)
// and the live Gemma 3 4B reader's answer, quotes and latency (showcase reader_ledger.jsonl).
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import type { ReaderCase, ReaderData, ReaderField, ReaderSegment } from "../components/pipeline/reader-types";

const web = path.join(import.meta.dirname, "..");
const root = path.join(web, "..");
const REPORTS = "data/evidence/reports43.json";
const LEDGER = "pipeline/results/showcase/reader_ledger.jsonl";

/** The showcase's three stories: a complete exam, a scope that stopped early, a prep grade the AI would not vouch for. */
const PICKS = ["u31", "u6", "u43"] as const;

type Fields = Record<ReaderField, string>;
type Report = { uid: string; name: string; sha: string; text: string; regex: Fields & { state: string; reason: string } };
type LedgerRow = {
  report_sha: string;
  caecum_reached: string;
  caecum_quote: string;
  prep: string;
  prep_quote: string;
  polyps_removed: string;
  polyps_quote: string;
  histology_present: string;
  histology_quote: string;
  state: string;
  reason: string;
  model_version: string;
  latency_ms: number;
  downgraded?: string[];
};

const reports: Report[] = JSON.parse(readFileSync(path.join(root, REPORTS), "utf8")).reports;
const ledger: LedgerRow[] = readFileSync(path.join(root, LEDGER), "utf8")
  .trim()
  .split("\n")
  .map((l) => JSON.parse(l));

const apiDir = path.join(web, "data", "api");
const fixtureEvidence = readdirSync(apiDir)
  .filter((f) => f.startsWith("case_") && !f.endsWith("_audit.json"))
  .flatMap((f) => {
    const c = JSON.parse(readFileSync(path.join(apiDir, f), "utf8"));
    return (c.evidence as { quote: string; report: string }[]).map((e) => ({
      loop: c.case.id as string,
      report: e.report.split("/").at(-1)!,
      quote: e.quote,
    }));
  });

/** MTSamples flattens headings into ", HEADING: ," runs; restore plain sentences without changing a word. */
function clean(text: string): string {
  return text
    .replace(/\s*,\s*([A-Z][A-Z ]+:)/g, " $1")
    .replace(/:\s*,\s*/g, ": ")
    .replace(/\.,/g, ". ")
    .replace(/\s+/g, " ")
    .trim();
}

/**
 * Where the reader's quote sits in the report. The live reader sometimes truncates a quote or runs words
 * together, so take the longest prefix (at least 20 characters) found verbatim, ignoring case.
 */
function locate(text: string, quote: string): [number, number] | null {
  const lower = text.toLowerCase();
  const q = quote.replace(/\s+/g, " ").trim().toLowerCase();
  for (let n = q.length; n >= 20; n--) {
    const at = lower.indexOf(q.slice(0, n));
    if (at >= 0) return [at, at + n];
  }
  return null;
}

function sentenceStart(text: string, at: number): number {
  const stop = text.lastIndexOf(". ", at);
  return stop < 0 ? 0 : stop + 2;
}

function sentenceEnd(text: string, at: number): number {
  const stop = text.indexOf(". ", at);
  return stop < 0 ? text.length : stop + 1;
}

const FIELD_KEYS: Record<ReaderField, { value: keyof LedgerRow; quote: keyof LedgerRow }> = {
  caecum: { value: "caecum_reached", quote: "caecum_quote" },
  prep: { value: "prep", quote: "prep_quote" },
  polyps: { value: "polyps_removed", quote: "polyps_quote" },
  histology: { value: "histology_present", quote: "histology_quote" },
};

function build(uid: string): ReaderCase {
  const report = reports.find((r) => r.uid === uid);
  if (!report) throw new Error(`${REPORTS}: no ${uid}`);
  const row = ledger.find((r) => r.report_sha === report.sha);
  if (!row) throw new Error(`${LEDGER}: no row for ${uid} (${report.sha.slice(0, 8)})`);

  const text = clean(report.text);
  const spans = (Object.keys(FIELD_KEYS) as ReaderField[])
    .map((field) => ({ field, at: locate(text, String(row[FIELD_KEYS[field].quote])) }))
    .filter((s): s is { field: ReaderField; at: [number, number] } => s.at !== null)
    .sort((a, b) => a.at[0] - b.at[0]);
  if (spans.length === 0) throw new Error(`${uid}: no reader quote found in the report text`);

  const from = sentenceStart(text, spans[0].at[0]);
  const to = sentenceEnd(text, sentenceEnd(text, spans.at(-1)!.at[1]) + 1);
  const segments: ReaderSegment[] = [];
  let cursor = from;
  for (const s of spans) {
    if (s.at[0] < cursor) continue;
    if (s.at[0] > cursor) segments.push({ text: text.slice(cursor, s.at[0]) });
    segments.push({ text: text.slice(s.at[0], s.at[1]), field: s.field });
    cursor = s.at[1];
  }
  if (cursor < to) segments.push({ text: text.slice(cursor, to) });
  if (from > 0) segments.unshift({ text: "… " });
  if (to < text.length) segments.push({ text: " …" });

  const pick = (f: Fields) => ({ caecum: f.caecum, prep: f.prep, polyps: f.polyps, histology: f.histology });
  const standsFor = fixtureEvidence.find((e) => e.quote.length >= 20 && text.toLowerCase().includes(e.quote.toLowerCase()));
  return {
    uid,
    title: report.name,
    sha: report.sha,
    ...(standsFor ? { standsFor: { loop: standsFor.loop, report: standsFor.report } } : {}),
    segments,
    rules: { ...pick(report.regex), state: report.regex.state, reason: report.regex.reason },
    ai: {
      caecum: row.caecum_reached,
      prep: row.prep,
      polyps: row.polyps_removed,
      histology: row.histology_present,
      state: row.state,
      reason: row.reason,
      downgraded: (row.downgraded ?? []) as ReaderField[],
      modelVersion: row.model_version,
      latencyMs: row.latency_ms,
    },
  };
}

const data: ReaderData = { label: "replay", source: { reports: REPORTS, ledger: LEDGER }, cases: PICKS.map(build) };
writeFileSync(path.join(web, "data", "reader.json"), JSON.stringify(data, null, 1) + "\n");
console.log(`build-reader: ${data.cases.map((c) => `${c.uid} ${c.ai.state}/${c.ai.reason} ${c.ai.latencyMs} ms`).join(", ")}`);
