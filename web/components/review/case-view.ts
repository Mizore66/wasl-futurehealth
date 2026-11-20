import { Evidence, evidenceLabelsFor, GUIDED_EVIDENCE_LABELS, isGuided, Link as LinkFacts, type CaseDetail, type Programme } from "@/lib/api/schema";
import type { ChainNode } from "@/components/wasl/sealed-chain";
import { formatDay } from "@/lib/domain/day";
import { caseHref, shortId } from "@/lib/domain/ids";
import { personFor } from "@/lib/people";
import { recordFace, type Face } from "@/lib/people/identity";
import { purposeOf, type ReviewItems, type ReviewPurpose } from "./decision";

/** CRC evidence items raised for one narrow decision; any other reason is the ordinary report-adequacy review. */
export const CRC_PURPOSES = {
  prep_ungraded_day15: {
    field: "Prep graded",
    question: "Was the bowel preparation adequate?",
    scope: "This answer settles only this question: the bowel preparation. Histology and other closure requirements can still keep the case open.",
  },
  completion_undocumented: {
    field: "Caecum reached",
    question: "Was the caecum reached?",
    scope: "This answer settles only this question: whether the caecum was reached. Histology and other closure requirements can still keep the case open.",
  },
} as const;
export type CrcPurpose = keyof typeof CRC_PURPOSES;

/** Item reasons the image reader raises or routes; the clinician judges the photographs, so they must be shown. */
const IMAGE_REASONS = ["completion_undocumented", "prep_grade_one_click", "image_prep_conflict"];

export type PhotoView = { sha: string; station: string; available: boolean; src: string };
export type ImageContext = { model: string; caecal: string; prep: string; hint: string; textPrep: string };
const crcPurposeOf = (reason: string | undefined): CrcPurpose | undefined => (reason && reason in CRC_PURPOSES ? (reason as CrcPurpose) : undefined);

/*
 * Everything the review desk shows, derived from one case detail.
 * Names come from data/people.json (gap G1, simulated); every other value is the API's.
 */

export type Verdict = "match" | "differ" | "missing" | "placeholder" | "unknown" | "invalid" | "unavailable" | "separate";
export type IdentityRow = { field: string; referral: string; report: string; verdict: Verdict };

export type ReaderValue = "yes" | "no" | "not_stated" | "adequate" | "inadequate";
export type CheckDecision = "agreed" | "agreed_none" | "not_needed" | "disputed";
export type Check = { field: string; rules: ReaderValue; second: ReaderValue; decision: CheckDecision; disputed: boolean };

export type AutomatedReaderFields = string | { quote?: string; [key: string]: string | number | boolean | undefined };
/** Only automated facts from this item's selected evidence block cross the client boundary. */
export type AutomatedReaderEvidence = {
  report: string;
  agree: boolean;
  label: string;
  quote: string;
  reader_model?: string;
  reader_a_regex?: string;
  reader_a?: string;
  reader_b?: string;
  reader_a_fields?: AutomatedReaderFields;
  reader_b_fields?: AutomatedReaderFields;
};

