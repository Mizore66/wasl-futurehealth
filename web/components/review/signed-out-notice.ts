/*
 * Why the browser was just signed out, carried across the re-render that replaces the governance controls with a sign-in
 * prompt. Module state in the browser only: it is set from a client event handler (so the server's copy stays empty),
 * never stored, gone on a full reload, and cleared by the next sign-in attempt.
 */
const notices = new Map<string, string>();

export const SESSION_REJECTED =
  "Nothing was appended: your sign-in was not accepted or has ended, so you were signed out. Sign in again to continue.";

export function setSignedOutNotice(caseId: string, message: string): void {
  notices.set(caseId, message);
}

export function signedOutNotice(caseId: string): string | undefined {
  return notices.get(caseId);
}

export function clearSignedOutNotice(caseId: string): void {
  notices.delete(caseId);
}
