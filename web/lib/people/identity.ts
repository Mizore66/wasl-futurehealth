import type { CaseDetail } from "@/lib/api/schema";
import { people } from ".";
import { effectiveReviews, finalDecisions } from "@/lib/domain/stage";
import type { PersonSeed } from "./types";

/*
 * Who a record's avatar shows. A face belongs to a person, and the person is the referral patient of a loop.
 * A downstream record wears that face only once the system trusts its link; until then it shows "?".
 */

/**
 * The referral patient of a loop. Fixture loops key on the referral registration from data/people.json.
 * Case summaries carry no registration, so every other loop keys on its referral id, which names one
 * referral and so one patient; list rows and records then draw the same face.
 */
export function referralSeed(loop: string): PersonSeed {
  const provider = loop.split("/")[0];
  const p = Object.values(people.people).find((x) => x.loop === loop && x.provider === provider);
  return p ? { key: p.registration, sex: p.sex } : { key: loop };
}

/**
 * How far the system trusts a link:
 * - `auto`: calibrated p at or above τ, and no review item holds it.
 * - `signed`: a person confirmed it is the same patient.
 * - `held`: a link review item is open (for example a missing second identifier), even if p ≥ τ.
 * - `below`: under τ and nobody has signed.
 * - `rejected`: a person said it is not the same patient.
 */
export type Trust = "auto" | "signed" | "held" | "below" | "rejected";

export const TRUSTED: Record<Trust, boolean> = { auto: true, signed: true, held: false, below: false, rejected: false };

export type LinkFacts = Pick<CaseDetail, "links" | "review_items" | "signoffs" | "api_reviews" | "api_review_states">;
type Link = CaseDetail["links"][number];

type Pair = readonly [string, string];

function registrationPair(value: unknown): Pair | undefined {
  if (!Array.isArray(value) || value.length !== 2) return undefined;
  const [a, b] = value;
  return typeof a === "string" && a.trim() !== "" && typeof b === "string" && b.trim() !== "" && a !== b ? [a, b] : undefined;
}

// Orientation is immaterial; the complete provider-qualified key (or API pseudonym) is not.
function samePair(a: Pair, b: Pair): boolean {
  return (a[0] === b[0] && a[1] === b[1]) || (a[0] === b[1] && a[1] === b[0]);
}

function directPair(link: Link): Pair | undefined {
  // The API exposes the weakest edge, not the full path or its downstream registration. Only an
  // unsubstituted one-hop edge containing the referral identifies both endpoints unambiguously.
  if (link.hops !== 1 || link.review_pair_substituted === true) return undefined;
  const pair = registrationPair([link.weakest_edge.a, link.weakest_edge.b]);
  return pair?.includes(link.referral_patient) ? pair : undefined;
}

export function linkTrust(c: LinkFacts, link: Link): Trust {
  const pair = directPair(link);
  if (!pair) return "held";
  // Historical exports can contain more than one association for an event. Do not guess which
  // registration a review confirms when those associations disagree or lack endpoint metadata.
  if (c.links.some((l) => l.loop === link.loop && l.downstream_event === link.downstream_event &&
    (!directPair(l) || !samePair(directPair(l)!, pair)))) return "held";

  const linkItems = c.review_items.filter((r) => r.loop === link.loop && r.kinds.includes("link"));
  if (linkItems.some((r) => !registrationPair(r.pair))) return "held";
  const itemPair = (item: string): Pair | undefined => {
    const rows = c.review_items.filter((r) => r.item === item);
    const first = rows[0] && registrationPair(rows[0].pair);
    return first && rows.every((r) => r.loop === link.loop && r.kinds.includes("link") &&
      registrationPair(r.pair) && samePair(registrationPair(r.pair)!, first)) ? first : undefined;
  };
  const items = linkItems.filter((r) => samePair(registrationPair(r.pair)!, pair)).map((r) => r.item);
  // An item ID must resolve to one pair, even if conflicting rows would otherwise be filtered out.
  if (items.some((item) => !itemPair(item))) return "held";

  const canonical: string[] = [];
  for (const s of c.signoffs.filter((s) => s.loop === link.loop && s.kinds.includes("link"))) {
    // Live signoffs carry a pair; older fixture signoffs join through their raised item instead.
    const explicit = registrationPair(s.pair);
    const raised = itemPair(s.item);
    if ((s.pair !== undefined && !explicit) ||
      (c.review_items.some((r) => r.item === s.item) && !raised) ||
      (explicit && raised && !samePair(explicit, raised))) return "held";
    const signedPair = explicit ?? raised;
    if (!signedPair) return "held";
    if (samePair(signedPair, pair)) canonical.push(s.link);
  }
  const legacy = effectiveReviews(c).filter((r) => r.case === link.loop);
  const states = (c.api_review_states ?? []).filter((s) => s.question === "link");
  // An orphan link answer cannot establish either that this pair is signed or that it has no hold.
  if (legacy.some((r) => r.action === "signoff_link" && !itemPair(r.item)) ||
    states.some((s) => !itemPair(s.item))) return "held";
  const said = [
    ...canonical,
    // Compensation stays item-scoped. Initial/own protocol reads are never terminal answers.
    ...legacy.flatMap((r) => (items.includes(r.item) ? [r.decision] : [])),
    ...finalDecisions({ api_review_states: states.filter((s) => items.includes(s.item)) }, "link"),
  ];
  if (said.includes("different")) return "rejected";
  if (said.includes("same")) return "signed";
  if (items.length > 0) return "held";
  return link.path_p_cal >= link.tau ? "auto" : "below";
}

/** What an avatar draws: a person, shown as their face or, while the link is not trusted, as "?". */
export type Face = { person: PersonSeed; state: "known" | "unknown" };

/** The face of one of the case's records, by registration. */
export function recordFace(c: CaseDetail, registration: string): Face {
  const referral = referralSeed(c.case.id);
  const links = c.links.filter((l) => l.loop === c.case.id);
  const isReferral = c.identities.find((i) => i.registration === registration)?.role === "referral" ||
    links.some((l) => l.referral_patient === registration);
  if (isReferral) return { person: referral, state: "known" };
  const matching = links.filter((l) => directPair(l)?.includes(registration) && l.referral_patient !== registration);
  const link = matching[0];
  // Never fall back to another registration's link, nor treat a multi-hop intermediate as its endpoint.
  if (!link || matching.some((l) => !samePair(directPair(l)!, directPair(link)!))) return { person: referral, state: "unknown" };
  const trust = linkTrust(c, link);
  if (trust === "rejected") return { person: { key: registration }, state: "known" };
  return { person: referral, state: TRUSTED[trust] ? "known" : "unknown" };
}

/** The case as it stood before any person answered its review items. */
export function beforeReview<C extends LinkFacts>(c: C): C {
  return { ...c, signoffs: [], api_reviews: [], api_review_states: [] };
}
