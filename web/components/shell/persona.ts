import type { SessionPersona } from "@/lib/auth/session-persona";

/** The cast member whose screen it is (design-brief-v2, canonical facts): a narration persona, not a signed-in user. */
export const USERS: {
  prefix: string;
  initials: string;
  name: string;
  role: string;
}[] = [
  { prefix: "/app/today", initials: "HA", name: "Hessa Al Ali", role: "Nurse coordinator" },
  { prefix: "/app/recovery", initials: "HA", name: "Hessa Al Ali", role: "Nurse coordinator" },
  { prefix: "/app/patients", initials: "HA", name: "Hessa Al Ali", role: "Nurse coordinator" },
  { prefix: "/app/review", initials: "OK", name: "Dr Omar Al Kaabi", role: "Medical director" },
  { prefix: "/app/referrals", initials: "LH", name: "Dr Layla Haddad", role: "Unit lead, SKMC endoscopy" },
  { prefix: "/app", initials: "ST", name: "Dr Salama Al Tamimi", role: "Programme director" },
];

export type ShellPersona = {
  initials: string;
  name: string;
  role: string;
  /** "signed-in": the review session's own actor. "viewing-as": the screen's narration persona, no session behind it. */
  source: "signed-in" | "viewing-as";
};

/** Pages whose actions use the review session's credential: the review desk, the audit trail and the disposition workflow. */
export function usesReviewSession(pathname: string): boolean {
  return pathname === "/app/review" || pathname.startsWith("/app/review/") || /^\/app\/patients\/[^/]+\/(audit|disposition)(\/|$)/.test(pathname);
}

function initialsOf(name: string): string {
  const words = name.split(/[\s._-]+/).filter((w) => /[a-z0-9]/i.test(w));
  return (words.length > 1 ? words[0][0] + words[1][0] : (words[0] ?? "?").slice(0, 2)).toUpperCase();
}

/**
 * Who the shell shows. A signed-in review session wins on the pages that act with it, so a reviewer never sees a
 * fixed medical-director persona while acting as someone else; every other screen labels its persona "Viewing as".
 */
export function shellPersona(pathname: string, session: SessionPersona | null): ShellPersona {
  if (session && usesReviewSession(pathname)) {
    return { initials: initialsOf(session.actor), name: session.actor, role: session.role, source: "signed-in" };
  }
  const user = USERS.find((u) => pathname.startsWith(u.prefix)) ?? USERS.at(-1)!;
  return { initials: user.initials, name: user.name, role: user.role, source: "viewing-as" };
}

export const PERSONA_PREFIX: Record<ShellPersona["source"], string> = { "signed-in": "Signed in as", "viewing-as": "Viewing as" };
