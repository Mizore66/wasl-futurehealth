/** Staff terms that appear on the review desk and the audit trail without an inline explanation everywhere (N10). */
export type GlossaryKey = "reads" | "canonical" | "overlay" | "anchor" | "comparison_hash";

export const GLOSSARY: Record<GlossaryKey, { term: string; meaning: string }> = {
  reads: {
    term: "r1, r2, r3",
    meaning:
      "The independent reads of one review question. r1 is the first reader. r2 is a second, different person who cannot see r1's answer until they submit their own. r3 is a senior reader, different from both, who decides only when r1 and r2 disagree.",
  },
  canonical: {
    term: "Canonical",
    meaning:
      "The clinical decision the pipeline engine recorded from source events. Staff actions on screen are saved beside it and never rewrite it. It changes only when the engine records a new decision.",
  },
  overlay: {
    term: "Overlay",
    meaning:
      "The staff ledger kept on top of the canonical decision: calls, owners, snoozes, review reads and other actions, each with its actor, time and hash.",
  },
  anchor: {
    term: "Anchor",
    meaning:
      "A chain head (the latest hash) recorded in a store outside the ledger. Matching the recomputed chain against it would show that no earlier entry changed afterwards. Here the page shows the heads the API reports. Checking the external store is designed, not built.",
  },
  comparison_hash: {
    term: "Comparison hash",
    meaning:
      "A hash of the export's content without its load time, so two exports of the same ledger state give the same value and can be compared directly.",
  },
};

export const ALL_GLOSSARY_KEYS = Object.keys(GLOSSARY) as GlossaryKey[];
