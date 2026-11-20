import { ZodError } from "zod";
import { ApiStatusError, MissingCredentialError } from "@/lib/api/source";

/**
 * Why a case read on Today failed, in words Today shows as given, and the status its route answers with: no credential
 * configured here (500), an answer that does not parse, an API refusal or error status, or no answer (502).
 */
export function readFailure(e: unknown): { status: number; error: string } {
  if (e instanceof MissingCredentialError) return { status: 500, error: "this site is not set up to read cases. Contact support" };
  // A 2xx body that is not JSON (res.json() throws SyntaxError) is an answer too, just not one this page can read.
  if (e instanceof ZodError || e instanceof SyntaxError) return { status: 502, error: "the case could not be read. Contact support" };
  if (e instanceof ApiStatusError) {
    if (e.status === 401 || e.status === 403) return { status: 502, error: "this site is not allowed to read cases. Contact support" };
    return { status: 502, error: "the case system returned an error. Try again" };
  }
  return { status: 502, error: "the case system did not respond. Try again" };
}
