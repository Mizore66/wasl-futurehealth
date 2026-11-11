import type { CaseDetail } from "@/lib/api/schema";

/** Client-safe constants and types for the reviewer credential. The token itself never appears in this module's types. */
export const REVIEW_COOKIE = "wasl_review_token";
export const REVIEW_COOKIE_PATH = "/app";
export const REVIEW_COOKIE_MAX_AGE = 8 * 60 * 60;
export const REVIEW_TOKEN_PATTERN = /^\S{16,4096}$/;

export const isReviewToken = (token: unknown): token is string => typeof token === "string" && REVIEW_TOKEN_PATTERN.test(token);

export type ReviewCase = { detail: CaseDetail; authenticated: true } | { detail: null; authenticated: false };

export type SignInResult = { ok: true } | { ok: false; error: string };
