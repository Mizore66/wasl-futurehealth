import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { beforeAll, describe, expect, it } from "vitest";
import { getFixtureReviewSource } from "@/lib/api/source";
import hold from "@/data/api/case_review_hold.json";
import reports43 from "../../data/evidence/reports43.json";

// #162 acceptance 1 in fixtures mode (simulated): the review_hold tissue disagreement opens its complete report.
const caseId = hold.case.id;
const evidenceItem = hold.review_items.find((r) => r.kinds.includes("evidence"))!.item;
const linkItem = hold.review_items.find((r) => !r.kinds.includes("evidence"))!.item;
const committed = path.join(process.cwd(), "data", "review-sources.json");

function withSources(mutate: (entry: Record<string, unknown>) => void): string {
  const all = JSON.parse(readFileSync(committed, "utf8"));
  mutate(all[caseId][evidenceItem]);
  const file = path.join(mkdtempSync(path.join(tmpdir(), "wasl-src-")), "review-sources.json");
  writeFileSync(file, JSON.stringify(all));
  return file;
}

beforeAll(() => {
  delete process.env.WASL_API_URL;
});

describe("fixture review source", () => {
  it("serves the complete matching report, not only the reader's quote", async () => {
    const res = await getFixtureReviewSource(caseId, evidenceItem);
    expect(res.ok).toBe(true);
    if (!res.ok) return;
    expect(res.source.report).toBe("skmc-endo/DiagnosticReport/SKM-dr-217");
    expect(res.source.report_version).toBe(1);
    expect(res.source.category).toBe("colonoscopy");
    // The tissue context both readers disagreed on is in the report, beyond "the level of the cecum".
    expect(res.source.text).toContain("the level of the cecum");
    expect(res.source.text).toContain("completely removed using multiple bites with cold biopsy forceps");
    expect(res.source.text.length).toBeGreaterThan("the level of the cecum".length * 10);
    // It is the committed synthetic text itself, byte for byte.
    const row = reports43.reports.find((r) => r.text === res.source.text);
    expect(row?.uid).toBe("u35");
    expect(createHash("sha256").update(res.source.text).digest("hex")).toBe(row?.sha);
  });

  it("refuses other items and cases without content", async () => {
    expect(await getFixtureReviewSource(caseId, linkItem)).toEqual({ ok: false, status: 400 });
    expect(await getFixtureReviewSource(caseId, "rv-elsewhere")).toEqual({ ok: false, status: 404 });
    expect(await getFixtureReviewSource("no/ServiceRequest/such-case", evidenceItem)).toEqual({ ok: false, status: 404 });
  });

  it("names a missing, wrong-version, wrong-report or altered source as unavailable", async () => {
    const cases: [string, string][] = [
      [withSources((e) => { e.report_version = 2; }), "missing"],
      [withSources((e) => { e.report = "skmc-endo/DiagnosticReport/SKM-dr-999"; }), "missing"],
      [withSources((e) => { e.text = `${e.text} tissue not taken`; }), "unreadable"],
      [path.join(tmpdir(), "wasl-no-such-review-sources.json"), "missing"],
    ];
    for (const [file, status] of cases) {
      const res = await getFixtureReviewSource(caseId, evidenceItem, file);
      expect(res).toEqual({ ok: false, status: 409, sourceStatus: status });
      expect(JSON.stringify(res)).not.toContain("cecum");
    }
  });
});
