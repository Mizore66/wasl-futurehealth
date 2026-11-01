// Builds data/results.json: every evaluation number a screen may show, each with its source file and label.
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import type { ResultEntry, Results } from "../lib/results/types";

/** `published` marks a figure from a peer-reviewed study, not from Wasl's own testing. `lib/honesty.ts` does not list it yet. */
type Label = ResultEntry["label"] | "published";
type Entry = Omit<ResultEntry, "label"> & { label: Label };

const web = path.join(import.meta.dirname, "..");
const repo = path.join(web, "..");
const read = (f: string) => readFileSync(path.join(repo, f), "utf8").replace(/\r\n/g, "\n");

const SUMMARY = "pipeline/results/final/SUMMARY.md";
const BRIEF = "docs/BRIEF.md";
const PITCH = "docs/PITCH.md";
const DASHBOARD = "docs/api-samples/dashboard.json";
const SHOWCASE = "pipeline/results/showcase/summary.txt";

const entries: Record<string, Entry> = {};
function put(id: string, e: Omit<Entry, "id">) {
  if (entries[id]) throw new Error(`duplicate result id ${id}`);
  entries[id] = { id, ...e };
}

const num = (s: string) => Number(s.replace(/,/g, "").replace("%", ""));
const fmt = (n: number) => n.toLocaleString("en-US");

// SUMMARY.md: one block per noise model; three table kinds.
const summary = read(SUMMARY);
const revision = summary.match(/Code revision `([0-9a-f]+)`/)?.[1];
if (!revision) throw new Error(`${SUMMARY}: no code revision`);

