import type { CaseDetail } from "@/lib/api/schema";
import type { ChainNode } from "@/components/wasl/sealed-chain";
import { formatDay } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";
import { personFor } from "@/lib/people";
import { recordFace, type Face } from "@/lib/people/identity";
import type { ReviewItems } from "./decision";

/*
 * Everything the review desk shows, derived from one case detail.
 * Names come from data/people.json (gap G1, simulated); every other value is the API's.
 */

export type Verdict = "agrees" | "matches" | "missing" | "differs";
export type IdentityRow = { field: string; referral: string; report: string; verdict: Verdict };

export type ReaderValue = "yes" | "no" | "not_stated" | "adequate" | "inadequate";
export type CheckDecision = "agreed" | "agreed_none" | "not_needed" | "disputed";
export type Check = { field: string; rules: ReaderValue; second: ReaderValue; decision: CheckDecision; disputed: boolean };

export type ReviewView = {
  caseId: string;
  shortId: string;
  name: string;
  nameAr: string;
  age: number;
  referralName: string;
  reportName: string;
  /** The referral's face, and the report's as the API's links and signatures leave it. */
  faces: { referral: Face; report: Face };
  reportRegistration: string;
  referralSource: string;
  reportSource: string;
  unit: string;
  held: string;
  referredOn: string;
  report: { id: string; on: string };
  link: { score: number; line: number; basis: string; rule: string };
  identity: IdentityRow[];
  identitySummary: string;
  checks: Check[];
  quote: string;
  readerModel: string;
  items: ReviewItems;
  trail: ChainNode[];
};

const PROVIDERS: Record<string, string> = {
  "ahs-mushrif": "Al Mushrif",
  "noor-endo": "Noor endoscopy",
  "skmc-endo": "SKMC endoscopy",
};
const providerName = (id: string) => PROVIDERS[id] ?? id;

const BASIS: Record<string, string> = { eid: "Emirates ID", p_cal: "name and details", wasl: "name and details", human: "a person", below_tau: "below the line" };

/** Reader fields arrive as `caecum/prep/tissue/histology`, e.g. `yes/not_stated/no/no`. */
const CHECK_FIELDS = ["Caecum reached", "Prep graded", "Tissue taken", "Histology back"] as const;

function readerValues(s: unknown): ReaderValue[] {
  return typeof s === "string" ? (s.split("/") as ReaderValue[]) : [];
}

function checksFrom(fields: Record<string, unknown> | undefined): Check[] {
  const rules = readerValues(fields?.regex);
  const second = readerValues(fields?.reader);
  const tissue = rules[2] === "yes" || second[2] === "yes";
  return CHECK_FIELDS.map((field, i) => {
    const r = rules[i] ?? "not_stated";
    const s = second[i] ?? "not_stated";
    const disputed = r !== s || r === "not_stated";
    const decision: CheckDecision = disputed
      ? "disputed"
      : i === 3 && !tissue
        ? "not_needed"
        : r === "no"
          ? "agreed_none"
          : "agreed";
    return { field, rules: r, second: s, decision, disputed };
  });
}

export function reviewView(c: CaseDetail): ReviewView {
  const referral = c.identities.find((i) => i.role === "referral");
  const downstream = c.identities.find((i) => i.role === "downstream");
  const a = referral && personFor(referral.registration);
  const b = downstream && personFor(downstream.registration);
  const linkItem = c.review_items.find((r) => r.kinds.includes("link"));
  const evidenceItem = c.review_items.find((r) => r.kinds.includes("evidence"));
  const ev = c.evidence[0];
  const link = c.links[0];
  const guard = linkItem?.chain.find((e) => e.type === "outreach_guard")?.body;
  const reportEvent = c.timeline.find((e) => ev && e.id === ev.report);
  const unit = providerName(downstream?.provider ?? "");
  const checks = checksFrom(ev?.fields);
  const noDob = linkItem?.reason === "second_identifier";
  const held = [noDob && "the report has no date of birth", checks.some((k) => k.disputed) && "neither reader found a bowel prep grade"].filter(
    (r): r is string => !!r,
  );

  const identity: IdentityRow[] = [
    { field: "Name", referral: a?.recorded_name ?? "", report: b?.recorded_name ?? "", verdict: "agrees" },
    {
      field: "Emirates ID",
      referral: "on record",
      report: link?.weakest_edge.basis === "eid" ? "on record" : "not on report",
      verdict: link?.weakest_edge.basis === "eid" ? "matches" : "missing",
    },
    { field: "Date of birth", referral: "on record", report: noDob ? "not on report" : "on record", verdict: noDob ? "missing" : "agrees" },
    {
      field: "Record",
      referral: referral?.registration.split("|")[1] ?? "",
      report: downstream?.registration.split("|")[1] ?? "",
      verdict: "differs",
    },
  ];

  const trail: ChainNode[] = [];
  const add = (id: string, who: string, what: string, day: number | undefined) =>
    day !== undefined && trail.push({ id, kind: "system", who, what, when: formatDay(day) });
  const sr = c.timeline.find((e) => e.id === c.case.id);
  add("referral", providerName(c.case.provider), "referred for colonoscopy", sr?.day ?? c.case.referral_day);
  const consent = c.timeline.find((e) => e.type === "Consent" && e.status === "permit");
  add("consent", a?.display_name.split(" ")[0] ?? "Patient", "allowed messages", consent?.day);
  const intake = c.timeline.find((e) => e.category === "referral-intake");
  add("intake", unit, "accepted the referral", intake?.day);
  add("report", unit, `colonoscopy report ${shortId(ev?.report ?? "")}`, reportEvent?.day);
  if (linkItem) add("link", "Wasl", `matched on ${BASIS[link?.weakest_edge.basis ?? ""] ?? "the record"}; no date of birth, needs a person`, reportEvent?.day);
  if (ev && !ev.agree) add("readers", "Two readers", "no bowel prep grade; needs a person", reportEvent?.day);

  return {
    caseId: c.case.id,
    shortId: shortId(c.case.id),
    name: a?.display_name ?? shortId(c.case.id),
    nameAr: a?.display_name_ar ?? "",
    age: a?.age ?? 0,
    referralName: a?.recorded_name ?? "",
    faces: {
      referral: recordFace(c, referral?.registration ?? ""),
      report: recordFace(c, downstream?.registration ?? ""),
    },
    reportRegistration: downstream?.registration ?? "",
    reportName: b?.recorded_name ?? "",
    referralSource: `${providerName(c.case.provider)} · stool test referral`,
    reportSource: `${unit} · colonoscopy report`,
    unit,
    held: held.length ? `Held: ${held.join(", and ")}.` : "",
    referredOn: formatDay(c.case.referral_day),
    report: { id: shortId(ev?.report ?? ""), on: reportEvent ? formatDay(reportEvent.day) : "" },
    link: {
      score: link?.path_p_cal ?? 0,
      line: link?.tau ?? 0,
      basis: BASIS[link?.weakest_edge.basis ?? ""] ?? "",
      rule: typeof guard?.rule === "string" ? guard.rule : "",
    },
    identity,
    identitySummary: noDob
      ? `Matched on ${BASIS[link?.weakest_edge.basis ?? ""] ?? "the record"}, but the report has no date of birth. A person must confirm.`
      : "A person must confirm the match.",
    checks,
    quote: ev?.quote ?? "",
    readerModel: ev?.reader_model ?? "",
    items: { link: linkItem?.item, evidence: evidenceItem?.item },
    trail,
  };
}
