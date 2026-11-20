"use client";

import { useId, useRef, useState, useTransition, type FormEvent } from "react";
import Link from "next/link";
import { appendGovernanceAction, signInForGovernance, signInPersonaForGovernance, signOutForGovernance } from "@/lib/api/governance-actions";
import type { ReviewAuthMode } from "@/lib/auth/oidc-config";
import { oidcStartHref } from "@/components/review/auth";
import { DEMO_PERSONA_WORDS, DemoPersonaPicker } from "@/components/review/demo-persona-picker";
import { clearSignedOutNotice, SESSION_REJECTED, setSignedOutNotice, signedOutNotice } from "@/components/review/signed-out-notice";

export type GovernanceOperation = "undo" | "contest" | "resolve";

const OPERATION_WORDS = {
  undo: "Undo this review",
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
      setResult({ ok: response.ok, message: response.ok ? response.hash ? `${response.message} Event ${response.hash.slice(0, 12)}.` : response.message : response.error });
      if (response.ok) form.current?.reset();
    });
  }

  return (
    <details className="mt-3 rounded-control bg-ground px-3 py-2">
      <summary className="min-h-8 cursor-pointer font-medium">{OPERATION_WORDS[operation]}</summary>
      <form ref={form} onSubmit={submit} className="mt-2 flex flex-col gap-3 border-t border-rule pt-3">
        <p className="m-0 text-small text-muted">
          {operation === "resolve"
            ? "You act under your own sign-in. You must be a different person from the original reviewer and from whoever contested it."
            : "You act under your own sign-in. The original review stays on record. This adds a new entry and does not change the original decision."}
        </p>
        {operation === "resolve" && (
          <label className="flex flex-col gap-1 text-small font-medium">
            Resolution
            <select name="resolution" required defaultValue="" disabled={pending} className="min-h-11 rounded-control border border-control bg-surface px-3 font-normal">
              <option value="">Choose a resolution</option>
              <option value="upheld">Uphold contest — undo the original review</option>
              <option value="rejected">Reject contest — keep the original review</option>
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
          {pending ? "Saving…" : OPERATION_WORDS[operation]}
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

const INDEPENDENCE = {
  demo: "Independent review needs each person to sign in separately with their own token. One person using several tokens does not count.",
  persona: "In this open demo anyone can act as anyone, so a different demo name does not prove a different person.",
  oidc: "Independent review needs each person to sign in separately with their own account. Sign-in alone does not prove they are different people.",
} as const;

/**
 * Shown instead of the controls when nobody is signed in. The same assigned-token sign-in as the review desk: the token is
 * sent once to a server action, kept in an HttpOnly cookie and never returned. No role or credential is offered.
 */
export function GovernanceSignIn({
  caseId,
  mode = "demo",
  purpose = "undo, contest or resolve",
  acts = "Undo, contest and resolve act",
}: {
  caseId: string;
  mode?: ReviewAuthMode;
  /** Completes the heading "Sign in to …". */
  purpose?: string;
  /** What acts as the signed-in credential, e.g. "Approval acts". */
  acts?: string;
}) {
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
        Sign in to {purpose}
      </h3>
      <p className="m-0 mt-1 max-w-[72ch] text-small text-muted">
        {mode === "persona" ? (
          <>
            {DEMO_PERSONA_WORDS} {acts} only as the person you pick. {INDEPENDENCE[mode]}
          </>
        ) : (
          <>
            {mode === "oidc" ? "Sign in with your organisation account." : "Enter the token assigned to you."} {acts} under your own sign-in only.{" "}
            {INDEPENDENCE[mode]}
          </>
        )}
      </p>
      {notice && (
        <p role="status" className="m-0 mt-2 max-w-[72ch] rounded-control bg-pill-negative-bg px-3 py-2 text-small text-pill-negative-ink" data-testid="signed-out-notice">
          {notice}
        </p>
      )}
      {mode === "oidc" ? (
        <Link href={oidcStartHref(caseId)} className="mt-3 inline-flex min-h-11 items-center rounded-control bg-cta px-4 font-medium text-white no-underline hover:bg-ink hover:text-white">
          Sign in with OIDC
        </Link>
      ) : mode === "persona" ? (
        <DemoPersonaPicker caseId={caseId} signIn={signInPersonaForGovernance} />
      ) : (
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
      )}
    </section>
  );
}

/**
 * Who acts, said plainly, with a way to end the session so a different actor can sign in. `signOut={false}` when the
 * host page already offers the same sign-out (the review desk), so the page has one "Sign out" control, not two.
 */
export function GovernanceIdentity({
  caseId,
  signOut = true,
  mode = "demo",
  expiresAt,
  acts = "Undo, contest and resolve act",
}: {
  caseId: string;
  signOut?: boolean;
  mode?: ReviewAuthMode;
  expiresAt?: string;
  /** What acts as this credential, e.g. "Approval acts". */
  acts?: string;
}) {
  const [pending, start] = useTransition();
  return (
    <div className="mt-3 flex flex-wrap items-start justify-between gap-3 rounded-control bg-ground px-3 py-3" data-testid="governance-identity">
      <p className="m-0 max-w-[64ch] text-small text-muted">
        {mode === "oidc"
          ? `${acts} under your organisation sign-in. Your role comes from your account.`
          : mode === "persona"
            ? `${acts} as the demo name you picked, recorded as a demo sign-off. Your role comes from that choice.`
            : `${acts} under the token you signed in with. Your role comes from that token.`}
        {expiresAt && <> Session expires at <time dateTime={expiresAt}>{expiresAt}</time>.</>}
        {!signOut && " To act as someone else, use Sign out on the review desk."} {INDEPENDENCE[mode]}
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
