import type { CaseDetail } from "@/lib/api/schema";

type Obligation = NonNullable<CaseDetail["case"]["crc"]>["obligations"][number];

/**
 * What a CRC obligation's confirmation needs, in one sentence that agrees with the review desk.
 * An unconfirmed, unblocked obligation is confirmed on the review desk by two independent clinicians (claim, then two
 * reads by different actors). The desk's result is a review overlay: the canonical record keeps the obligation awaiting
 * confirmation until the pipeline's own sign-off, so the words never say the care pathway was activated.
 */
export function obligationConfirmationWords(need: Obligation, detail: Pick<CaseDetail, "api_review_states">): string {
  if (need.confirmation) return `Confirmed by ${need.reviewer}.`;
  if (need.state === "blocked" || need.blockers.length > 0) return "Confirmation waits until the holds above are resolved.";
  const st = (detail.api_review_states ?? []).find((s) => s.item === need.review_item && s.question === "evidence");
  switch (st?.state) {
    case "resolved_agreed":
    case "resolved_senior":
      return "Confirmed by two independent clinicians on the review desk. The main record updates after final sign-off.";
    case "awaiting_r2":
      return "One of two independent clinician confirmations is in; a second clinician confirms it on the review desk.";
    case "awaiting_r3":
      return "The two clinician reads differ; a senior clinician resolves it on the review desk.";
    default:
      return "Needs confirmation by two independent clinicians on the review desk.";
  }
}

/** True while any CRC obligation of the case still waits for its own clinician confirmation. */
export function awaitingObligationConfirmation(detail: Pick<CaseDetail, "case">): boolean {
  return (detail.case.crc?.obligations ?? []).some((n) => !n.confirmation && n.state === "awaiting_confirmation");
}

/**
 * The review page's card when no question is open on the desk. It never says the case waits for nothing while a CRC
 * obligation still needs its own clinician confirmation.
 */
export function settledWords(detail: Pick<CaseDetail, "case" | "review_items">): { title: string; body: string } {
  if (awaitingObligationConfirmation(detail)) {
    return {
      title: "A care pathway still needs clinician confirmation",
      body: "No confirmation question is open on this desk for you now. Open the case page: each CRC obligation shows whether it needs confirmation by two independent clinicians and where that stands.",
    };
  }
  if (detail.review_items.length === 0) {
    return { title: "This case is not waiting for a signature", body: "No review item was raised for this case. Open the case page to see the recorded evidence and outcomes." };
  }
  return {
    title: "This case is not waiting for a signature",
    body: "Every review item on this case already has its answer. The answers are below, and the case page shows what they changed.",
  };
}
