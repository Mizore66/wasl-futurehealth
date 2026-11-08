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
  /** Canonical case state from the latest server render, never inferred from a 201. */
  closed: boolean;
  /** Only CRC uses the guided same-patient / colonoscopy report form. */
  guided: boolean;
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
  /** The checks the two readers could not settle: what the one evidence answer covers. */
  disputed: { field: string; says: string }[];
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

const PROGRAMME_COPY: Record<CaseDetail["case"]["programme"], { procedure: string; referral: string; report: string }> = {
  crc: { procedure: "colonoscopy", referral: "stool test referral", report: "colonoscopy report" },
  cervical: { procedure: "colposcopy", referral: "cervical screening referral", report: "colposcopy report" },
  lung_nodule: { procedure: "follow-up CT", referral: "lung nodule referral", report: "CT report" },
  diabetic_retinopathy: { procedure: "eye-clinic exam", referral: "diabetic eye screening referral", report: "eye exam report" },
  breast: { procedure: "breast assessment", referral: "breast screening referral", report: "breast assessment report" },
  lung_ldct: { procedure: "LDCT follow-up", referral: "lung cancer screening referral", report: "LDCT report" },
  ifhas: { procedure: "IFHAS follow-up", referral: "IFHAS screening referral", report: "IFHAS results report" },
};

const BASIS: Record<string, string> = { eid: "Emirates ID", p_cal: "name and details", wasl: "name and details", human: "a person", below_tau: "below the line" };

/** Reader fields arrive as `caecum/prep/tissue/histology`, e.g. `yes/not_stated/no/no`. */
const PLAIN_FIELD: Record<string, string> = {
  "Caecum reached": "whether the caecum was reached",
  "Prep graded": "the bowel prep grade",
  "Tissue taken": "whether tissue was taken",
  "Histology back": "the histology",
};
const READER_WORD: Record<string, string> = { yes: "yes", no: "none", not_stated: "not stated", adequate: "adequate", inadequate: "not adequate" };

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
  const copy = PROGRAMME_COPY[c.case.programme];
  const guided = c.case.programme === "crc";
  const checks = guided ? checksFrom(ev?.fields) : [];
  /*
   * The API names the guard that raised the link item: "second_identifier" means no exact date of birth, and no Emirates ID
   * with agreeing name, sex and date of birth, was on file for the pair. It does not say which side lacks it, so the screen
   * says only that the second identifier is not confirmed.
   */
  const noSecondId = linkItem?.reason === "second_identifier";
  const disputed = checks
    .filter((k) => k.disputed)
    .map((k) => ({
      field: k.field,
      says:
        k.rules === k.second
          ? `neither reader found ${PLAIN_FIELD[k.field] ?? k.field.toLowerCase()}`
          : `the readers differ on ${PLAIN_FIELD[k.field] ?? k.field.toLowerCase()}: rules reader ${READER_WORD[k.rules] ?? k.rules}, second reader ${READER_WORD[k.second] ?? k.second}`,
    }));
  const evidenceHold = guided
    ? disputed.length > 0 && `the two readers could not settle ${disputed.map((d) => PLAIN_FIELD[d.field] ?? d.field).join(" and ")}`
    : ev && !ev.agree && `${copy.report} readers disagree`;
  const held = [noSecondId && "no second identifier is confirmed for the two records", evidenceHold].filter((r): r is string => !!r);

  const identity: IdentityRow[] = [
    { field: "Name", referral: a?.recorded_name ?? "", report: b?.recorded_name ?? "", verdict: "agrees" },
    {
      field: "Emirates ID",
      referral: "on record",
      report: link?.weakest_edge.basis === "eid" ? "on record" : "not on report",
      verdict: link?.weakest_edge.basis === "eid" ? "matches" : "missing",
    },
    {
      field: "Second identifier",
      referral: "on record",
      report: noSecondId ? "not confirmed" : "on record",
      verdict: noSecondId ? "missing" : "agrees",
    },
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
  add("referral", providerName(c.case.provider), `referred for ${copy.procedure}`, sr?.day ?? c.case.referral_day);
  const consent = c.timeline.find((e) => e.type === "Consent" && e.status === "permit");
  add("consent", a?.display_name.split(" ")[0] ?? "Patient", "allowed messages", consent?.day);
  const intake = c.timeline.find((e) => e.category === "referral-intake");
  add("intake", unit, "accepted the referral", intake?.day);
  /** The guard held the appointment reminder when that appointment reached Wasl, before the report existed. */
  const guardEvent = typeof guard?.event === "string" ? c.timeline.find((e) => e.id === guard.event) : undefined;
  if (linkItem)
    add(
      "link",
      "Wasl",
      `matched on ${BASIS[link?.weakest_edge.basis ?? ""] ?? "the record"}; ${noSecondId ? "no second identifier confirmed" : "held"}, needs a person`,
      guardEvent?.arrival_day ?? reportEvent?.day,
    );
  add("report", unit, `${copy.report} ${shortId(ev?.report ?? "")}`, reportEvent?.day);
  if (ev && !ev.agree && c.case.programme === "crc" && disputed.length)
    add("readers", "Two readers", `could not settle ${disputed.map((d) => PLAIN_FIELD[d.field] ?? d.field).join(" and ")}; needs a person`, reportEvent?.day);
  else if (ev && !ev.agree) add("readers", "Two readers", `${copy.report} readers disagree; needs a person`, reportEvent?.day);

  return {
    caseId: c.case.id,
    closed: c.case.closed,
    guided,
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
    referralSource: `${providerName(c.case.provider)} · ${copy.referral}`,
    reportSource: `${unit} · ${copy.report}`,
    unit,
    held: held.length ? `Held: ${held.join(", and ")}.` : "",
    disputed,
    referredOn: formatDay(c.case.referral_day),
    report: { id: shortId(ev?.report ?? ""), on: reportEvent ? formatDay(reportEvent.day) : "" },
    link: {
      score: link?.path_p_cal ?? 0,
      line: link?.tau ?? 0,
      basis: BASIS[link?.weakest_edge.basis ?? ""] ?? "",
      rule: typeof guard?.rule === "string" ? guard.rule : "",
    },
    identity,
    identitySummary: noSecondId
      ? `Matched on ${BASIS[link?.weakest_edge.basis ?? ""] ?? "the record"}, but no second identifier is confirmed. A person must confirm.`
      : "A person must confirm the match.",
    checks,
    quote: ev?.quote ?? "",
    readerModel: ev?.reader_model ?? "",
    items: { link: linkItem?.item, evidence: evidenceItem?.item },
    trail,
  };
}
