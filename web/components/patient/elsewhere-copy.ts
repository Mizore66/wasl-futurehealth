/*
 * Copy for the "already done elsewhere" report (issue #177). English only: there is no medically reviewed Arabic,
 * Urdu or Filipino version yet, so other phone languages show this English copy, marked lang="en", with the shared
 * GatedNote (the #133/#165 policy for unreviewed clinical copy). It says plainly that the report is checked, not
 * accepted as a result.
 */
export type ElsewhereCopy = {
  open: string;
  title: string;
  intro: string;
  procedure: string;
  performed: string;
  performedHint: string;
  provider: string;
  reportRef: string;
  optional: string;
  submit: string;
  sending: string;
  cancel: string;
  failed: string;
  pending: string;
  overdue: string;
  resumed: Record<"not_found" | "not_matching" | "non_closing", string>;
  evidenceClosed: string;
  byCareTeam: string;
  receipt: string;
  notResult: string;
  limit: string;
};

export const ELSEWHERE_COPY: ElsewhereCopy = {
  open: "I already had this done somewhere else",
  title: "Done somewhere else",
  intro: "Tell us where and roughly when. Your care team will ask that place for the report. Until they have it, we will not ask you to book.",
  procedure: "What was done",
  performed: "Roughly when",
  performedHint: "The month is enough.",
  provider: "Where (hospital or clinic)",
  reportRef: "Report or reference number",
  optional: "optional",
  submit: "Send to my care team",
  sending: "Sending…",
  cancel: "Cancel",
  failed: "This could not be sent. Nothing was saved. Please try again or call us.",
  pending: "We are asking {provider} for the report. Your care team aims to have it by {due}.",
  overdue: "We are still waiting for the report from {provider}. Your care team will call you.",
  resumed: {
    not_found: "{provider} could not find the report, so your care team will contact you about the next step.",
    not_matching: "The report we received was for a different test, so your care team will contact you about the next step.",
    non_closing: "The report we received was not enough to finish this check, so your care team will contact you about the next step.",
  },
  evidenceClosed: "This check is now closed. Your care team can tell you more if you call.",
  byCareTeam: "Your care team recorded this from a call with you.",
  receipt: "Reference",
  notResult: "This is a report for your care team to check, not a result. This is a demo: no hospital is contacted.",
  limit: "To report this again, please call your care team.",
};