export type ReviewView = {
  caseId: string;
  /** Canonical case state from the latest server render, never inferred from a 201. */
  closed: boolean;
  /** CRC and the five API-supported non-CRC programmes use the guided review desk. */
  guided: boolean;
  /** The raised evidence item asks about a CRC pathology report: histology or no histology, not colonoscopy adequacy. */
  pathology: boolean;
  programme: Programme;
  evidenceQuestion: string;
  /** Set when the evidence item asks one narrow CRC question; the answer does not decide whole-case closure. */
  evidenceScope?: string;
  /** The one check row a narrow answer settles; every other disputed row (histology included) stays open. */
  evidenceField?: string;
  evidenceChoices: string[];
  crcItems?: Record<string, { title: string; choices: string[] }>;
  /** The evidence item was raised on photographic evidence: photos are shown, or their absence is said. */
  imageBacked: boolean;
  /** The photos the API filed on this item's report at the station the item asks about; never inferred from scores. */
  photos: PhotoView[];
  /** The image model's read of those photos, shown only after the clinician's own first read. */
  imageContext?: ImageContext;
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
  /** Set when the evidence item is a pilot review: a suggested closure to confirm, or an audit of a closed case. */
  purpose?: ReviewPurpose;
  /** The checks the two readers could not settle: what the one evidence answer covers. */
  disputed: { field: string; says: string }[];
  referredOn: string;
  report: { id: string; on: string; version?: number };
  referralRegistration: string;
  link: { score: number; line: number; basis: string; rule: string };
  identity: IdentityRow[];
  identitySummary: string;
  /** Availability of the registration comparison, never original item-time or guard authority. */
  identityAvailable: boolean;
  /** Explicit source qualification and original configured guard unavailability; safe to serialize. */
  identityContext: string;
  /** Limited visible-field diagnostics, never the original configured guard result. */
  identityGuard: string[];
  checks: Check[];
  quote: string;
  readerModel: string;
  /** No human override, independent-human answers or loose unknown evidence fields. */
  automatedEvidence?: AutomatedReaderEvidence;
  items: ReviewItems;
  /**
   * Whether the exact context each question needs is on screen. False when a raised item's own pair or own report cannot be
   * established from the case; nothing is then borrowed from another item and the question is shown but cannot be answered.
   * This is structural binding only: it does not say the context is current, original or guard-checked.
   */
  context: { link: boolean; evidence: boolean };
  trail: ChainNode[];
};

const PROVIDERS: Record<string, string> = {
  "ahs-mushrif": "Al Mushrif",
  "noor-endo": "Noor endoscopy",
  "skmc-endo": "SKMC endoscopy",
};
const providerName = (id: string) => PROVIDERS[id] ?? id;

const PROGRAMME_COPY: Record<Programme, { procedure: string; referral: string; report: string }> = {
  crc: { procedure: "colonoscopy", referral: "stool test referral", report: "colonoscopy report" },
  cervical: { procedure: "colposcopy", referral: "cervical screening referral", report: "colposcopy report" },
  lung_nodule: { procedure: "follow-up CT", referral: "lung nodule referral", report: "CT report" },
  diabetic_retinopathy: { procedure: "eye-clinic exam", referral: "diabetic eye screening referral", report: "eye exam report" },
  breast: { procedure: "breast assessment", referral: "breast screening referral", report: "breast assessment report" },
  lung_ldct: { procedure: "LDCT follow-up", referral: "lung cancer screening referral", report: "LDCT report" },
  ifhas: { procedure: "IFHAS follow-up", referral: "IFHAS screening referral", report: "IFHAS results report" },
};

const SEX_WORD: Record<string, string> = { F: "Female", M: "Male" };
const GUARD_WORDS: Record<string, string> = {
  dob_missing: "Date of birth is not recorded on one or both records.",
  dob_placeholder: "A date of birth is a placeholder (1 January or 1 July), so it does not confirm the match.",
  dob_differ: "The dates of birth differ.",
  dob_incompatible: "The dates of birth are not compatible.",
  eid_missing: "Emirates ID is not recorded on one or both records.",
  eid_placeholder: "An Emirates ID is a default or placeholder number.",
  eid_invalid: "An Emirates ID fails its check digit.",
  eid_differ: "The Emirates IDs differ.",
  eid_second_disabled: "The EID path is disabled for this visible-field check.",
  sex_unknown: "Sex is unknown on one or both records, so a matching Emirates ID does not confirm the match.",
  sex_differ: "The recorded sexes differ.",
};
const IDENTIFIER_ROWS = [["name", "Name"], ["dob", "Date of birth"], ["sex", "Sex"], ["eid", "Emirates ID"]] as const;

