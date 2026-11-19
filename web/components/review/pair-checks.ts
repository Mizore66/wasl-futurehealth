import type { IdentityRow, Verdict } from "./case-view";

/** How one identifier row reads in the record pair: differences stand out, matches stay quiet. */
export type PairState = "differ" | "match" | "missing" | "unclear" | "separate";

const FROM_VERDICT: Record<Verdict, PairState> = {
  match: "match",
  differ: "differ",
  missing: "missing",
  placeholder: "unclear",
  unknown: "unclear",
  invalid: "unclear",
  unavailable: "unclear",
  separate: "separate",
};

export const PAIR_WORD: Record<PairState, string> = {
  differ: "Differs",
  match: "Matches",
  missing: "Missing",
  unclear: "Not compared",
  separate: "Separate records",
};

const ARABIC = /[؀-ۿ]/;
const LATIN = /[A-Za-z]/;
const ABSENT = /^(?:|not recorded|unknown)$/i;
const plain = (v: string) => v.normalize("NFKC").trim().replace(/\s+/g, " ").toLowerCase();

/**
 * The row's state, from the data on screen. The API's comparison verdict decides when it gave one; only a row the API
 * did not compare is compared here, from the two values themselves. Names in different scripts are never called a
 * difference: one may be a transliteration of the other.
 */
export function pairState(row: IdentityRow): PairState {
  if (row.verdict !== "unavailable") return FROM_VERDICT[row.verdict];
  const [a, b] = [row.referral, row.report].map(plain);
  if (ABSENT.test(a) || ABSENT.test(b)) return "missing";
  if (a === b) return "match";
  const scripts = [row.referral, row.report].map((v) => (ARABIC.test(v) ? (LATIN.test(v) ? "mixed" : "ar") : "latin"));
  return scripts[0] === scripts[1] && scripts[0] !== "mixed" ? "differ" : "unclear";
}

/** The rows the pair-checks summary lists: every identifier except the record ids, which are always two records. */
export function pairChecks(rows: IdentityRow[]): { field: string; state: PairState }[] {
  return rows.filter((r) => r.verdict !== "separate").map((r) => ({ field: r.field, state: pairState(r) }));
}
