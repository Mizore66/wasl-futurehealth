"use client";

import { useId, useRef, useState, useTransition, type FormEvent } from "react";
import { appendGovernanceAction, signInForGovernance, signOutForGovernance } from "@/lib/api/governance-actions";
import { clearSignedOutNotice, SESSION_REJECTED, setSignedOutNotice, signedOutNotice } from "@/components/review/signed-out-notice";

export type GovernanceOperation = "undo" | "contest" | "resolve";

const OPERATION_WORDS = {
  undo: "Append compensation",
  contest: "Contest review",
  resolve: "Resolve contest",
} as const;

/**
 * Live-only controls. Fixture history never receives this component, so a preview cannot look persisted. There is no
 * credential picker: the server action acts as the signed-in token, and the API decides its role and actor.
 */
export function GovernanceControls({ caseId, target, operation }: { caseId: string; target: string; operation: GovernanceOperation }) {
  const [pending, startTransition] = useTransition();
  const [result, setResult] = useState<{ ok: boolean; message: string }>();
  const form = useRef<HTMLFormElement>(null);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    startTransition(async () => {
      const response = await appendGovernanceAction({
        caseId,
        target,
        operation,
        resolution: data.get("resolution") || undefined,
        reason: data.get("reason"),
      });
      if (!response.ok && response.signIn) {
        /*
         * No session, or the API rejected the stored one: clear the cookie and re-render, so the page shows its sign-in
         * prompt (which repeats this notice) instead of controls that cannot act. Only this submit signs out; nothing
         * signs in or retries automatically, so it cannot loop.
         */
        setResult({ ok: false, message: SESSION_REJECTED });
        setSignedOutNotice(caseId, SESSION_REJECTED);
        await signOutForGovernance(caseId);
        return;
      }
      setResult({ ok: response.ok, message: response.ok ? `${response.message} Event ${response.hash.slice(0, 12)}.` : response.error });
      if (response.ok) form.current?.reset();
    });
  }

  return (
    <details className="mt-3 rounded-control bg-ground px-3 py-2">
      <summary className="min-h-8 cursor-pointer font-medium">{OPERATION_WORDS[operation]}</summary>
      <form ref={form} onSubmit={submit} className="mt-2 flex flex-col gap-3 border-t border-rule pt-3">
        <p className="m-0 text-small text-muted">
          {operation === "resolve"
            ? "You act as your signed-in credential. It must be a different authorised actor from both the original reviewer and the contester; the API refuses a same-actor resolution."
            : "You act as your signed-in credential. Original event remains unchanged: this appends to the review ledger and does not change the canonical decision."}
        </p>
        {operation === "resolve" && (
          <label className="flex flex-col gap-1 text-small font-medium">
            Resolution
            <select name="resolution" required defaultValue="" disabled={pending} className="min-h-11 rounded-control border border-control bg-surface px-3 font-normal">
              <option value="">Choose a resolution</option>
              <option value="upheld">Uphold contest — compensate the original review</option>
              <option value="rejected">Reject contest — return the original review to active</option>
            </select>
          </label>
        )}
        <label className="flex flex-col gap-1 text-small font-medium">
          Reason
          <textarea
            name="reason"
            required
            minLength={1}
            maxLength={1000}
            rows={3}
            disabled={pending}
            className="resize-y rounded-control border border-control bg-surface px-3 py-2 font-normal"
          />
        </label>
        <button type="submit" disabled={pending} className="min-h-11 rounded-control bg-cta px-4 font-medium text-white hover:bg-ink disabled:opacity-60">
          {pending ? "Appending…" : OPERATION_WORDS[operation]}
        </button>
        {result && (
          <p role={result.ok ? "status" : "alert"} className={result.ok ? "m-0 text-small text-positive" : "m-0 text-small text-negative"}>
            {result.message}
          </p>
        )}
      </form>
    </details>
  );
}

