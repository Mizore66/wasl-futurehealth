import type { CaseDetail } from "@/lib/api/schema";
import { people } from ".";
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

export type LinkFacts = Pick<CaseDetail, "links" | "review_items" | "signoffs" | "api_reviews">;
type Link = CaseDetail["links"][number];

export function linkTrust(c: LinkFacts, link: Link): Trust {
  const items = c.review_items.filter((r) => r.loop === link.loop && r.kinds.includes("link")).map((r) => r.item);
  const said = [
    ...c.signoffs.filter((s) => s.loop === link.loop && s.kinds.includes("link")).map((s) => s.link),
    ...c.api_reviews.filter((r) => items.includes(r.item)).map((r) => r.decision),
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
  const link = c.links.find((l) => l.referral_patient === registration || l.weakest_edge.b === registration) ?? c.links[0];
  const isReferral = c.identities.find((i) => i.registration === registration)?.role === "referral" || link?.referral_patient === registration;
  if (isReferral) return { person: referral, state: "known" };
  if (!link) return { person: referral, state: "unknown" };
  const trust = linkTrust(c, link);
  if (trust === "rejected") return { person: { key: registration }, state: "known" };
  return { person: referral, state: TRUSTED[trust] ? "known" : "unknown" };
}

/** The case as it stood before any person answered its review items. */
export function beforeReview<C extends LinkFacts>(c: C): C {
  return { ...c, signoffs: [], api_reviews: [] };
}
