import { createHash } from "node:crypto";
import { mkdirSync, mkdtempSync, readdirSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { CaseDetail } from "../lib/api/schema";
import hold from "../data/api/case_review_hold.json";

export const PURPOSES = ["shadow_confirm", "auto_audit"] as const;
export type Purpose = (typeof PURPOSES)[number];

const SOURCE_REPORT = "skmc-endo/DiagnosticReport/SKM-dr-217";
const SOURCE_TEXT = "Synthetic colonoscopy report for a browser test. The caecum was reached; bowel preparation adequate; no tissue was taken.";

/** Synthetic browser input, not a pipeline run, a signed snapshot, or clinical evidence. */
export function purposeFixture(purpose: Purpose, suffix = "preview"): CaseDetail {
  const base = CaseDetail.parse(hold);
  const item = base.review_items.find((entry) => entry.kinds.includes("evidence"))!;
  const id = `synthetic/ServiceRequest/issue111-${purpose}-${suffix}`;
  const state = purpose === "auto_audit" ? "closed" : "procedure_done";
  const evidence = { report: item.report, agree: true, label: "adequate", regex: "adequate", gemma: "adequate", human_override: "", quote: "Synthetic report: caecum reached, adequate preparation, no tissue taken.", reader_model: "synthetic-agreeing-readers", fields: { regex: "yes/adequate/no/no", reader: "yes/adequate/no/no" } };
  return CaseDetail.parse({
    ...base,
    meta: { ...base.meta, run: "synthetic-issue111-browser", reader_model: "synthetic-agreeing-readers", note: "Synthetic Playwright input only; not a pipeline result or clinical validation." },
    case: { ...base.case, id, closed: purpose === "auto_audit", closed_day: purpose === "auto_audit" ? 31 : 0, state, review: "open", fixture: "", decisions: 0 },
    // Reuse only the existing synthetic identity/display scaffolding. Do not copy its signed history or disagreement.
    timeline: [],
    links: base.links.map((link) => ({ ...link, loop: id })),
    evidence: [evidence],
    // One evidence block naming the provider-qualified report, so the source report binds to this item (#162).
    review_items: [{ item: `synthetic-${purpose}-${suffix}`, loop: id, kinds: ["evidence"], pair: item.pair, p: item.p, report: item.report, reason: purpose, verdict: "adequate", quote: "Synthetic agreeing-reader pilot review",
      chain: [{ type: "evidence", body: { ...evidence, report: SOURCE_REPORT }, prev: "genesis", hash: "0".repeat(64) }], head: "" }],
    holds: [], outreach: [], decisions: [], signoffs: [], api_reviews: [],
    api_review_states: undefined, api_contact: undefined, api_overlay: undefined, api_patient: undefined, review_ledger: undefined,
    current: { id: `synthetic-current-${purpose}-${suffix}`, loop: id, state, chain: [] },
    booking: { status: "not_applicable", reason: "Synthetic review-only browser fixture" },
  });
}

/**
 * Give a real next start its own fs-backed fixture manifest, without modifying data/api or adding a production override.
 * The built application/dependencies are linked, not copied or rebuilt. Only this test-owned directory is removed later.
 */
export function fixtureApplication(details: CaseDetail[]): string {
  const root = mkdtempSync(path.join(tmpdir(), "wasl-purpose-browser-"));
  const web = process.cwd();
  for (const name of [".next", "node_modules", "public"]) symlinkSync(path.join(web, name), path.join(root, name), "dir");
  writeFileSync(path.join(root, "package.json"), JSON.stringify({ name: "synthetic-purpose-browser", private: true }));
  const data = path.join(root, "data");
  mkdirSync(path.join(data, "api"), { recursive: true });
  for (const name of readdirSync(path.join(web, "data"))) {
    if (name !== "api" && name !== "review-sources.json") symlinkSync(path.join(web, "data", name), path.join(data, name));
  }
  // The synthetic source report each fixture item judges, in the shape scripts/build-review-sources.ts writes.
  writeFileSync(path.join(data, "review-sources.json"), JSON.stringify(Object.fromEntries(details.map((detail) => [detail.case.id, Object.fromEntries(detail.review_items.map((item) => [item.item, {
    report: SOURCE_REPORT, report_version: 1, day: 30, text: SOURCE_TEXT, sha256: createHash("sha256").update(SOURCE_TEXT).digest("hex"), provenance: "synthetic Playwright input",
  }]))]))));
  for (const name of readdirSync(path.join(web, "data", "api"))) {
    if (name !== "fixtures.json" && !name.startsWith("case_") && !name.startsWith("cases_")) {
      symlinkSync(path.join(web, "data", "api", name), path.join(data, "api", name));
    }
  }
  const json = (name: string, value: unknown) => writeFileSync(path.join(data, "api", name), JSON.stringify(value));
  const [shadow, audit] = details;
  json("fixtures.json", { meta: shadow.meta, fixtures: { evidence_closure: shadow.case, false_closure_trap: shadow.case, review_hold: audit.case } });
  json("cases_crc_page1.json", { meta: shadow.meta, cases: details.map((detail) => detail.case), total: details.length, limit: 50, offset: 0 });
  // source.ts reserves *_audit.json for Audit history, not CaseDetail: keep auto_audit discoverable as a case.
  for (const detail of details) json(`case_${detail.review_items[0].reason}_detail.json`, detail);
  return root;
}
