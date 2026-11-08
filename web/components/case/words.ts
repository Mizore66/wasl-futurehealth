import type { Audit, CaseDetail, Programme } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";
import { plain } from "@/lib/domain/terms";
import { statusWords } from "@/components/worklist/model";

/*
 * Plain words for what the case and audit endpoints return (issues #1 and #5).
 * Nothing here invents data: every sentence is built from a field the API sent.
 */

type Chain = Audit["chain"][number];
type Hold = CaseDetail["holds"][number];

/** Chain entry types that the shared terms do not cover yet. */
const ENTRY_LABEL: Record<string, string> = {
  recommendation: "Next step recommended",
  outreach_guard: "Message held until the identity is confirmed",
  escalation_decision: "Sent to the specialist team",
};

export function entryLabel(type: string): string {
  return ENTRY_LABEL[type] ?? plain(type);
}

/** `9f8e4e4a…ae4ec`: enough to compare by eye, the full value stays one click away. */
export function shortHash(h: string): string {
  return h.length > 16 ? `${h.slice(0, 8)}…${h.slice(-6)}` : h;
}

export const BASIS_WORDS: Record<string, string> = {
  eid: "Emirates ID",
  p_cal: "name and details",
  wasl: "name and details",
  human: "a person's confirmation",
  below_tau: "name and details, below the auto-close line",
};

export function basisWords(basis: string): string {
  return BASIS_WORDS[basis] ?? plain(basis).toLowerCase();
}

export function roleWords(role: string): string {
  return plain(role);
}

const num = (v: unknown): number | undefined => (typeof v === "number" ? v : undefined);
const str = (v: unknown): string | undefined => (typeof v === "string" && v ? v : undefined);

/** Why the safety gate held, in a sentence. */
export function holdWords(h: Pick<Hold, "state" | "reason">): string {
  if (h.state === "inadequate_evidence") return "The report does not show the exam was complete, so the case stays open.";
  if (h.reason === "awaiting_eye_clinic_exam") return "Waiting for the eye-clinic exam report before the case can close.";
  if (h.reason) return `${plain(h.reason)}.`;
  return `${plain(h.state)}.`;
}

/** One line for a chain entry's body. */
export function bodySummary(e: Chain): string {
  const b = e.body;
  switch (e.type) {
    case "link_decision": {
      const edge = b.weakest_edge as { basis?: string } | undefined;
      const p = num(b.path_p_cal);
      const tau = num(b.tau);
      return `Report ${shortId(str(b.downstream_event) ?? "")} joined to the referral by ${basisWords(edge?.basis ?? "")}${p !== undefined ? `, match probability ${p.toFixed(3)}` : ""}${tau !== undefined ? `, auto-close line ${tau}` : ""}.`;
    }
    case "evidence": {
      const label = str(b.label);
      const base = b.agree ? `Both readers agree${label ? `: ${plain(label).toLowerCase()}` : ""}.` : "The two readers disagree.";
      const override = str(b.human_override);
      return override ? `${base} A person signed it as ${override}.` : base;
    }
    case "hold_decision":
      return holdWords({ state: str(b.state) ?? "held", reason: str(b.reason) });
    case "closure_decision": {
      const day = num(b.closed_day);
      const sla = num(b.sla_days);
      const inside = b.within_sla === true ? "inside" : b.within_sla === false ? "past" : null;
      return `Closed${day !== undefined ? ` on ${formatDay(day)}` : ""}${inside && sla ? `, ${inside} the ${sla}-day standard` : ""}.`;
    }
    case "recommendation": {
      const action = str(b.action);
      const days = num(b.deadline_days);
      return `${action ? plain(action) : "Recommendation"}${days ? ` within ${days} days` : ""}${str(b.urgency) ? `, ${str(b.urgency)}` : ""}.`;
    }
    case "outreach_guard":
      return `Rule: ${str(b.rule) ?? "second identifier or human confirmation"}.`;
    default: {
      const bits = Object.entries(b)
        .filter(([, v]) => typeof v === "string" || typeof v === "number" || typeof v === "boolean")
        .slice(0, 4)
        .map(([k, v]) => `${plain(k)}: ${String(v)}`);
      return bits.join(" · ") || "No summary for this entry type.";
    }
  }
}

/** The closure reason (closed) or the hold reason (open), as the summary at the top of the case. */
export function closureReason(d: CaseDetail): { title: string; body: string } {
  const c = d.case;
  if (c.closed) {
    const closure = d.current.chain.find((e) => e.type === "closure_decision")?.body ?? {};
    const sla = num(closure.sla_days);
    const inside = closure.within_sla === true ? "inside" : closure.within_sla === false ? "past" : null;
    const link = d.links[0];
    const parts = [
      d.evidence.some((e) => e.agree || e.human_override) ? "the readers' evidence was accepted" : "evidence was recorded",
      link ? `the link rests on ${basisWords(link.weakest_edge.basis)}` : null,
      inside && sla ? `${inside} the ${sla}-day standard` : null,
    ].filter(Boolean);
    return {
      title: "Closed with evidence",
      body: `${parts.join(", ")}.${str(closure.rule) ? ` Rule: ${str(closure.rule)}.` : ""}`.replace(/^./, (m) => m.toUpperCase()),
    };
  }
  const hold = d.holds[0];
  if (hold) return { title: "Held, not closed", body: `${holdWords(hold)}${hold.rule ? ` Rule: ${hold.rule}.` : ""}` };
  return { title: "Open", body: statusWords(c).next + "." };
}

export type OutreachRow = { key: string; what: string; outcome: string; sent: boolean | null; raw: Record<string, unknown> };