function identifierCell(field: string, v: { state: string; value: string }): string {
  if (v.state === "present") return field === "sex" ? (SEX_WORD[v.value] ?? v.value) : v.value;
  if (v.state === "missing") return "not recorded";
  if (v.state === "unknown") return "unknown";
  return `${v.value} (${v.state})`.trim();
}

type ComparedIdentity = Extract<NonNullable<CaseDetail["identity_review"]>[number], { available: true }>;

/**
 * The phone row, only when at least one record has a number. Values are masked by the server (last three digits) and
 * the difference marking follows the server's verdict; runs served before phone was materialized have no row.
 */
function phoneRows(compared: ComparedIdentity): IdentityRow[] {
  const [a, b] = [compared.records[0].phone, compared.records[1].phone];
  if (a?.state !== "present" && b?.state !== "present") return [];
  const cell = (v: typeof a) => (v ? identifierCell("phone", v) : "not recorded");
  return [{ field: "Phone", referral: cell(a), report: cell(b), verdict: compared.comparison.find((x) => x.field === "phone")?.verdict ?? "unavailable" }];
}

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

export const PURPOSE_LABEL: Record<ReviewPurpose, string> = { shadow_confirm: "Suggested closure", auto_audit: "Post-closure audit" };

const PURPOSE_SUMMARY: Record<ReviewPurpose, (closed: boolean) => string> = {
  shadow_confirm: (closed) =>
    `Suggested automatic closure to confirm. The case is ${closed ? "closed" : "open"}. Confirming permits closure; a different answer is followed instead.`,
  auto_audit: (closed) =>
    `Post-closure audit of an automatic closure. The case is ${closed ? "closed" : "open"}. Agreeing leaves it closed; disagreeing reopens it.`,
};

type RaisedItem = CaseDetail["review_items"][number];

// Canonical reader wire fields (lungread/drread/breastread/ldctread.go), not a
// classifier. IFHAS's existing reader fields are automated provenance notes.
const AUTOMATED_FIELDS: Partial<Record<CaseDetail["case"]["programme"], readonly string[]>> = {
  lung_nodule: ["type", "long_mm", "short_mm", "count", "solid_mm", "prior_long_mm", "prior_short_mm", "resolved", "not_assessable", "stated", "mean_mm"],
  diabetic_retinopathy: ["r_right", "r_left", "m_right", "m_left", "urgent", "attended", "fundus", "outcome", "review_days"],
  breast: ["birads_right", "birads_left", "birads_overall", "attended", "assessment", "biopsy", "outcome"],
  lung_ldct: ["lungrads_overall", "lungrads_worst", "attended", "workup", "mdt", "tissue", "outcome"],
};

function automatedFields(value: unknown, programme: CaseDetail["case"]["programme"]): AutomatedReaderFields | undefined {
  if (programme === "ifhas" && typeof value === "string") return value;
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const source = value as Record<string, unknown>;
  const fields: Exclude<AutomatedReaderFields, string> = {};
  for (const key of [...(AUTOMATED_FIELDS[programme] ?? []), "label", "quote"]) {
    const field = source[key];
    if (typeof field === "string" || (key !== "quote" && key !== "label" && (typeof field === "boolean" || (typeof field === "number" && Number.isFinite(field))))) {
      fields[key] = field;
    }
  }
  return fields;
}

function automatedSnapshot(ev: CaseDetail["evidence"][number] | undefined, programme: CaseDetail["case"]["programme"]): AutomatedReaderEvidence | undefined {
  if (!ev) return undefined;
  // Deliberately no spreads from a loose API object or its chain/history.
  return {
    report: ev.report,
    agree: ev.agree,
    label: ev.label,
    quote: ev.quote,
    reader_model: ev.reader_model,
    reader_a_regex: ev.reader_a_regex,
    reader_a: ev.reader_a,
    reader_b: ev.reader_b,
    reader_a_fields: automatedFields(ev.reader_a_fields, programme),
    reader_b_fields: automatedFields(ev.reader_b_fields, programme),
  };
}

