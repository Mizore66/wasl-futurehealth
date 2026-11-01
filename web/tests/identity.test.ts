import { describe, expect, it } from "vitest";
import { CaseDetail } from "@/lib/api/schema";
import { beforeReview, linkTrust, recordFace, referralSeed } from "@/lib/people/identity";
import { portrait, signature } from "@/components/avatar/traits";
import hold from "@/data/api/case_review_hold.json";
import closure from "@/data/api/case_evidence_closure.json";

const mariam = CaseDetail.parse(hold);
const fatima = CaseDetail.parse(closure);
const MARIAM = { referral: "ahs-mushrif|AHS-MRN000516", report: "skmc-endo|SKM-MRN000511" };
const FATIMA = { referral: "ahs-mushrif|AHS-MRN000985", report: "skmc-endo|SKM-MRN000392" };
const face = (c: CaseDetail, reg: string) => recordFace(c, reg);
const drawn = (c: CaseDetail, reg: string) => {
  const f = face(c, reg);
  return f.state === "unknown" ? "?" : signature(portrait(f.person));
};

describe("identity model: who a record's avatar shows", () => {
  it("keys a fixture referral on its registration, and any other loop on its referral id", () => {
    expect(referralSeed(mariam.case.id).key).toBe(MARIAM.referral);
    expect(referralSeed("ahs-mushrif/ServiceRequest/AHS-sr-1000").key).toBe("ahs-mushrif/ServiceRequest/AHS-sr-1000");
  });

  it("Mariam: before review her colonoscopy report is '?' although p 0.9985 ≥ τ, because a link review item holds it", () => {
    const before = beforeReview(mariam);
    expect(mariam.links[0].path_p_cal).toBeGreaterThanOrEqual(mariam.links[0].tau);
    expect(linkTrust(before, before.links[0])).toBe("held");
    expect(drawn(before, MARIAM.report)).toBe("?");
    expect(drawn(before, MARIAM.referral)).not.toBe("?");
  });

  it("Mariam: once signed same, the report wears exactly the referral's face", () => {
    expect(mariam.signoffs.map((s) => s.role)).toEqual(["reviewer", "clinician"]);
    expect(linkTrust(mariam, mariam.links[0])).toBe("signed");
    expect(drawn(mariam, MARIAM.report)).toBe(drawn(mariam, MARIAM.referral));
  });

  it("Fatima: her report's link scored 0.9485, under τ; '?' before the reviewer, her face after", () => {
    expect(fatima.review_items[0].p).toBeLessThan(fatima.links[0].tau);
    expect(drawn(beforeReview(fatima), FATIMA.report)).toBe("?");
    expect(linkTrust(fatima, fatima.links[0])).toBe("signed");
    expect(drawn(fatima, FATIMA.report)).toBe(drawn(fatima, FATIMA.referral));
  });

  it("Fatima and Mariam never share a face", () => {
    expect(drawn(fatima, FATIMA.referral)).not.toBe(drawn(mariam, MARIAM.referral));
  });

  it("trusts a link at or above τ with no review item, and not one below τ", () => {
    const bare = { ...beforeReview(mariam), review_items: [] };
    expect(linkTrust(bare, { ...bare.links[0], path_p_cal: 0.99 })).toBe("auto");
    expect(linkTrust(bare, { ...bare.links[0], path_p_cal: 0.989 })).toBe("below");
    const below = { ...bare, links: [{ ...bare.links[0], path_p_cal: 0.989 }] };
    expect(drawn(below, MARIAM.report)).toBe("?");
  });

  it("a report a person rejected gets its own face, never the referral's", () => {
    const rejected = { ...mariam, signoffs: mariam.signoffs.map((s) => (s.kinds.includes("link") ? { ...s, link: "different" } : s)) };
    expect(linkTrust(rejected, rejected.links[0])).toBe("rejected");
    const f = face(rejected, MARIAM.report);
    expect(f.state).toBe("known");
    expect(f.person.key).toBe(MARIAM.report);
    expect(drawn(rejected, MARIAM.report)).not.toBe(drawn(rejected, MARIAM.referral));
  });
});