const SUPPRESS: Record<string, string> = {
  second_identifier_unconfirmed: "stopped: the match has no second identifier yet",
  link_below_tau_unconfirmed: "stopped: the match is below the auto-close line and nobody has confirmed it",
  consent_unknown: "stopped: no consent on file yet",
  consent_denied: "stopped: the patient declined messages",
};

/** Messages Wasl held back or sent, from the case's outreach and every decision's outreach. */
export function outreachRows(d: CaseDetail): OutreachRow[] {
  const all = [...d.outreach, ...d.decisions.flatMap((x) => (Array.isArray(x.outreach) ? (x.outreach as Record<string, unknown>[]) : []))];
  const seen = new Set<string>();
  const rows: OutreachRow[] = [];
  for (const o of all) {
    const key = JSON.stringify([o.kind, o.ev, o.suppress, o.released]);
    if (seen.has(key)) continue;
    seen.add(key);
    const kind = str(o.kind);
    const ev = str(o.ev);
    const suppress = str(o.suppress);
    const released = typeof o.released === "boolean" ? o.released : null;
    rows.push({
      key,
      what: `${kind ? plain(kind) : "Message"}${ev ? ` for ${shortId(ev)}` : ""}`,
      outcome: suppress
        ? (SUPPRESS[suppress] ?? `stopped: ${plain(suppress).toLowerCase()}`)
        : released
          ? "sent"
          : released === false
            ? "not sent"
            : "recorded",
      sent: released,
      raw: o,
    });
  }
  return rows;
}

export type ConsentRow = { key: string; when: string; what: string };

const CONSENT_NOW: Record<string, string> = {
  granted: "Allowed text messages",
  denied: "Declined text messages",
  opted_out: "Opted out of text messages",
  unknown: "No answer on messages yet",
};
export const consentNow = (c: string): string => CONSENT_NOW[c] ?? plain(c);

/** Consent events from the timeline, plus any change the decisions logged. */
export function consentRows(d: CaseDetail): ConsentRow[] {
  const rows: ConsentRow[] = d.timeline
    .filter((e) => e.type === "Consent")
    .map((e) => ({
      key: e.id,
      when: formatDay(e.arrival_day ?? e.day),
      what: e.status === "permit" ? "Patient allowed text messages" : `Consent recorded: ${plain(e.status ?? "unknown").toLowerCase()}`,
    }));
  const seen = new Set<string>();
  for (const dec of d.decisions) {
    for (const l of dec.consent_log) {
      const key = JSON.stringify(l);
      if (seen.has(key)) continue;
      seen.add(key);
      rows.push({ key, when: "during the run", what: `${l.kind ? plain(l.kind) : "Consent"} to ${plain(l.to).toLowerCase()}` });
    }
  }
  return rows;
}

const EVIDENCE_LABEL: Record<Programme, string> = {
  crc: "Bowel prep",
  cervical: "Cervical screening evidence",
  lung_nodule: "Lung nodule evidence",
  diabetic_retinopathy: "Eye screening evidence",
  breast: "Breast screening evidence",
  lung_ldct: "Lung-RADS",
  ifhas: "IFHAS results",
};

const LDCT_WORKUP_WORDS: Record<string, string> = {
  wk_treatment: "LDCT work-up: treatment referral",
  wk_benign: "LDCT work-up: benign; return to annual screening",
  wk_surveillance: "LDCT work-up: surveillance CT plan",
  wk_incomplete: "LDCT work-up: incomplete",
  wk_dna: "LDCT work-up: did not attend",
};

function evidenceDecisionWords(decision: string, programme: Programme): string {
  if (programme === "lung_ldct") return LDCT_WORKUP_WORDS[decision] ?? `Lung-RADS ${decision || "no answer"}`;
  const answer = decision === "inadequate" ? "not adequate" : plain(decision || "no answer").toLowerCase();
  return `${EVIDENCE_LABEL[programme]} ${answer}`;
}

/** A sign-off's decision, as the reviewer would say it. */
export function signoffWords(s: CaseDetail["signoffs"][number], programme: Programme): string {
  if (s.kinds.includes("link")) return s.link === "same" ? "Same patient" : s.link === "different" ? "Not the same patient" : plain(s.link || "no answer");
  if (s.kinds.includes("evidence")) return evidenceDecisionWords(s.evidence, programme);
  return plain(s.link || s.evidence || "signed");
}

/** Review actions appended through the API: the decision as a sentence. */
export function reviewWords(decision: string, programme: Programme): string {
  if (decision === "same") return "Same patient";
  if (decision === "different") return "Not the same patient";
  if (decision === "adequate" || decision === "inadequate" || programme !== "crc") {
    return evidenceDecisionWords(decision, programme);
  }
  return plain(decision);
}

const EVENT_WORDS: Record<string, string> = {
  ServiceRequest: "Referral",
  Consent: "Consent",
  Appointment: "Appointment",
  DiagnosticReport: "Report",
  Encounter: "Clinic visit",
};

export function eventWords(e: CaseDetail["timeline"][number], caseId: string): string {
  if (e.type === "ServiceRequest") return e.id === caseId ? "Referral sent" : "Referral received by the unit";
  const base = EVENT_WORDS[e.type] ?? e.type;
  return e.status && e.type !== "DiagnosticReport"
    ? `${base}, ${plain(e.status).toLowerCase()}`
    : e.category
      ? `${base}, ${plain(e.category).toLowerCase()}`
      : base;
}

/** The start of a run-time stamp in ns since the epoch, as a UTC date and time. */
export function stampWords(ns: number | undefined): string {
  if (!ns) return "time not recorded";
  const d = new Date(ns / 1e6);
  return `${d.toISOString().slice(0, 10)} ${d.toISOString().slice(11, 16)} UTC`;
}

export function atWords(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : `${d.toISOString().slice(0, 10)} ${d.toISOString().slice(11, 16)} UTC`;
}
