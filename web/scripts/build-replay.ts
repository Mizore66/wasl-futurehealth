// Builds data/replay.json from the recorded showcase run in pipeline/results/showcase.
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { COUNTERS, type Counters, type Outcome, type Replay, type ReplayEvent, type ScriptedCase } from "../lib/replay/types";

const web = path.join(import.meta.dirname, "..");
const showcase = path.join(web, "..", "pipeline", "results", "showcase");
const rel = (f: string) => path.relative(path.join(web, ".."), path.join(showcase, f)).replaceAll("\\", "/");

const summary = readFileSync(path.join(showcase, "summary.txt"), "utf8");
const report = JSON.parse(readFileSync(path.join(showcase, "report7.json"), "utf8"));
const tamper = JSON.parse(readFileSync(path.join(showcase, "tamper.json"), "utf8"));
const ledger = readFileSync(path.join(showcase, "reader_ledger.jsonl"), "utf8")
  .trim()
  .split("\n")
  .map((line) => JSON.parse(line));

function grab(pattern: RegExp): number {
  const m = summary.match(pattern);
  if (!m) throw new Error(`summary.txt: no match for ${pattern}`);
  return Number(m[1].replaceAll(",", ""));
}

const outputs = grab(/duplicates \d+ \/ (\d+) outputs/);
const duplicates = grab(/duplicates (\d+) \/ \d+ outputs/);
const chains = grab(/provenance chains verified (\d+)\/\d+/);
const chainsTotal = grab(/provenance chains verified \d+\/(\d+)/);
if (chains !== chainsTotal) throw new Error(`summary.txt: ${chains}/${chainsTotal} chains verified`);
const forgedWithEffect = grab(/forged events with effect (\d+)\/\d+/);
const forgedEvents = grab(/forged events with effect \d+\/(\d+)/);
const throughput = grab(/throughput (\d+) ev\/s/);

if (report.crash_recovery.live_outputs !== outputs) throw new Error("report7.json and summary.txt disagree on outputs");
const attacks = tamper.attacks.filter((a: { attack: string }) => !a.attack.startsWith("none"));

const totals: Counters = {
  fhirMessages: report.fhir_msgs,
  readerReports: ledger.length,
  outputs,
  chainsVerified: chains,
};

const closingReads = ledger.filter((r) => r.state === "closed").length;
if (closingReads === 0) throw new Error("reader_ledger.jsonl has no closing read");

/** Reports per reader verdict in the scripted loop: every closing read, then a mix that matches the ledger's shape. */
const QUOTA: Record<string, number> = {
  "closed/criteria_met": closingReads,
  "not_closed/incomplete_exam": 4,
  "not_closed/awaiting_histology": 3,
  "not_closed/inadequate_prep": 2,
  "needs_human/missing_documentation": 11,
};

const picked = Object.entries(QUOTA).flatMap(([verdict, n]) => {
  const rows = ledger.filter((r) => `${r.state}/${r.reason}` === verdict);
  if (rows.length < n) throw new Error(`reader_ledger.jsonl has ${rows.length} ${verdict}, need ${n}`);
  return rows.slice(0, n);
});
picked.sort((a, b) => (a.report_sha < b.report_sha ? -1 : 1));

const outcomeOf = (state: string): Outcome =>
  state === "closed" ? "closed_with_evidence" : state === "needs_human" ? "waiting_for_reviewers" : "kept_open";

const cases: ScriptedCase[] = picked.map((r, i) => ({
  id: `showcase-${String(i + 1).padStart(2, "0")}`,
  source: report.source.Source === "hl7" ? "hl7" : "fhir",
  read: {
    reportSha: r.report_sha,
    caecumReached: r.caecum_reached,
    caecumQuote: r.caecum_quote,
    prep: r.prep,
    prepQuote: r.prep_quote,
    tissueTaken: r.tissue_taken,
    tissueQuote: r.tissue_quote,
    histologyPresent: r.histology_present,
    histologyQuote: r.histology_quote,
    state: r.state,
    reason: r.reason,
    modelVersion: r.model_version,
    latencyMs: r.latency_ms,
  },
  gatePassed: r.state === "closed",
  outcome: outcomeOf(r.state),
}));

/** Splits `total` into `n` integer shares that sum exactly to it. */
function shares(total: number, n: number): number[] {
  return Array.from({ length: n }, (_, i) => Math.floor((total * (i + 1)) / n) - Math.floor((total * i) / n));
}

const closedCount = cases.filter((c) => c.outcome === "closed_with_evidence").length;
const fhirShare = shares(totals.fhirMessages, cases.length);
const readShare = shares(totals.readerReports, cases.length);
const outShare = shares(totals.outputs, cases.length);
const chainShare = shares(totals.chainsVerified, closedCount);

/** Cases overlap: a new case is received every two cases' worth of steps, so several are in flight at once. */
const perCase = (c: ScriptedCase, i: number, closedIndex: number): ReplayEvent[] => [
  { type: "received", caseId: c.id, add: { fhirMessages: fhirShare[i] } },
  { type: "matching", caseId: c.id },
  { type: "matched", caseId: c.id },
  { type: "reading", caseId: c.id },
  { type: "read", caseId: c.id, add: { readerReports: readShare[i] } },
  { type: "gate", caseId: c.id, passed: c.gatePassed },
  { type: "outcome", caseId: c.id, outcome: c.outcome },
  {
    type: "recorded",
    caseId: c.id,
    add: { outputs: outShare[i], ...(closedIndex >= 0 ? { chainsVerified: chainShare[closedIndex] } : {}) },
  },
];

let closedSeen = 0;
const tracks = cases.map((c, i) => perCase(c, i, c.outcome === "closed_with_evidence" ? closedSeen++ : -1));
const STAGGER = 3;
const events: ReplayEvent[] = [];
for (let tick = 0; events.length < tracks.length * 8; tick++) {
  tracks.forEach((track, i) => {
    const at = tick - i * STAGGER;
    if (at >= 0 && at < track.length) events.push(track[at]);
  });
}
events.push({ type: "loop_end" });

const readerStates: Record<string, number> = {};
for (const r of ledger) readerStates[`${r.state}/${r.reason}`] = (readerStates[`${r.state}/${r.reason}`] ?? 0) + 1;

const replay: Replay = {
  label: "replay",
  source: { run: report.run, files: ["summary.txt", "reader_ledger.jsonl", "report7.json", "tamper.json"].map(rel) },
  totals,
  facts: {
    loops: report.loops,
    duplicates,
    lost: report.crash_recovery.lost_outputs,
    throughputEvS: throughput,
    forgedEvents,
    forgedWithEffect,
    tamperAttacks: attacks.length,
    tamperDetected: attacks.filter((a: { detected: boolean }) => a.detected).length,
    readerModel: ledger[0].model_version,
    readerStates,
  },
  cases,
  events,
};

for (const k of COUNTERS) {
  const sum = events.reduce((s, e) => s + ("add" in e ? (e.add[k] ?? 0) : 0), 0);
  if (sum !== totals[k]) throw new Error(`replay ${k}: events add ${sum}, total is ${totals[k]}`);
}

writeFileSync(path.join(web, "data", "replay.json"), JSON.stringify(replay, null, 1) + "\n");
console.log(`build-replay: ${cases.length} cases, ${events.length} events, totals ${JSON.stringify(totals)}`);
