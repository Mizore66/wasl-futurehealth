/**
 * The IFHAS evidence label, built one domain at a time instead of picked from 304 raw strings.
 *
 * pipeline/ifhasrule.go defines two label families, and the review endpoint accepts both on an IFHAS case
 * (ifhasEvAlt in pipeline/ifhas.go):
 * - a screening label, every domain with one cell: "bp=<N|H|S>;glyc=<N|P|D|S>;lipid=<N|H>;weight=<N|O>" (ifhasRule);
 * - a follow-up document label: "dna", or "<domain>=<managed|not_confirmed|pending>;..." over the domains the
 *   document mentions, at least one, keys in sorted order as ifhasFormat writes them (ifhasNoteParse).
 * The composer only writes strings in those shapes; the server action still checks the result against
 * GUIDED_EVIDENCE_LABELS.ifhas for the item it signs.
 */

/** In the order ifhasFormat writes them: sorted, which is also the order of ifhasDomains. */
export const IFHAS_DOMAINS = ["bp", "glyc", "lipid", "weight"] as const;
export type IfhasDomain = (typeof IFHAS_DOMAINS)[number];

export const IFHAS_DOMAIN_WORDS: Record<IfhasDomain, string> = { bp: "Blood pressure", glyc: "Glycaemia", lipid: "Lipids", weight: "Weight" };

/** Screening cells per domain, with what ifhasCells means by each (pipeline/ifhasrule.go). */
export const IFHAS_SCREEN_CELLS: Record<IfhasDomain, readonly (readonly [cell: string, words: string])[]> = {
  bp: [
    ["N", "normal"],
    ["H", "high"],
    ["S", "severe"],
  ],
  glyc: [
    ["N", "normal"],
    ["P", "prediabetes"],
    ["D", "diabetes"],
    ["S", "severe"],
  ],
  lipid: [
    ["N", "normal"],
    ["H", "high"],
  ],
  weight: [
    ["N", "normal"],
    ["O", "obese"],
  ],
};

/** Follow-up statuses (IfhasManaged, IfhasNotConfirmed, IfhasPending). A domain the document does not mention is left out. */
export const IFHAS_FOLLOWUP_STATUSES: readonly (readonly [status: string, words: string])[] = [
  ["managed", "managed"],
  ["not_confirmed", "not confirmed"],
  ["pending", "pending"],
];

export type IfhasKind = "screening" | "followup" | "dna";
export const IFHAS_KINDS: [IfhasKind, string][] = [
  ["screening", "Screening result"],
  ["followup", "Follow-up document"],
  ["dna", "Did not attend"],
];

/** A label in progress: the family, and the value chosen for each domain so far ("" or absent = not chosen / not mentioned). */
export type IfhasDraft = { kind?: IfhasKind; values: Partial<Record<IfhasDomain, string>> };

const allowed = (kind: IfhasKind, domain: IfhasDomain): string[] =>
  kind === "screening" ? IFHAS_SCREEN_CELLS[domain].map(([cell]) => cell) : IFHAS_FOLLOWUP_STATUSES.map(([status]) => status);

/** The canonical label for a finished draft, or "" while it is incomplete. */
export function composeIfhasLabel(draft: IfhasDraft): string {
  const { kind, values } = draft;
  if (kind === "dna") return "dna";
  if (!kind) return "";
  const parts: string[] = [];
  for (const domain of IFHAS_DOMAINS) {
    const value = values[domain];
    if (!value) {
      if (kind === "screening") return ""; // a screening label needs every domain
      continue; // a follow-up label lists only the domains the document mentions
    }
    if (!allowed(kind, domain).includes(value)) return "";
    parts.push(`${domain}=${value}`);
  }
  return parts.join(";");
}

/** The draft a label came from, so a saved answer shows in the controls. Anything else is an empty draft. */
export function parseIfhasLabel(label: string | undefined): IfhasDraft {
  if (!label) return { values: {} };
  if (label === "dna") return { kind: "dna", values: {} };
  const values: Partial<Record<IfhasDomain, string>> = {};
  for (const part of label.split(";")) {
    const [domain, value, ...rest] = part.split("=");
    if (rest.length > 0 || !(IFHAS_DOMAINS as readonly string[]).includes(domain) || values[domain as IfhasDomain] !== undefined) return { values: {} };
    values[domain as IfhasDomain] = value;
  }
  const kind: IfhasKind = IFHAS_DOMAINS.some((d) => IFHAS_SCREEN_CELLS[d].some(([cell]) => cell === values[d])) ? "screening" : "followup";
  const draft = { kind, values };
  return composeIfhasLabel(draft) === label ? draft : { values: {} };
}

/** Every label the composer can produce, in control order. */
export function composableIfhasLabels(): string[] {
  const out = [composeIfhasLabel({ kind: "dna", values: {} })];
  for (const kind of ["screening", "followup"] as const) {
    const choices = (domain: IfhasDomain) => (kind === "screening" ? allowed(kind, domain) : ["", ...allowed(kind, domain)]);
    const visit = (i: number, values: IfhasDraft["values"]) => {
      if (i === IFHAS_DOMAINS.length) {
        const label = composeIfhasLabel({ kind, values });
        if (label) out.push(label);
        return;
      }
      for (const value of choices(IFHAS_DOMAINS[i])) visit(i + 1, { ...values, [IFHAS_DOMAINS[i]]: value });
    };
    visit(0, {});
  }
  return out;
}
