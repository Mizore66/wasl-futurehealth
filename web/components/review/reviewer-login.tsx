"use client";

import { useId, useRef, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { signInReviewer, signOutReviewer } from "@/app/app/review/[id]/actions";
import type { ReviewAuthMode } from "@/lib/auth/oidc-config";
import { oidcStartHref, sessionEndedMessage, type ReviewSessionInfo, type SessionEndedReason } from "./auth";
import type { SessionPersona } from "@/lib/auth/session-persona";
import { clearSignedOutNotice, signedOutNotice } from "./signed-out-notice";

const INDEPENDENCE = {
  demo: "A token proves a configured demo identity, not that a separate person is present.",
  oidc: "OIDC verifies a provider subject and maps it to a configured actor. That alone does not prove that separate real-world people took part.",
} as const;

/** Explicitly selected demo token entry or OIDC redirect. OIDC renders no token input or identity picker. */
export function ReviewerLogin({
  caseId,
  mode = "demo",
  authError = false,
  returnTo,
  reason,
}: {
  caseId: string;
  mode?: ReviewAuthMode;
  authError?: boolean;
  returnTo?: string;
  /** Why an existing session no longer works (expired, revoked or remapped); shown above the card. */
  reason?: SessionEndedReason;
}) {
  const id = useId();
  const router = useRouter();
  const field = useRef<HTMLInputElement>(null);
  const [error, setError] = useState<string>();
  const [notice, setNotice] = useState(() => signedOutNotice(caseId));
  const [pending, start] = useTransition();

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const token = field.current?.value ?? "";
    clearSignedOutNotice(caseId);
    setNotice(undefined);
    start(async () => {
      const res = await signInReviewer(caseId, token);
      if (field.current) field.current.value = "";
      if (!res.ok) {
        setError(res.error);
        field.current?.focus();
        return;
      }
      setError(undefined);
      router.refresh();
    });
  }

  const ended = reason ? sessionEndedMessage(reason) : undefined;
  return (
    <>
      {ended && (
        <p
          role="status"
          className="m-0 mt-6 max-w-[56ch] rounded-control bg-pill-negative-bg px-3 py-2 text-pill-negative-ink"
          data-testid="session-ended-notice"
        >
          {ended}
        </p>
      )}
      <section aria-labelledby={`${id}-h`} className="mt-6 max-w-[56ch] rounded-card border border-line bg-surface p-4 md:p-6" data-testid="reviewer-login">
        <h2 id={`${id}-h`} className="m-0 text-section font-medium">
          Sign in to review
        </h2>
        <p className="m-0 mt-1 text-muted">
          {mode === "oidc"
            ? "Use your organisation’s OIDC sign-in. The verified provider subject selects a server-side credential. This page offers no role or credential choice. "
            : "Enter the review token assigned to you. Your token decides who you are and what you may answer. This page never offers a choice of credential. "}
          {INDEPENDENCE[mode]}
        </p>
        {(notice || authError) && (
          <p role="status" className="m-0 mt-2 rounded-control bg-pill-negative-bg px-3 py-2 text-small text-pill-negative-ink" data-testid="signed-out-notice">
            {authError ? "OIDC sign-in could not be completed. Try again or contact the operator." : notice}
          </p>
        )}
        {mode === "oidc" ? (
          /*
           * A plain anchor, not a client-router Link: the start route answers with a cross-origin redirect to the identity provider,
           * which needs a full-page navigation. The client router would fetch it as an RSC request instead.
           */
          <a
            href={oidcStartHref(caseId, returnTo)}
            className="mt-4 inline-flex min-h-11 items-center self-start rounded-control bg-cta px-4 font-medium text-white no-underline hover:bg-ink hover:text-white"
          >
            Sign in with OIDC
          </a>
        ) : (
          <form onSubmit={submit} className="mt-3 flex flex-col gap-3">
            <div>
              <label htmlFor={`${id}-token`} className="block font-medium">
                Review token
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
    </>
  );
}

/** `persona` names the OIDC subject and its mapped credential (from reviewerPersona), so a reviewer can check who they are before acting. */
export function ReviewerSessionStatus({ session, persona }: { session: ReviewSessionInfo; persona?: SessionPersona | null }) {
  const oidcPersona = session.method === "oidc" && persona?.method === "oidc" ? persona : undefined;
  return (
    <p className="m-0 mt-2 text-small text-muted" data-testid="reviewer-session">
      {session.method === "oidc" ? (
        oidcPersona ? (
          <>
            Signed in with OIDC as{" "}
            <span className="font-medium text-ink" data-testid="reviewer-session-subject">
              {oidcPersona.actor}
            </span>{" "}
            ({oidcPersona.role}).
          </>
        ) : (
          "Signed in with OIDC."
        )
      ) : (
        "Signed in with a demo token."
      )}{" "}
      {session.expiresAt ? (
        <>
          Session expires at <time dateTime={session.expiresAt}>{session.expiresAt}</time>.
        </>
      ) : (
        "The demo cookie lasts up to eight hours."
      )}
    </p>
  );
}

export function ReviewerSignOut({ caseId }: { caseId: string }) {
  const router = useRouter();
  const [pending, start] = useTransition();
  return (
    <button
      type="button"
      aria-disabled={pending}
      onClick={() =>
        !pending &&
        start(async () => {
          await signOutReviewer(caseId);
          router.refresh();
        })
      }
      className="min-h-11 rounded-control border border-control bg-surface px-3 text-ink hover:bg-active aria-disabled:opacity-60"
    >
      Sign out
    </button>
  );
}
