import { plain } from "@/lib/domain/terms";

/** Plain words for the read names and tools in the agent lane. Unknown ids fall back to plain(). */
const READ_WORDS: Record<string, string> = {
  r1: "Human reviewer, first read",
  r2: "Human reviewer, second read",
  agent: "Evidence agent (experimental)",
};

export const readWords = (r: string): string => READ_WORDS[r] ?? plain(r);

const TOOL_WORDS: Record<string, string> = {
  get_record: "Looked up a registration record",
};

export const toolWords = (t: string): string => TOOL_WORDS[t] ?? plain(t);

const FIELD_WORDS: Record<string, string> = {
  name: "Name",
  sex: "Sex",
  script: "Writing system",
  dob: "Date of birth",
};

export const fieldWords = (f: string): string => FIELD_WORDS[f] ?? plain(f);

const LABEL_WORDS: Record<string, string> = {
  same: "Same person",
  different: "Different people",
};

export const labelWords = (l: string): string => LABEL_WORDS[l] ?? plain(l);

/** A record reference such as `reg_533471be2e678a419e653ab0ed53f8e1`, shortened for reading. */
export function refWords(ref: string): string {
  const i = ref.indexOf("_");
  const tail = i >= 0 ? ref.slice(i + 1) : ref;
  return `${i >= 0 ? ref.slice(0, i + 1) : ""}${tail.slice(0, 8)}…`;
}

/** `provider|MRN` pairs from the link review, as two parts. */
export function recordParts(pair: string): { provider: string; mrn: string } {
  const [provider, mrn] = pair.split("|");
  return { provider: provider ?? pair, mrn: mrn ?? "" };
}

/** The reviewer string is `sim-clinician e=0.000 double=merge+close reads=[r1 r2]+agent`; the first word names who and how. */
export function signerWords(reviewer: string, role: string): string {
  const simulated = reviewer.startsWith("sim-");
  const who = role === "clinician" ? "clinician" : role === "reviewer" ? "link reviewer" : plain(role).toLowerCase();
  return simulated ? `Simulated ${who}` : `A ${who}`;
}

/** The auto-close line out of the policy string, if it has one. */
export function tauOf(policy: string): string | null {
  return /tau=([0-9.]+)/.exec(policy)?.[1] ?? null;
}
