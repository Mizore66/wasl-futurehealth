import { addBusinessDays } from "@/lib/domain/handoff";

/**
 * Clinician-to-clinician consult ("Ask a specialist"): who is asked, by when, and what Today should show. Pure, so the
 * case page, Today and the tests read the same rules. Days are API run days (see lib/domain/day).
 */

/** Working days the specialist has to reply; the same 2-day clock as an urgent referral return (lib/domain/handoff). */
export const CONSULT_REPLY_DAYS = 2;

export const SPECIALTIES = [
  {
    id: "gastroenterology",
    label: "Gastroenterology",
    owner: "Dr Layla Haddad",
  },
  { id: "pathology", label: "Pathology", owner: "Dr Omar Al Kaabi" },
  { id: "radiology", label: "Radiology", owner: "Dr Sara Nasser" },
] as const;

export type SpecialtyId = (typeof SPECIALTIES)[number]["id"];

export const QUESTION_MAX = 300;

export type Consult = {
  id: string;
  caseId: string;
  /** Short label for the case, shown on Today. */
  caseLabel: string;
  specialty: SpecialtyId;
  owner: string;
  question: string;
  askedDay: number;
  replyBy: number;
  reply?: string;
  repliedDay?: number;
  /** The asker has read the reply; it leaves Today. */
  seen?: boolean;
};

/** The day a consult asked on `now` must be answered by. */
export const consultReplyBy = (now: number) =>
  addBusinessDays(now, CONSULT_REPLY_DAYS);

export type ConsultStatus = "waiting" | "overdue" | "answered";

export const specialty = (id: SpecialtyId) =>
  SPECIALTIES.find((s) => s.id === id) ?? SPECIALTIES[0];

export const isSpecialty = (v: unknown): v is SpecialtyId =>
  SPECIALTIES.some((s) => s.id === v);

/** A new consult, or the reason it cannot be asked yet. */
export function newConsult(
  input: {
    caseId: string;
    caseLabel: string;
    specialty: string;
    question: string;
    now: number;
  },
  id: string,
): { ok: true; consult: Consult } | { ok: false; error: string } {
  const question = input.question.trim().replace(/\s+/g, " ");
  if (!isSpecialty(input.specialty))
    return { ok: false, error: "Choose a specialty." };
  if (!question) return { ok: false, error: "Write your question." };
  if (question.length > QUESTION_MAX)
    return {
      ok: false,
      error: `Keep the question under ${QUESTION_MAX} characters.`,
    };
  const s = specialty(input.specialty);
  return {
    ok: true,
    consult: {
      id,
      caseId: input.caseId,
      caseLabel: input.caseLabel,
      specialty: s.id,
      owner: s.owner,
      question,
      askedDay: input.now,
      replyBy: consultReplyBy(input.now),
    },
  };
}

export function consultStatus(c: Consult, now: number): ConsultStatus {
  if (c.reply) return "answered";
  return now > c.replyBy ? "overdue" : "waiting";
}

/** Records the specialist's reply; a second reply keeps the first. */
export function answerConsult(c: Consult, reply: string, now: number): Consult {
  const text = reply.trim();
  if (c.reply || !text) return c;
  return { ...c, reply: text.slice(0, 1000), repliedDay: now };
}

/** Today shows a consult while its reply is overdue, or answered and not yet read. Overdue first, then oldest. */
export function consultsForToday(
  all: readonly Consult[],
  now: number,
): Consult[] {
  const rank = (c: Consult) => (consultStatus(c, now) === "overdue" ? 0 : 1);
  return all
    .filter((c) => {
      const s = consultStatus(c, now);
      return s === "overdue" || (s === "answered" && !c.seen);
    })
    .sort((a, b) => rank(a) - rank(b) || a.replyBy - b.replyBy);
}

export const statusWords: Record<ConsultStatus, string> = {
  waiting: "Waiting for reply",
  overdue: "Reply overdue",
  answered: "Answered",
};
