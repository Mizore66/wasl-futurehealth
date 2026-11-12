import "server-only";
import { cookies } from "next/headers";
import { notFound } from "next/navigation";
import { revalidatePath } from "next/cache";
import { apiMode, getCase, getCaseWithToken } from "@/lib/api/source";
import {
  isReviewToken,
  OIDC_REVIEW_COOKIE,
  REVIEW_COOKIE,
  REVIEW_COOKIE_MAX_AGE,
  REVIEW_COOKIE_PATH,
  type ReviewCase,
  type SignInResult,
} from "@/components/review/auth";
import { mappedCredentialToken, oidcConfig, reviewAuthMode } from "@/lib/auth/oidc-config";
import { defaultSessionStore } from "@/lib/auth/oidc-store";

export type ResolvedReviewer =
  | { ok: true; token: string; method: "demo" | "oidc"; expiresAt?: string }
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
    const resolved = defaultSessionStore(config.sessionDb).resolveSession(jar.get(OIDC_REVIEW_COOKIE)?.value);
    if (!resolved.ok) return resolved;
    const { session } = resolved;
    if (session.issuer !== config.issuer) return { ok: false, reason: "unmapped" };
    const token = mappedCredentialToken(config, session.subject, session.credential);
    return token
      ? { ok: true, token, method: "oidc", expiresAt: new Date(session.expiresAt).toISOString() }
      : { ok: false, reason: "unmapped" };
  } catch {
    return { ok: false, reason: "anonymous" };
  }
}

/** Narrow compatibility wrapper for review and governance writes. */
export async function reviewerToken(): Promise<string | undefined> {
  const resolved = await resolveReviewer();
  return resolved.ok ? resolved.token : undefined;
}

const REFUSED: SignInResult = { ok: false, error: "Sign-in failed. Check the token and try again." };

/** Demo token entry only. Explicit OIDC mode cannot be downgraded by this server action. */
export async function signInReviewer(caseId: string, token: string): Promise<SignInResult> {
  if (apiMode() !== "live" || reviewAuthMode() !== "demo" || !isReviewToken(token)) return REFUSED;
  let verified;
  try {
    verified = await getCaseWithToken(caseId, token);
  } catch {
    return REFUSED;
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
      defaultSessionStore(oidcConfig().sessionDb).revokeSession(jar.get(OIDC_REVIEW_COOKIE)?.value);
    } catch {
      /* Cookie deletion still prevents this browser from using the session; no global IdP logout is claimed. */
    }
  }
  jar.delete({ name: REVIEW_COOKIE, path: REVIEW_COOKIE_PATH });
  jar.set(OIDC_REVIEW_COOKIE, "", { httpOnly: true, secure: true, sameSite: "strict", path: REVIEW_COOKIE_PATH, maxAge: 0 });
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
