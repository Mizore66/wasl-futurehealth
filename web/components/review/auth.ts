import type { CaseDetail } from "@/lib/api/schema";
import type { ReviewAuthMode } from "@/lib/auth/oidc-config";

/** Client-safe constants and types. Neither bearer tokens nor OIDC provider tokens appear in these types. */
export const REVIEW_COOKIE = "wasl_review_token";
export const OIDC_REVIEW_COOKIE = "__Secure-wasl_review_session";
export const OIDC_CORRELATION_COOKIE = "__Secure-wasl_oidc_correlation";
export const OIDC_CORRELATION_COOKIE_PATH = "/app/auth/oidc/callback";
export const REVIEW_COOKIE_PATH = "/app";
export const REVIEW_COOKIE_MAX_AGE = 8 * 60 * 60;
export const REVIEW_TOKEN_PATTERN = /^\S{16,4096}$/;

export const isReviewToken = (token: unknown): token is string => typeof token === "string" && REVIEW_TOKEN_PATTERN.test(token);

export type ReviewSessionInfo = { method: ReviewAuthMode; expiresAt?: string };
export type ReviewCase =
  | { detail: CaseDetail; authenticated: true; session?: ReviewSessionInfo }
  | { detail: null; authenticated: false; reason?: "anonymous" | "expired" | "revoked" | "unmapped" };

export type SignInResult = { ok: true } | { ok: false; error: string };

export function oidcStartHref(caseId: string): string {
  return `/app/auth/oidc/start?returnTo=${encodeURIComponent(`/app/review/${encodeURIComponent(caseId)}`)}`;
}