for (const block of summary.split(/^## /m).slice(1)) {
  const noise = block.match(/^Noise (v\d)/)?.[1];
  if (!noise) continue;
  const context = noise === "v2" ? "frozen evaluation, held-out v2 noise, 5 seeds" : "frozen evaluation, dev v1 noise, 5 seeds";
  for (const table of block.split(/\n\n/)) {
    const rows = table.split("\n").filter((l) => l.startsWith("|") && !/^\|[-\s|]+\|$/.test(l));
    if (rows.length < 2) continue;
    const head = rows[0];
    const cells = (l: string) => l.split("|").slice(1, -1).map((c) => c.trim());
    for (const row of rows.slice(1)) {
      const [name, ...rest] = cells(row);
      if (head.includes("| Mean |")) {
        const [lo, hi] = rest[1].split("–").map((s) => num(s.trim()));
        put(`${noise}.${name}`, { value: num(rest[0]), display: rest[0], ci95: [lo, hi], label: "simulated", source: SUMMARY, context });
      } else if (head.includes("| Events |")) {
        const key = name.replace(/ \(.*\)$/, "");
        const value = num(rest[0]);
        const denominator = rest[1] === "–" ? undefined : num(rest[1]);
        put(`${noise}.safety.${key}`, {
          value,
          denominator,
          upper95: rest[2] === "–" ? undefined : rest[2],
          display: denominator === undefined ? fmt(value) : `${fmt(value)} / ${fmt(denominator)}`,
          label: "simulated",
          source: SUMMARY,
          context: `${context}, pooled`,
        });
      } else if (head.includes("| Count |")) {
        put(`${noise}.counts.${name}`, { value: num(rest[0]), display: rest[0], label: "simulated", source: SUMMARY, context });
      }
    }
  }
}
if (!entries["v2.safety.crc_false_closures"]) throw new Error(`${SUMMARY}: held-out safety table not found`);

// The BRIEF evidence table, verbatim, plus the numbers screens quote from it.
const brief = read(BRIEF);
const evidenceBlock = brief.split(/^## Evidence/m)[1]?.split(/^## /m)[0];
if (!evidenceBlock) throw new Error(`${BRIEF}: no Evidence section`);
const evidence = evidenceBlock
  .split("\n")
  .filter((l) => l.startsWith("|") && !l.startsWith("| What") && !/^\|[-\s|]+\|$/.test(l))
  .map((l) => {
    const [what, result] = l.split("|").slice(1, -1).map((c) => c.trim());
    return { what, result, source: BRIEF, label: "simulated" as const };
  });

function fromBrief(id: string, pattern: RegExp, context: string, label: Label = "simulated") {
  const m = evidenceBlock!.match(pattern);
  if (!m) throw new Error(`${BRIEF}: no match for ${id} (${pattern})`);
  put(id, { value: num(m[1]), display: m[1], label, source: BRIEF, context });
}
fromBrief("double_review.false_closures_denominator", /\*\*0\/([\d,]+) false closures\*\*\. Single review/, "v7 gate offline sweep, double review, 5 seeds × 3 reviewer error rates");
fromBrief("single_review.false_closures", /Single review gives (\d+)/, "v7 gate offline sweep, single review");
fromBrief("ltt.calibration_loops", /Learn-then-Test gate, ([\d,]+) calibration loops/, "Learn-then-Test certification of τ_close 0.99");
fromBrief("hl7.resources_exactly_once", /([\d,]+)\/[\d,]+ resources delivered exactly once/, "HL7 v2 over MLLP with drops and re-sends");
fromBrief("encoder.recall_at_10", /R@10 (0\.\d+) vs/, "Wasl encoder, name-pair recall at 10");
fromBrief("encoder.recall_at_10_best_single", /R@10 0\.\d+ vs (0\.\d+)/, "best single off-the-shelf encoder");
fromBrief("reader.corpora", /: (\d+) more 300-report corpora/, "adversarial report corpora, 4 model families");

// The published Abu Dhabi baseline the landing quotes (the BRIEF's own citation: SEHA, BMC Cancer 2021).
const baseline = brief.match(/after a median \*\*(\d+) days\*\*, against a (\d+)-day standard \(SEHA, BMC Cancer 2021\)/);
if (!baseline) throw new Error(`${BRIEF}: published colonoscopy wait not found`);
const cited = "SEHA, BMC Cancer 2021, as cited in docs/BRIEF.md";
put("published.crc_followup_median_wait_days", { value: num(baseline[1]), display: baseline[1], label: "published", source: BRIEF, context: `median days from a positive FIT to colonoscopy in Abu Dhabi; ${cited}` });
put("published.crc_followup_standard_days", { value: num(baseline[2]), display: baseline[2], label: "published", source: BRIEF, context: `the follow-up standard that wait is measured against; ${cited}` });

// Demo cohort KPI: the run the API serves (docs/api-samples).
const dashboard = JSON.parse(read(DASHBOARD));
const kpi = dashboard.metrics.crc_kpi_31d;
for (const [arm, key] of [["eid_only", "eid_only"], ["wasl", "wasl_live"], ["truth", "truth"]] as const) {
  const v = kpi[key].kpi_all_referrals as number;
  put(`kpi31.demo.${arm}`, {
    value: v,
    display: `${(v * 100).toFixed(1)}%`,
    label: "simulated",
    source: DASHBOARD,
    context: `CRC 31-day completion, all referrals, demo run ${dashboard.meta.run} (${dashboard.meta.policy})`,
  });
}
for (const [arm, key] of [["eid_only", "kpi_eid_only"], ["wasl", "kpi_wasl"], ["truth", "kpi_truth"]] as const) {
  const e = entries[`v2.${key}`];
  put(`kpi31.heldout.${arm}`, { value: e.value, ci95: e.ci95, display: `${(e.value * 100).toFixed(1)}%`, label: e.label, source: e.source, context: `CRC 31-day completion, ${e.context}` });
}

// Showcase (integrated) run totals that the pipeline replay ticks toward.
const showcase = read(SHOWCASE);
const decisions = showcase.match(/duplicates (\d+) \/ (\d+) outputs/);
const chains = showcase.match(/provenance chains verified (\d+)\/(\d+)/);
if (!decisions || !chains) throw new Error(`${SHOWCASE}: totals not found`);
put("showcase.decisions", { value: num(decisions[2]), display: fmt(num(decisions[2])), label: "simulated", source: SHOWCASE, context: "integrated showcase run, decision outputs, 0 duplicated" });
put("showcase.chains_verified", { value: num(chains[1]), denominator: num(chains[2]), display: `${fmt(num(chains[1]))} / ${fmt(num(chains[2]))}`, label: "simulated", source: SHOWCASE, context: "integrated showcase run, provenance chains hash-verified" });

// Exactly-once figures: two sources disagree. Record both; do not choose.
const pitch = read(PITCH);
const pitchMatch = pitch.match(/0 of ([\d,]+) outputs duplicated or lost/);
if (!pitchMatch) throw new Error(`${PITCH}: exactly-once figure not found`);
put("exactly_once.pitch_outputs", { value: num(pitchMatch[1]), display: pitchMatch[1], label: "simulated", source: PITCH, context: "claimed: 10 frozen-evaluation runs with process and broker kills" });

const v1 = entries["v1.safety.duplicate_or_lost_outputs"];
const v2 = entries["v2.safety.duplicate_or_lost_outputs"];
const conflicts = [
  {
    id: "exactly_once_outputs",
    question: "How many outputs did the frozen evaluation check for duplicates or losses?",
    values: [
      { result: "exactly_once.pitch_outputs", value: entries["exactly_once.pitch_outputs"].value, source: PITCH },
      { result: "v2.safety.duplicate_or_lost_outputs", value: v2.denominator!, source: SUMMARY },
      { result: "v1.safety.duplicate_or_lost_outputs", value: v1.denominator!, source: SUMMARY },
    ],
    note: `PITCH says 10 runs; SUMMARY's v1 + v2 denominators sum to ${fmt(v1.denominator! + v2.denominator!)}, matching neither. Unresolved: screens must not show an exactly-once denominator until the backend owner picks one.`,
  },
];

const results: Omit<Results, "entries"> & { entries: Record<string, Entry> } = {
  frozen_revision: revision,
  sources: [SUMMARY, BRIEF, PITCH, DASHBOARD, SHOWCASE],
  entries,
  evidence,
  conflicts,
};

writeFileSync(path.join(web, "data", "results.json"), JSON.stringify(results, null, 1) + "\n");
console.log(`build-results: ${Object.keys(entries).length} numbers, ${evidence.length} evidence rows, ${conflicts.length} conflict(s)`);