/** The evidence one raised item judges: its own chain's block first, so a revised report keeps its own version's readers and quote. */
function evidenceFromChain(item: RaisedItem) {
  const block = item.chain.find((e) => e.type === "evidence" && shortId(String(e.body.report)) === shortId(item.report));
  const parsed = block && Evidence.safeParse(block.body);
  return parsed?.success ? parsed.data : undefined;
}

function evidenceFor(c: CaseDetail, item: RaisedItem | undefined, only: boolean) {
  if (!item) return only ? undefined : c.evidence[0];
  return evidenceFromChain(item) ?? c.evidence.find((e) => shortId(e.report) === shortId(item.report));
}

/** The link decision in a raised item's own chain: the item's original link context, which may be a multi-hop path whose weakest edge is not the outreach pair. */
function linkFromChain(item: RaisedItem): CaseDetail["links"][number] | undefined {
  const block = item.chain.find((e) => e.type === "link_decision");
  const parsed = block && LinkFacts.safeParse(block.body);
  return parsed?.success && parsed.data.loop === item.loop ? parsed.data : undefined;
}

/** With `only`, everything shown is derived from that one raised item; without it, from the case's first link and evidence items. */
export function reviewView(c: CaseDetail, only?: RaisedItem): ReviewView {
  if (c.case.lifecycle === "retained") throw new Error("Retained cases have no current review work");
  const raised = only ? [only] : c.review_items;
  const linkItem = raised.find((r) => r.kinds.includes("link"));
  const evidenceItem = raised.find((r) => r.kinds.includes("evidence"));
  /*
   * A view of one raised item takes its endpoints only from that item's own pair, by exact registration, and its link details only
   * from that item's own chain or the exact edge. Nothing is borrowed from the referral, the first downstream record or links[0].
   */
  const pair = linkItem?.pair ?? evidenceItem?.pair;
  const strict = Boolean(only && linkItem);
  const exact = (registration: string | undefined) => (registration ? c.identities.find((i) => i.registration === registration) : undefined);
  const referral = strict ? exact(pair?.[0]) : (c.identities.find((i) => pair && i.registration === pair[0]) ?? c.identities.find((i) => i.role === "referral"));
  // An evidence-only item's absent registration is not the case's first downstream record either.
  const downstream = only ? exact(pair?.[1]) : (c.identities.find((i) => pair && i.registration === pair[1]) ?? c.identities.find((i) => i.role === "downstream"));
  const a = referral && personFor(referral.registration);
  const b = downstream && personFor(downstream.registration);
  const ev = evidenceFor(c, evidenceItem, Boolean(only));
  const chainLink = strict && linkItem ? linkFromChain(linkItem) : undefined;
  /* An accepted case-link fallback must be the exact edge for this pair and this item's own loop; another loop's edge is not borrowed. */
  const edge = (l: CaseDetail["links"][number]) =>
    Boolean(pair) && l.weakest_edge.a === pair?.[0] && l.weakest_edge.b === pair?.[1] && (!strict || l.loop === linkItem?.loop);
  const endpoints = pair?.length === 2 && Boolean(referral) && Boolean(downstream);
  const link = strict ? (endpoints ? (chainLink ?? c.links.find(edge)) : undefined) : (c.links.find(edge) ?? c.links[0]);
  /* Strict views need both exact endpoints and the item's own link detail; absent either, no score or line is invented. */
  const linkBound = !strict || (endpoints && Boolean(link));
  const guard = linkItem?.chain.find((e) => e.type === "outreach_guard")?.body;
  const reportEvents = c.timeline.filter((e) => ev && e.id === ev.report);
  // Timeline rows have no report-version association. A versioned item's date
  // cannot come from an unversioned historical row, even when it is the only one.
  const reportEvent = only && evidenceItem
    ? (evidenceItem.report_version === undefined && reportEvents.length === 1 ? reportEvents[0] : undefined)
    : reportEvents[0];
  // The canonical evidence block carries its own provider-qualified report ID;
  // missing qualification stays unavailable rather than borrowing another pair.
  const evidenceProvider = /^([^/]+)\/DiagnosticReport\/[^/]+$/.exec(ev?.report ?? "")?.[1];
  const unit = only && evidenceItem
    ? (evidenceProvider ? providerName(evidenceProvider) : "Source unavailable")
    : providerName(downstream?.provider ?? "");
  const programme = c.case.programme;
  const copy = PROGRAMME_COPY[programme];
  const guided = isGuided(programme);
  const evidenceChoices = !guided ? [] : evidenceItem ? evidenceLabelsFor(c, evidenceItem.report) : GUIDED_EVIDENCE_LABELS[programme];
  const pathology = evidenceChoices.includes("histology");
  const reportWord = pathology ? "pathology report" : copy.report;
  const crcPurpose = programme === "crc" && !pathology ? crcPurposeOf(evidenceItem?.reason) : undefined;
  const imageBacked = programme === "crc" && !pathology && !!evidenceItem?.reason && IMAGE_REASONS.includes(evidenceItem.reason);
  const evidenceQuestion = pathology
    ? "Does the pathology report show histology?"
    : crcPurpose
      ? CRC_PURPOSES[crcPurpose].question
      : programme === "crc"
        ? "Is the colonoscopy report adequate?"
        : `Select the ${copy.report} evidence label`;
  const checks = programme === "crc" && !pathology ? checksFrom(ev?.fields) : [];
  /* The image model is read only from this exact item's own chain; aggregate evidence never carries it. */
  const chainEvidence = imageBacked && evidenceItem ? evidenceFromChain(evidenceItem) : undefined;
  const imageContext = chainEvidence?.image_reader ? { image_reader: chainEvidence.image_reader, fields: chainEvidence.fields } : undefined;
  /*
   * The recorded reason identifies the question that was raised, not its original per-field or configured guard checks.
   * The identifier projection below is materialized context only; it does not establish an original guard outcome.
   * "second_identifier" means no exact date of birth, and no Emirates ID with agreeing name, sex and date of birth,
   * was on file for the pair; it does not say which side lacks it.
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
  const evidenceHold = programme === "crc" && !pathology
    ? disputed.length > 0 && `the two readers could not settle ${disputed.map((d) => PLAIN_FIELD[d.field] ?? d.field).join(" and ")}`
    : ev && !ev.agree && `${reportWord} readers disagree`;
  const held = [noSecondId && "no second identifier is confirmed for the two records", evidenceHold].filter((r): r is string => !!r);

  const purpose = purposeOf(evidenceItem?.reason);

  const projection = linkItem && c.identity_review?.find((e) => e.item === linkItem.item);
  const compared = projection && projection.available ? projection : undefined;
  const recordRow: IdentityRow = {
    field: "Record",
    referral: referral?.registration.split("|")[1] ?? "",
    report: downstream?.registration.split("|")[1] ?? "",
    verdict: "separate",
  };
  const identity: IdentityRow[] = compared
    ? [
        ...IDENTIFIER_ROWS.map(([key, field]): IdentityRow => ({
          field,
          referral: identifierCell(key, compared.records[0][key]),
          report: identifierCell(key, compared.records[1][key]),
          verdict: compared.comparison.find((x) => x.field === key)?.verdict ?? "unavailable",
        })),
        ...phoneRows(compared),
        recordRow,
      ]
    : [{ field: "Name", referral: a?.recorded_name ?? "", report: b?.recorded_name ?? "", verdict: "unavailable" }, recordRow];
  const identityContext = compared?.source_scope === "materialized_registration"
    ? "These identifiers are materialized across recorded events, not an original item-time snapshot. The original configured guard result is unavailable."
    : "The identifier source scope is unavailable. The original configured guard result is unavailable.";
  const visibleChecks = compared?.guard.visible_checks;
  const identityGuard = visibleChecks && !visibleChecks.met ? visibleChecks.failures.flatMap((code) => GUARD_WORDS[code] ? [GUARD_WORDS[code]] : []) : [];

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
  add("report", unit, `${reportWord} ${shortId(ev?.report ?? "")}`, reportEvent?.day);
  if (ev && !ev.agree && c.case.programme === "crc" && disputed.length)
    add("readers", "Two readers", `could not settle ${disputed.map((d) => PLAIN_FIELD[d.field] ?? d.field).join(" and ")}; needs a person`, reportEvent?.day);
  else if (ev && !ev.agree) add("readers", "Two readers", `${reportWord} readers disagree; needs a person`, reportEvent?.day);

  return {
    caseId: c.case.id,
    closed: c.case.closed,
    guided,
    pathology,
    programme,
    evidenceQuestion,
    evidenceScope: crcPurpose && CRC_PURPOSES[crcPurpose].scope,
    evidenceField: crcPurpose && CRC_PURPOSES[crcPurpose].field,
    evidenceChoices: [...evidenceChoices],
    crcItems: Object.fromEntries(c.review_items.filter((it) => it.reason === "crc_disposition").map((it) => {
      const need = c.case.crc?.obligations.find((p) => p.review_item === it.item);
      return [it.item, { title: need ? `${need.outcome.replaceAll("_", " ")} · ${need.source_report} v${need.source_version}` : "Superseded CRC obligation", choices: need && !need.confirmation && need.blockers.length === 0 && it.verdict ? [it.verdict] : [] }];
    })),
    imageBacked,
    photos: imageBacked && evidenceItem
      ? (c.review_photos?.[evidenceItem.item] ?? []).map((p) => ({ ...p, src: `${caseHref("/app/review", c.case.id)}/photo/${encodeURIComponent(evidenceItem.item)}/${p.sha}` }))
      : [],
    imageContext: imageContext && {
      model: imageContext.image_reader.model,
      caecal: imageContext.image_reader.caecal_photo,
      prep: imageContext.image_reader.prep,
      hint: imageContext.image_reader.hint,
      textPrep: readerValues(imageContext.fields?.regex)[1] ?? "not_stated",
    },
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
    reportSource: `${unit} · ${reportWord}`,
    unit,
    held: purpose ? PURPOSE_SUMMARY[purpose](c.case.closed) : held.length ? `Held: ${held.join(", and ")}.` : "",
    purpose,
    disputed,
    referredOn: formatDay(c.case.referral_day),
    report: { id: shortId(ev?.report ?? ""), on: reportEvent ? formatDay(reportEvent.day) : "", version: evidenceItem?.report_version },
    referralRegistration: referral?.registration ?? "",
    link: {
      score: link?.path_p_cal ?? 0,
      line: link?.tau ?? 0,
      basis: BASIS[link?.weakest_edge.basis ?? ""] ?? "",
      rule: typeof guard?.rule === "string" ? guard.rule : "",
    },
    identity,
    identityAvailable: Boolean(compared),
    identityContext,
    identityGuard,
    identitySummary: !compared
      ? "The identifier comparison is not available for this item, so the two records cannot be compared on this page. A person must confirm."
      : noSecondId
        ? `Matched on ${BASIS[link?.weakest_edge.basis ?? ""] ?? "the record"}, but no second identifier is confirmed. A person must confirm.`
        : "A person must confirm the match.",
    checks,
    quote: ev?.quote ?? "",
    readerModel: ev?.reader_model ?? "",
    automatedEvidence: only && evidenceItem && programme !== "crc" ? automatedSnapshot(ev, programme) : undefined,
    items: { link: linkItem?.item, evidence: evidenceItem?.item, ...(crcPurpose && { scoped: true }) },
    context: { link: linkBound, evidence: !only || !evidenceItem || Boolean(ev) },
    trail,
  };
}

export function reviewItemViews(c: CaseDetail): Record<string, ReviewView> {
  return Object.fromEntries(c.review_items.map((item) => [item.item, reviewView(c, item)]));
}
