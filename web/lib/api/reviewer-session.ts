import "server-only";
import { cookies } from "next/headers";
import { notFound } from "next/navigation";
import { revalidatePath } from "next/cache";
import { apiMode, getCase, getCaseWithToken } from "@/lib/api/source";
import {
  isReviewToken,
  OIDC_REVIEW_COOKIE,
  OIDC_SESSION_SAME_SITE,
  REVIEW_COOKIE,
  REVIEW_COOKIE_MAX_AGE,
  REVIEW_COOKIE_PATH,
  type ReviewCase,
  type SignInResult,
} from "@/components/review/auth";
import { mappedCredentialToken, oidcConfig, reviewAuthMode } from "@/lib/auth/oidc-config";
import { defaultSessionStore } from "@/lib/auth/oidc-store";
import { logOidcFailure } from "@/lib/auth/oidc";
import { personaFromOidc, personaFromToken, type SessionPersona } from "@/lib/auth/session-persona";

export type ResolvedReviewer =
  | { ok: true; token: string; method: "demo" | "oidc"; expiresAt?: string; subject?: string; credential?: string }
  | { ok: false; reason: "anonymous" | "expired" | "revoked" | "unmapped" };

/** Resolve the current request without ever falling back to a shared API credential. */
export async function resolveReviewer(): Promise<ResolvedReviewer> {
  const jar = await cookies();
  if (reviewAuthMode() === "demo") {
    const value = jar.get(REVIEW_COOKIE)?.value;
    return isReviewToken(value) ? { ok: true, token: value, method: "demo" } : { ok: false, reason: "anonymous" };
  }
  try {
    const config = oidcConfig();
    const resolved = await (await defaultSessionStore(config.sessionStore)).resolveSession(jar.get(OIDC_REVIEW_COOKIE)?.value);
    if (!resolved.ok) return resolved;
    const { session } = resolved;
    if (session.issuer !== config.issuer) return { ok: false, reason: "unmapped" };
    const token = mappedCredentialToken(config, session.subject, session.credential);
    return token
      ? { ok: true, token, method: "oidc", expiresAt: new Date(session.expiresAt).toISOString(), subject: session.subject, credential: session.credential }
      : { ok: false, reason: "unmapped" };
  } catch (error) {
    /* An unreadable configuration or an unreachable session store fails closed; the operator sees why. */
    logOidcFailure("session", "session_resolve", error);
    return { ok: false, reason: "anonymous" };
  }
}

/**
 * The signed-in review session's actor and role for the app shell, or null without one (fixtures never sign in).
 * Never throws: the shell falls back to its "Viewing as" persona.
 */
export async function reviewerPersona(): Promise<SessionPersona | null> {
  if (apiMode() !== "live") return null;
  try {
    const resolved = await resolveReviewer();
    if (!resolved.ok) return null;
    return resolved.method === "oidc" && resolved.subject && resolved.credential
      ? personaFromOidc(resolved.subject, resolved.credential)
      : personaFromToken(resolved.token);
  } catch {
    return null;
  }
}

/** Narrow compatibility wrapper for review and governance writes. */
export async function reviewerToken(): Promise<string | undefined> {
  const resolved = await resolveReviewer();
  return resolved.ok ? resolved.token : undefined;
}

const REFUSED: SignInResult = { ok: false, error: "Sign-in failed. Check the token and try again." };
/** A network error, timeout or server error says nothing about the token, so it is not reported as a bad token. */
export const UNREACHABLE: SignInResult = {
  ok: false,
  error: "Sign-in could not be checked. The review server could not be reached or did not answer in time. The token was not rejected. Try again in a moment.",
};

/** Demo token entry only. Explicit OIDC mode cannot be downgraded by this server action. */
export async function signInReviewer(caseId: string, token: string): Promise<SignInResult> {
  if (apiMode() !== "live" || reviewAuthMode() !== "demo" || !isReviewToken(token)) return REFUSED;
  let verified;
  try {
    verified = await getCaseWithToken(caseId, token);
  } catch {
    return UNREACHABLE;
  }
  if (!verified.ok) return REFUSED;
  (await cookies()).set(REVIEW_COOKIE, token, {
    httpOnly: true,
    secure: true,
    sameSite: "strict",
    path: REVIEW_COOKIE_PATH,
    maxAge: REVIEW_COOKIE_MAX_AGE,
  });
  revalidatePath(`/app/review/${encodeURIComponent(caseId)}`);
  return { ok: true };
}

export async function signOutReviewer(caseId: string): Promise<void> {
  const jar = await cookies();
  if (reviewAuthMode() === "oidc") {
    try {
      await (await defaultSessionStore(oidcConfig().sessionStore)).revokeSession(jar.get(OIDC_REVIEW_COOKIE)?.value);
    } catch (error) {
      logOidcFailure("session", "session_store", error);
      /* Cookie deletion still prevents this browser from using the session; no global IdP logout is claimed. */
    }
  }
  jar.delete({ name: REVIEW_COOKIE, path: REVIEW_COOKIE_PATH });
  jar.set(OIDC_REVIEW_COOKIE, "", { httpOnly: true, secure: true, sameSite: OIDC_SESSION_SAME_SITE, path: REVIEW_COOKIE_PATH, maxAge: 0 });
  revalidatePath(`/app/review/${encodeURIComponent(caseId)}`);
  revalidatePath(`/app/patients/${encodeURIComponent(caseId)}/audit`);
  revalidatePath("/app/review");
}

/** Live reads require a current reviewer session. Fixtures remain unauthenticated. */
export async function getReviewCase(caseId: string): Promise<ReviewCase> {
  if (apiMode() !== "live") {
    const detail = await getCase(caseId);
    if (!detail) notFound();
    return { detail, authenticated: true };
  }
  const reviewer = await resolveReviewer();
  if (!reviewer.ok) return { detail: null, authenticated: false, ...(reviewer.reason === "anonymous" ? {} : { reason: reviewer.reason }) };
  const res = await getCaseWithToken(caseId, reviewer.token);
  if (res.ok) {
    return {
      detail: res.detail,
      authenticated: true,
      session: { method: reviewer.method, ...(reviewer.expiresAt ? { expiresAt: reviewer.expiresAt } : {}) },
    };
  }
  if (res.status === 404) notFound();
  return { detail: null, authenticated: false };
}
