// Builds data/replay.json from the recorded showcase run in pipeline/results/showcase.
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { hash } from "../components/avatar/traits";
import type { People } from "../lib/people/types";
import { COUNTERS, type Counters, type Outcome, type Replay, type ReplayEvent, type ReplayPerson, type ScriptedCase } from "../lib/replay/types";
import type { ReaderData } from "../components/pipeline/reader-types";

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

/*
 * Names. Run after build-people and build-reader: a report the Report reading page tells as a cast member's
 * keeps that person from data/people.json, so both pages show the same name and face. Every other report
 * gets a generated name: a common given name and an "Al" family name drawn from different people in the
 * cohort's Wikidata pool, so the pairing names nobody in it. The draw keys on the report's hash, so it is stable.
 */
type PoolPerson = { en: string; ar: string; sex: string; origin: string };
type Part = { en: string; ar: string };
const pool: PoolPerson[] = JSON.parse(readFileSync(path.join(web, "..", "data", "persons3.json"), "utf8"));
const people = (JSON.parse(readFileSync(path.join(web, "data", "people.json"), "utf8")) as People).people;
const reader = JSON.parse(readFileSync(path.join(web, "data", "reader.json"), "utf8")) as ReaderData;

const twoWords = pool.filter((p) => p.origin === "arab" && /^[A-Za-z]{3,9} (al-)?[A-Za-z]{3,9}$/i.test(p.en) && p.ar.split(" ").length === 2);
const split = (p: PoolPerson, i: number): Part => ({ en: p.en.split(" ")[i], ar: p.ar.split(" ")[i] });

/** Parts that at least `min` people in the pool share, spelled the way most of them spell it in Arabic. */
function common(parts: Part[], min: number): Part[] {
  const byEn = new Map<string, Map<string, number>>();
  for (const x of parts) {
    const ar = byEn.get(x.en) ?? new Map<string, number>();
    byEn.set(x.en, ar.set(x.ar, (ar.get(x.ar) ?? 0) + 1));
  }
  return [...byEn]
    .filter(([, ar]) => [...ar.values()].reduce((n, k) => n + k, 0) >= min)
    .map(([en, ar]) => ({ en, ar: [...ar].sort((x, y) => y[1] - x[1])[0][0] }))
    .sort((x, y) => (x.en < y.en ? -1 : 1));
}

const given = {
  female: common(twoWords.filter((p) => p.sex === "F").map((p) => split(p, 0)), 15),
  male: common(twoWords.filter((p) => p.sex === "M").map((p) => split(p, 0)), 15),
};
const family = common(
  twoWords.filter((p) => /^al-/i.test(p.en.split(" ")[1]) && p.ar.split(" ")[1].startsWith("ال")).map((p) => {
    const f = split(p, 1);
    return { en: `Al ${f.en[3].toUpperCase()}${f.en.slice(4)}`, ar: f.ar };
  }),
  2,
);
const realNames = new Set(pool.map((p) => p.en.toLowerCase().replace(/ al-/, " al ")));

/** A downstream clinic's Latin record runs the family name together, as "Fatima Alzaabi" does. */
const reportSpelling = (name: string) => name.replace(/ Al (\w)/, (_, c: string) => ` Al${c.toLowerCase()}`);

const castBySha = new Map(reader.cases.filter((c) => c.standsFor).map((c) => [c.sha, c.standsFor!.loop]));
const taken = new Set<string>();

function personFor(reportSha: string): ReplayPerson {
  const loop = castBySha.get(reportSha);
  if (loop) {
    const provider = loop.split("/")[0];
    const cast = Object.values(people).filter((p) => p.loop === loop);
    const referral = cast.find((p) => p.provider === provider);
    const report = cast.find((p) => p.provider !== provider);
    if (!referral || !report) throw new Error(`people.json: no cast for ${loop}; run build-people first`);
    return {
      key: referral.registration,
      ...(referral.sex ? { sex: referral.sex } : {}),
      name: referral.display_name,
      nameAr: referral.display_name_ar,
      reportName: report.recorded_name,
      source: "canonical-cast",
    };
  }
  for (let salt = 0; ; salt++) {
    const sex = hash(`${reportSha}|sex|${salt}`) % 2 === 0 ? "female" : "male";
    const g = given[sex][hash(`${reportSha}|given|${salt}`) % given[sex].length];
    const f = family[hash(`${reportSha}|family|${salt}`) % family.length];
    const name = `${g.en} ${f.en}`;
    if (taken.has(name) || realNames.has(name.toLowerCase())) continue;
    taken.add(name);
    return { key: `replay|${reportSha.slice(0, 16)}`, sex, name, nameAr: `${g.ar} ${f.ar}`, reportName: reportSpelling(name), source: "generated" };
  }
}

const cases: ScriptedCase[] = picked.map((r, i) => ({
  id: `showcase-${String(i + 1).padStart(2, "0")}`,
  person: personFor(r.report_sha),
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
console.log(`build-replay: ${cases.length} cases (${cases.map((c) => c.person.name).join(", ")}), ${events.length} events, totals ${JSON.stringify(totals)}`);
