/** Staff terms that appear on the review desk and the audit trail without an inline explanation everywhere (N10). */
export type GlossaryKey = "reads" | "canonical" | "overlay" | "anchor" | "comparison_hash";

export const GLOSSARY: Record<GlossaryKey, { term: string; meaning: string }> = {
  reads: {
    term: "r1, r2, r3",
    meaning:
      "The separate answers to one review question. r1 is the first reviewer. r2 is a second person who cannot see r1's answer until they give their own. r3 is a senior reviewer, different from both, who decides only when r1 and r2 disagree.",
  },
  canonical: {
    term: "Canonical",
    meaning:
      "The clinical decision Wasl recorded from the clinic and hospital records. Staff actions are saved beside it and never change it. It changes only when new records arrive.",
  },
  overlay: {
    term: "Overlay",
    meaning:
      "What staff did on the case, kept beside the clinical decision: calls, owners, snoozes, review answers and other actions, each with who did it and when.",
  },
  anchor: {
    term: "Anchor",
    meaning:
      "A copy of the latest record entry, kept in a separate place, so anyone can check that no earlier entry was changed later. Checking against that copy is designed, not built yet.",
  },
  comparison_hash: {
    term: "Comparison hash",
    meaning:
      "A short code made from an export's contents. Two exports of the same record give the same code, so you can tell they match.",
  },
};

export const ALL_GLOSSARY_KEYS = Object.keys(GLOSSARY) as GlossaryKey[];