const INDEPENDENCE =
  "Independence holds only when each actor signs in separately, in their own session, with a distinct token the operator issued to them. A token proves a configured demo identity, not that a separate person is present; one person holding several tokens defeats it.";

/**
 * Shown instead of the controls when nobody is signed in. The same assigned-token sign-in as the review desk: the token is
 * sent once to a server action, kept in an HttpOnly cookie and never returned. No role or credential is offered.
 */
export function GovernanceSignIn({ caseId }: { caseId: string }) {
  const id = useId();
  const field = useRef<HTMLInputElement>(null);
  const [error, setError] = useState<string>();
  const [notice, setNotice] = useState(() => signedOutNotice(caseId));
  const [pending, start] = useTransition();

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const token = field.current?.value ?? "";
    clearSignedOutNotice(caseId);
    setNotice(undefined);
    start(async () => {
      const res = await signInForGovernance(caseId, token);
      if (field.current) field.current.value = "";
      if (!res.ok) {
        setError(res.error);
        field.current?.focus();
        return;
      }
      setError(undefined);
    });
  }

  return (
    <section aria-labelledby={`${id}-h`} className="mt-3 rounded-control bg-ground px-3 py-3" data-testid="governance-signin">
      <h3 id={`${id}-h`} className="m-0 font-medium">
        Sign in to undo, contest or resolve
      </h3>
      <p className="m-0 mt-1 max-w-[72ch] text-small text-muted">
        Enter the token assigned to you. Undo, contest and resolve act only as that credential: it decides who you are and what you may do,
        and this page never offers a choice of credential. {INDEPENDENCE}
      </p>
      {notice && (
        <p role="status" className="m-0 mt-2 max-w-[72ch] rounded-control bg-pill-negative-bg px-3 py-2 text-small text-pill-negative-ink" data-testid="signed-out-notice">
          {notice}
        </p>
      )}
      <form onSubmit={submit} className="mt-3 flex flex-col gap-3">
        <div>
          <label htmlFor={`${id}-token`} className="block text-small font-medium">
            Your token
          </label>
          <input
            ref={field}
            id={`${id}-token`}
            name="token"
            type="password"
            autoComplete="off"
            spellCheck={false}
            required
            aria-describedby={error ? `${id}-err` : undefined}
            aria-invalid={error ? true : undefined}
            className="mt-1 min-h-11 w-full rounded-control border border-control bg-surface px-3 text-ink"
          />
        </div>
        {error && (
          <p id={`${id}-err`} role="alert" className="m-0 rounded-control bg-pill-negative-bg px-3 py-2 text-pill-negative-ink">
            {error}
          </p>
        )}
        <button
          type="submit"
          aria-disabled={pending}
          onClick={(event) => pending && event.preventDefault()}
          className="min-h-11 self-start rounded-control bg-cta px-4 font-medium text-white hover:bg-ink aria-disabled:opacity-60"
        >
          {pending ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </section>
  );
}

/**
 * Who acts, said plainly, with a way to end the session so a different actor can sign in. `signOut={false}` when the
 * host page already offers the same sign-out (the review desk), so the page has one "Sign out" control, not two.
 */
export function GovernanceIdentity({ caseId, signOut = true }: { caseId: string; signOut?: boolean }) {
  const [pending, start] = useTransition();
  return (
    <div className="mt-3 flex flex-wrap items-start justify-between gap-3 rounded-control bg-ground px-3 py-3" data-testid="governance-identity">
      <p className="m-0 max-w-[64ch] text-small text-muted">
        Undo, contest and resolve act as the token you signed in with; the API decides your role and actor from it.
        {!signOut && " To act as someone else, use Sign out on the review desk."} {INDEPENDENCE}
      </p>
      {signOut && (
        <button
          type="button"
          aria-disabled={pending}
          onClick={() => !pending && start(() => signOutForGovernance(caseId))}
          className="min-h-11 rounded-control border border-control bg-surface px-3 text-ink hover:bg-active aria-disabled:opacity-60"
        >
          Sign out
        </button>
      )}
    </div>
  );
}
