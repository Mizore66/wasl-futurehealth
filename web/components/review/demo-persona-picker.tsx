"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { DEMO_PERSONAS, type SignInResult } from "./auth";

/** What the open demo is, said once wherever a visitor picks a persona. */
export const DEMO_PERSONA_WORDS =
  "This is an open demo: pick a demo persona to act as. Each sign-off is recorded against the persona you pick, as a demo sign-off in the shared synthetic record that other visitors also see. The two-person rule still holds: a second read must come from a different persona, so sign out and pick another to give it. In production, staff sign in through their provider’s SSO and every sign-off is bound to a named clinician.";

/**
 * The open demo's sign-in (WASL_REVIEW_AUTH_MODE=persona). The browser sends only a persona name; the web server signs
 * with that persona's own server-side token, and the API still decides its role and actor. `signIn` is the host's
 * server action, so this module imports no server code.
 */
export function DemoPersonaPicker({ caseId, signIn }: { caseId: string; signIn: (caseId: string, credential: string) => Promise<SignInResult> }) {
  const router = useRouter();
  const [error, setError] = useState<string>();
  const [pending, start] = useTransition();

  function pick(credential: string) {
    if (pending) return;
    start(async () => {
      const res = await signIn(caseId, credential);
      if (!res.ok) {
        setError(res.error);
        return;
      }
      setError(undefined);
      router.refresh();
    });
  }

  return (
    <div className="mt-3 flex flex-col gap-3" data-testid="demo-persona-picker">
      <ul className="m-0 flex list-none flex-wrap gap-2 p-0">
        {DEMO_PERSONAS.map((persona) => (
          <li key={persona.credential}>
            <button
              type="button"
              aria-disabled={pending}
              onClick={() => pick(persona.credential)}
              className="min-h-11 rounded-control border border-control bg-surface px-3 text-left text-ink hover:bg-active aria-disabled:opacity-60"
            >
              Act as {persona.name}
            </button>
          </li>
        ))}
      </ul>
      {error && (
        <p role="alert" className="m-0 rounded-control bg-pill-negative-bg px-3 py-2 text-pill-negative-ink">
          {error}
        </p>
      )}
    </div>
  );
}
