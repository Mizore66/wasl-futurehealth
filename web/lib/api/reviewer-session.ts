import "server-only";
import { cookies } from "next/headers";
import { notFound } from "next/navigation";
import { revalidatePath } from "next/cache";
import { apiMode, getCase, getCaseWithToken } from "@/lib/api/source";
import {
  isReviewToken,
  REVIEW_COOKIE,
  REVIEW_COOKIE_MAX_AGE,
  REVIEW_COOKIE_PATH,
  type ReviewCase,
  type SignInResult,
} from "@/components/review/auth";

/** The reviewer's token from the HttpOnly cookie, or undefined. Server-only: it never reaches props, URLs or storage. */
export async function reviewerToken(): Promise<string | undefined> {
  const value = (await cookies()).get(REVIEW_COOKIE)?.value;
  return isReviewToken(value) ? value : undefined;
}

const REFUSED: SignInResult = { ok: false, error: "Sign-in failed. Check the token and try again." };

/** Verifies the token by reading this case with it, then stores it in an HttpOnly Secure SameSite=Strict review-path cookie. */
export async function signInReviewer(caseId: string, token: string): Promise<SignInResult> {
  if (apiMode() !== "live" || !isReviewToken(token)) return REFUSED;
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
  (await cookies()).delete({ name: REVIEW_COOKIE, path: REVIEW_COOKIE_PATH });
  revalidatePath(`/app/review/${encodeURIComponent(caseId)}`);
}

/**
 * The case as the signed-in reviewer may see it. Live mode without a valid credential returns a sign-in-required
 * state and never falls back to a shared role token. Fixtures mode needs no credential.
 */
export async function getReviewCase(caseId: string): Promise<ReviewCase> {
  if (apiMode() !== "live") {
    const detail = await getCase(caseId);
    if (!detail) notFound();
    return { detail, authenticated: true };
  }
  const token = await reviewerToken();
  if (!token) return { detail: null, authenticated: false };
  const res = await getCaseWithToken(caseId, token);
  if (res.ok) return { detail: res.detail, authenticated: true };
  if (res.status === 404) notFound();
  return { detail: null, authenticated: false };
}
