import { readFileSync } from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { HistoryCard } from "@/components/case/audit-view";
import { LedgerCard } from "@/components/case/sections";
import { Audit, CaseDetail } from "@/lib/api/schema";

/*
 * fixtures/ledger_referral_*.json are GET /api/cases/{id} and GET /api/cases/{id}/audit as the Go handler serves them
 * after a conditional acceptance, and after a return the referring GP acknowledged and answered
 * (pipeline/api_referrer_test.go TestReferralLedgerFixtures, WASL_WRITE_LEDGER_FIXTURES=1).
 */
const load = (f: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", f), "utf8"));
const returned = CaseDetail.parse(load("ledger_referral_returned_detail.json"));
const accepted = CaseDetail.parse(load("ledger_referral_accepted_detail.json"));
const audit = Audit.parse(load("ledger_referral_audit.json"));

describe("referral answers in the case history", () => {
  it("parses both referral ledger kinds as coordinator events", () => {
    const kinds = [...returned.api_reviews, ...accepted.api_reviews, ...audit.history].map((e) => `${e.ledger}:${"kind" in e ? e.kind : ""}`);
    expect(kinds).toEqual([
      "coordinator:referral_response",
      "coordinator:referrer_response",
      "coordinator:referrer_response",
      "coordinator:referral_response",
      "coordinator:referral_response",
      "coordinator:referrer_response",
      "coordinator:referrer_response",
    ]);
  });

  it("the patient record names the unit's answer and the GP's response", () => {
    const html = renderToStaticMarkup(LedgerCard({ detail: returned }));
    expect(html).toContain("returned the referral to the GP: blood thinner plan missing");
    expect(html).toContain("Referring GP acknowledged the returned referral");
    expect(html).toContain("Referring GP sent the information the unit asked for");
    expect(html).not.toContain("Unrecognised ledger event");

    const conditional = renderToStaticMarkup(LedgerCard({ detail: accepted }));
    expect(conditional).toContain("accepted the referral with conditions: Arabic interpreter on the day; Hold blood thinners as advised");
  });

  it("the audit history keeps the return and both GP answers without free text", () => {
    const html = renderToStaticMarkup(HistoryCard({ audit, programme: "crc" } as never));
    expect(html).toContain("returned the referral to the GP");
    expect(html).toContain("Referring GP acknowledged the returned referral");
    expect(html).toContain("Referring GP sent the information the unit asked for");
    expect(html).not.toContain("Unrecognised ledger event");
    expect(html).not.toContain("apixaban");
  });
});
