// @vitest-environment happy-dom

import { act, createElement, type ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { GovernanceControls, GovernanceIdentity, GovernanceSignIn } from "@/components/case/governance-controls";
import { clearSignedOutNotice, SESSION_REJECTED } from "@/components/review/signed-out-notice";

const actions = vi.hoisted(() => ({ appendGovernanceAction: vi.fn(), signInForGovernance: vi.fn(), signOutForGovernance: vi.fn() }));
vi.mock("@/lib/api/governance-actions", () => actions);

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

async function mount(element: ReactElement) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(element));
}

async function unmount() {
  if (root) await act(async () => root!.unmount());
  container?.remove();
  root = undefined;
  container = undefined;
}

async function submitContest(reason = "Needs a second look") {
  container!.querySelector("textarea")!.value = reason;
  await act(async () => container!.querySelector("form")!.requestSubmit());
}

beforeEach(() => {
  vi.clearAllMocks();
  clearSignedOutNotice("c1");
  actions.signOutForGovernance.mockResolvedValue(undefined);
});
afterEach(unmount);

describe("a governance action whose session is missing or rejected", () => {
  it("signs out once, says why, and the sign-in prompt that replaces the controls repeats it", async () => {
    actions.appendGovernanceAction.mockResolvedValue({ ok: false, error: "The API did not accept your sign-in. Sign in again.", signIn: true });
    await mount(createElement(GovernanceControls, { caseId: "c1", target: "review-hash", operation: "contest" }));
    await submitContest();

    expect(actions.signOutForGovernance).toHaveBeenCalledTimes(1);
    expect(actions.signOutForGovernance).toHaveBeenCalledWith("c1");
    expect(container!.querySelector('[role="alert"]')?.textContent).toBe(SESSION_REJECTED);

    /* The server re-renders the page signed out: the controls give way to the sign-in prompt. */
    await unmount();
    await mount(createElement(GovernanceSignIn, { caseId: "c1" }));
    expect(container!.querySelector('[data-testid="signed-out-notice"]')?.textContent).toBe(SESSION_REJECTED);
    /* Nothing signs in, retries or signs out again by itself. */
    expect(actions.signInForGovernance).not.toHaveBeenCalled();
    expect(actions.appendGovernanceAction).toHaveBeenCalledTimes(1);
    expect(actions.signOutForGovernance).toHaveBeenCalledTimes(1);

    /* The next sign-in attempt clears the notice, here and for later prompts. */
    actions.signInForGovernance.mockResolvedValue({ ok: false, error: "Sign-in failed. Check the token and try again." });
    container!.querySelector("input")!.value = "another-token-1234567890";
    await act(async () => container!.querySelector("form")!.requestSubmit());
    expect(container!.querySelector('[data-testid="signed-out-notice"]')).toBeNull();
    await unmount();
    await mount(createElement(GovernanceSignIn, { caseId: "c1" }));
    expect(container!.querySelector('[data-testid="signed-out-notice"]')).toBeNull();
  });

  it("does not sign out on an ordinary refusal", async () => {
    actions.appendGovernanceAction.mockResolvedValue({ ok: false, error: "actor must differ" });
    await mount(createElement(GovernanceControls, { caseId: "c1", target: "review-hash", operation: "contest" }));
    await submitContest();
    expect(actions.signOutForGovernance).not.toHaveBeenCalled();
    expect(container!.querySelector('[role="alert"]')?.textContent).toBe("actor must differ");
  });

  it("a fresh sign-in prompt shows no notice", async () => {
    await mount(createElement(GovernanceSignIn, { caseId: "c1" }));
    expect(container!.querySelector('[data-testid="signed-out-notice"]')).toBeNull();
  });
});

describe("GovernanceIdentity", () => {
  it("offers its own sign-out by default and none when the host page already has one", async () => {
    await mount(createElement(GovernanceIdentity, { caseId: "c1" }));
    expect([...container!.querySelectorAll("button")].map((b) => b.textContent)).toEqual(["Sign out"]);
    await unmount();
    await mount(createElement(GovernanceIdentity, { caseId: "c1", signOut: false }));
    expect(container!.querySelectorAll("button")).toHaveLength(0);
    expect(container!.textContent).toContain("use Sign out on the review desk");
  });
});
