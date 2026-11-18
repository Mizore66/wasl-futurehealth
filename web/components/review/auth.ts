import type { CaseDetail } from "@/lib/api/schema";
import type { ReviewAuthMode } from "@/lib/auth/oidc-config";

/** Client-safe constants and types. Neither bearer tokens nor OIDC provider tokens appear in these types. */
export const REVIEW_COOKIE = "wasl_review_token";
export const OIDC_REVIEW_COOKIE = "__Secure-wasl_review_session";
/*
 * The OIDC session cookie is SameSite=Lax, not Strict. The callback is reached by a top-level navigation that starts on
 * the identity provider's site, which in production is always a different site. Browsers withhold Strict cookies from
 * every request in a redirect chain that began cross-site, so with Strict the first page after sign-in rendered as
 * signed out until a manual reload. Lax sends the cookie on top-level GET navigations only.
 *
 * Lax keeps CSRF protection here because the cookie (path /app) authorises no state change on a GET:
 * - every write is a Next server action ("use server" modules under app/app and lib/api/*-actions.ts). Those are POST
 *   only, Lax cookies are never sent on a cross-site POST, and Next also rejects a server action whose Origin does not
 *   match the Host;
 * - the GET route handlers under /app (the audit export, review photos, the Today ledger) only read, and their bytes go
 *   to the signed-in browser itself, never to the cross-site page that started the navigation;
 * - pages only read. The cookie still never reaches cross-site subresource, fetch or iframe requests.
 * The demo token cookie stays Strict: it is set by a same-site server action, so it never meets this redirect problem.
 */
export const OIDC_SESSION_SAME_SITE = "lax" as const;
export const OIDC_CORRELATION_COOKIE = "__Secure-wasl_oidc_correlation";
export const OIDC_CORRELATION_COOKIE_PATH = "/app/auth/oidc/callback";
export const REVIEW_COOKIE_PATH = "/app";
export const REVIEW_COOKIE_MAX_AGE = 8 * 60 * 60;
export const REVIEW_TOKEN_PATTERN = /^\S{16,4096}$/;

export const isReviewToken = (token: unknown): token is string => typeof token === "string" && REVIEW_TOKEN_PATTERN.test(token);

export type ReviewSessionInfo = { method: ReviewAuthMode; expiresAt?: string };
/** Why a session cookie that was presented no longer signs anyone in. */
export type SessionEndedReason = "expired" | "revoked" | "unmapped";
export type ReviewCase =
  | { detail: CaseDetail; authenticated: true; session?: ReviewSessionInfo }
  | { detail: null; authenticated: false; reason?: "anonymous" | SessionEndedReason };

const SESSION_ENDED: Record<SessionEndedReason, string> = {
  expired: "Your session has expired. Sign in again to continue.",
  revoked: "Your session was ended. Sign in again to continue.",
  unmapped: "Your access has changed since you signed in. Sign in again, or contact the operator if it keeps happening.",
};

/** A short message for the sign-in page; anything other than a known ended-session reason shows nothing. */
export function sessionEndedMessage(reason: unknown): string | undefined {
  return typeof reason === "string" && Object.hasOwn(SESSION_ENDED, reason) ? SESSION_ENDED[reason as SessionEndedReason] : undefined;
}

export type SignInResult = { ok: true } | { ok: false; error: string };

/** `returnTo` defaults to the case's review page; another /app/ page (the disposition workflow) may pass its own path. */
export function oidcStartHref(caseId: string, returnTo = `/app/review/${encodeURIComponent(caseId)}`): string {
  return `/app/auth/oidc/start?returnTo=${encodeURIComponent(returnTo)}`;
}

/** How a clinician signs in, in words that match the configured mode (OIDC deployments have no tokens to hand out). */
export function clinicianSignInWords(mode: ReviewAuthMode): string {
  return mode === "oidc" ? "Sign in with your organisation’s OIDC account." : "Sign in with the clinician token assigned to you.";
}
