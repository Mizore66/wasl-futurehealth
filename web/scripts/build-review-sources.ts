// Builds data/review-sources.json: the complete synthetic report text behind each raised evidence item in the fixture
// cases, so the fixtures-mode review desk can show the report a reviewer judges (issue #162).
//
// Simulated, not a clinical-document store: the text is the committed MTSamples report (data/evidence/reports43.json)
// that data/reader.json already matched to the fixture by its quote and both readers' answers. The live API never
// reads this file; it re-reads the received FHIR message by pointer (pipeline/api_review_source.go).
import { createHash } from "node:crypto";
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { CaseDetail } from "../lib/api/schema";
import type { ReaderData } from "../components/pipeline/reader-types";

const web = path.join(import.meta.dirname, "..");
const root = path.join(web, "..");
const REPORTS = "data/evidence/reports43.json";

type Report = { uid: string; sha: string; text: string };
const reports: Report[] = JSON.parse(readFileSync(path.join(root, REPORTS), "utf8")).reports;
const reader: ReaderData = JSON.parse(readFileSync(path.join(web, "data", "reader.json"), "utf8"));

export type FixtureSource = {
  /** Provider-qualified DiagnosticReport the item's evidence chain names. */
  report: string;
  report_version: number;
  day: number;
  category?: string;
  text: string;
  /** sha256 of `text`; the fixtures loader refuses a text that no longer matches it. */
  sha256: string;
  /** Where the text came from: the MTSamples row in data/evidence/reports43.json. */
  provenance: string;
};

const out: Record<string, Record<string, FixtureSource>> = {};
const dir = path.join(web, "data", "api");
for (const f of readdirSync(dir).filter((f) => /^case_.*\.json$/.test(f) && !f.endsWith("_audit.json")).sort()) {
  const detail = CaseDetail.parse(JSON.parse(readFileSync(path.join(dir, f), "utf8")));
  for (const item of detail.review_items) {
    if (!item.kinds.includes("evidence") || !item.report) continue;
    const refs = [...new Set(item.chain.map((b) => b.body.report).filter((r): r is string => typeof r === "string" && r.endsWith(`/DiagnosticReport/${item.report}`)))];
    if (refs.length !== 1) continue; // missing or ambiguous: the loader reports it unavailable
    const match = reader.cases.filter((c) => c.standsFor?.loop === detail.case.id && c.standsFor.report === item.report);
    if (match.length !== 1) continue;
    const row = reports.find((r) => r.sha === match[0].sha);
    if (!row) throw new Error(`${REPORTS}: no report with sha ${match[0].sha}`);
    const sha256 = createHash("sha256").update(row.text).digest("hex");
    if (sha256 !== row.sha) throw new Error(`${REPORTS}: ${row.uid} text does not hash to its recorded sha`);
    const event = detail.timeline.find((t) => t.id === refs[0]);
    (out[detail.case.id] ??= {})[item.item] = {
      report: refs[0],
      report_version: item.report_version ?? 1,
      day: event?.day ?? 0,
      ...(event?.category ? { category: event.category } : {}),
      text: row.text,
      sha256,
      provenance: `${REPORTS}#${row.uid} (MTSamples, synthetic case binding)`,
    };
  }
}

writeFileSync(path.join(web, "data", "review-sources.json"), JSON.stringify(out, null, 1) + "\n");
const n = Object.values(out).reduce((a, m) => a + Object.keys(m).length, 0);
console.log(`build-review-sources: ${n} source report(s) -> data/review-sources.json`);
