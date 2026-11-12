import { describe, expect, it } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { composableIfhasLabels, composeIfhasLabel, IFHAS_DOMAINS, parseIfhasLabel } from "@/components/review/ifhas-label";
import { GUIDED_EVIDENCE_LABELS, ReviewDecision, type CaseDetail, type Programme } from "@/lib/api/schema";
import { programmeDetail } from "./programme-cases";

type GuidedProgramme = Extract<Programme, "crc" | "lung_nodule" | "diabetic_retinopathy" | "breast" | "lung_ldct" | "ifhas">;

function detailFor(programme: GuidedProgramme): CaseDetail {
  const detail = programmeDetail("lung_ldct");
  detail.case.programme = programme;
  detail.case.review = "open";
  detail.review_items = [
    {
      item: `rv-${programme}`,
      loop: detail.case.id,
      kinds: ["evidence"],
      pair: [],
      p: 0,
      report: detail.evidence[0].report,
      verdict: "disagree",
      quote: "",
      chain: [],
    },
  ];
  return detail;
}

const lungOutcomes = ["stable", "resolved", "growing", "suspicious_solid", "not_assessable", "pet_low", "pet_avid", "benign", "nondiagnostic", "malignant"];
const lungIndex = ["solid", "ground_glass", "part_solid"].flatMap((type) =>
  ["<6", "6-8", ">8", ">=6"].flatMap((size) => ["single", "multiple"].map((count) => `${type}/${size}/${count}`)),
);
const drGrades = ["0", "1", "2", "3A", "3S"].flatMap((r) =>
  ["0", "1"].flatMap((m) => [`R${r}M${m}`, `R${r}M${m}/urgent`]),
);
const drOutcomes = ["examined_treated", "examined_followup", "examined_discharged", "not_examined", "dna", "not_assessable"];

const EXPECTED: Record<Exclude<GuidedProgramme, "crc" | "ifhas">, string[]> = {
  lung_nodule: [...lungOutcomes, ...lungIndex],
  diabetic_retinopathy: [...drGrades, "U", ...drOutcomes],
  breast: ["B0", "B1", "B2", "B3", "B4", "B5", "TR", "return_routine", "early_recall", "to_treatment", "incomplete", "dna"],
  lung_ldct: ["LR0", "LR1", "LR2", "LR3", "LR4A", "LR4B", "LR4X", "wk_treatment", "wk_benign", "wk_surveillance", "wk_incomplete", "wk_dna"],
};

describe("programme evidence vocabulary", () => {
  it.each(Object.entries(EXPECTED))("offers every backend-supported %s label and no others", (programme, expected) => {
    const view = reviewView(detailFor(programme as GuidedProgramme));
    expect(view.guided).toBe(true);
    expect(view.evidenceChoices).toEqual(expected);
    for (const label of expected) expect(ReviewDecision.safeParse(label).success).toBe(true);
  });

  it("offers all 48 IFHAS screening cells and all 256 follow-up labels", () => {
    const view = reviewView(detailFor("ifhas"));
    expect(view.guided).toBe(true);
    expect(view.evidenceChoices).toHaveLength(304);
    expect(view.evidenceChoices).toEqual(
      expect.arrayContaining([
        "bp=N;glyc=N;lipid=N;weight=N",
        "bp=S;glyc=S;lipid=H;weight=O",
        "bp=N;glyc=P;lipid=N;weight=N",
        "dna",
        "bp=managed",
        "bp=managed;glyc=not_confirmed;lipid=pending;weight=managed",
        "weight=pending",
      ]),
    );
    for (const label of view.evidenceChoices) expect(ReviewDecision.safeParse(label).success).toBe(true);
  });

  it("keeps the two CRC evidence answers and the no-API ask action separate", () => {
    expect(reviewView(detailFor("crc")).evidenceChoices).toEqual(["adequate", "inadequate"]);
  });

  it.each(["pet_low", "pet_avid", "benign", "nondiagnostic", "malignant"])("accepts the lung procedure label %s at the API boundary", (label) => {
    expect(ReviewDecision.safeParse(label).success).toBe(true);
  });

  it.each(["R9M0", "R2M2", "solid/9-12/single", "unknown/6-8/single", "glyc=closed", "bp=managed;bp=pending"])(
    "rejects unsupported evidence label %s",
    (label) => {
      expect(ReviewDecision.safeParse(label).success).toBe(false);
    },
  );
});

describe("IFHAS label composer", () => {
  const composable = composableIfhasLabels();
  const allowed = GUIDED_EVIDENCE_LABELS.ifhas;

  it("composes exactly the allowed IFHAS labels: 48 screening, dna and 255 follow-up", () => {
    expect(new Set(composable).size).toBe(composable.length);
    expect(composable).toHaveLength(allowed.length);
    expect(composable).toHaveLength(304);
    for (const label of composable) expect(allowed).toContain(label);
    expect([...composable].sort()).toEqual([...allowed].sort());
    expect(composable.filter((l) => /=(N|H|S|P|D|O)(;|$)/.test(l))).toHaveLength(48);
    expect(composable.filter((l) => /=(managed|not_confirmed|pending)(;|$)/.test(l))).toHaveLength(255);
  });

  it("writes follow-up domains in the sorted order pipeline/ifhasrule.go ifhasFormat writes", () => {
    expect([...IFHAS_DOMAINS]).toEqual([...IFHAS_DOMAINS].sort());
    for (const label of composable) {
      if (label === "dna") continue;
      const keys = label.split(";").map((p) => p.split("=")[0]);
      expect(keys).toEqual([...keys].sort());
    }
  });

  it("round-trips every composable label through the controls' draft", () => {
    for (const label of composable) expect(composeIfhasLabel(parseIfhasLabel(label))).toBe(label);
  });

  it("composes nothing until the draft is complete", () => {
    expect(composeIfhasLabel({ values: {} })).toBe("");
    expect(composeIfhasLabel({ kind: "screening", values: { bp: "H", glyc: "N", lipid: "N" } })).toBe("");
    expect(composeIfhasLabel({ kind: "followup", values: {} })).toBe("");
    expect(composeIfhasLabel({ kind: "followup", values: { bp: "", glyc: "" } })).toBe("");
    expect(composeIfhasLabel({ kind: "screening", values: { bp: "managed", glyc: "N", lipid: "N", weight: "N" } })).toBe("");
    expect(composeIfhasLabel({ kind: "followup", values: { glyc: "H" } })).toBe("");
    expect(composeIfhasLabel({ kind: "screening", values: { weight: "O", bp: "S", lipid: "H", glyc: "P" } })).toBe("bp=S;glyc=P;lipid=H;weight=O");
    expect(composeIfhasLabel({ kind: "followup", values: { weight: "pending", glyc: "not_confirmed" } })).toBe("glyc=not_confirmed;weight=pending");
    expect(composeIfhasLabel({ kind: "dna", values: { bp: "managed" } })).toBe("dna");
  });

  it.each(["", "indeterminate", "glyc=managed;bp=pending", "bp=managed;bp=pending", "bp=N;glyc=managed", "bp=X;glyc=N;lipid=N;weight=N", "solid/6-8/single"])(
    "shows no draft for %j, which the composer never writes",
    (label) => {
      expect(parseIfhasLabel(label)).toEqual({ values: {} });
    },
  );
});
