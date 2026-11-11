// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { ConsentActions } from "@/components/case/consent-actions";

const actions = vi.hoisted(() => ({ recordConsent: vi.fn() }));
vi.mock("@/app/app/patients/[id]/actions", () => ({ recordConsent: actions.recordConsent }));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const detail = () => JSON.parse(readFileSync(path.join(__dirname, "fixtures", "ledger_case_detail.json"), "utf8"));
let container: HTMLDivElement | undefined;
let root: Root | undefined;

async function mount(expectedHead: string, contact = detail().api_contact) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => {
    root!.render(
      createElement(ConsentActions, {
        caseId: "case/1",
        expectedHead,
        contact,
        live: true,
        canWrite: true,
      }),
    );
  });
}

async function unmount() {
  if (root) await act(async () => root!.unmount());
  container?.remove();
  root = undefined;
  container = undefined;
}

function confirmButton(label: string) {
  const button = [...container!.querySelectorAll("button")].find((candidate) => candidate.textContent === label);
  expect(button).toBeDefined();
  return button!;
}

const confirmGrant = () => confirmButton("Confirm consent granted");

function offered() {
  return [...container!.querySelectorAll("summary")].map((summary) => summary.textContent);
}

function optedOutContact() {
  const contact = detail().api_contact;
  const optOut = { ...contact.history[0], decision: "consent_opted_out", hash: "opt-out-hash", prev: contact.history[0].hash };
  contact.history = [...contact.history, optOut];
  contact.overlay = {
    ...contact.overlay,
    status: "opted_out",
    demo_pending_consent: "cancelled",
    contact_blocked: true,
    events: 2,
    last_event_hash: optOut.hash,
  };
  return contact;
}

afterEach(async () => {
  await unmount();
  vi.clearAllMocks();
});

it("blocks a stale-head retry until refreshed case state is mounted", async () => {
  actions.recordConsent.mockImplementation(async (_caseId: string, head: string) =>
    head === "stale-head"
      ? { ok: false, status: 409, error: "stale expected_head", currentHead: "new-head" }
      : { ok: true, status: 201, expectedHead: "new-head", contact: detail().api_contact },
  );
  await mount("stale-head");

  const retry = confirmGrant();
  await act(async () => {
    retry.click();
  });
  expect(container!.textContent).toContain("Reload and review the latest case before retrying");
  expect(actions.recordConsent).toHaveBeenCalledTimes(1);

  const controls = [...container!.querySelectorAll("button, textarea")];
  expect(controls.every((control) => (control as HTMLButtonElement | HTMLTextAreaElement).disabled)).toBe(true);
  await act(async () => {
    retry.click();
  });
  expect(actions.recordConsent).toHaveBeenCalledTimes(1);

  await unmount();
  await mount("new-head");
  await act(async () => {
    confirmGrant().click();
  });
  expect(actions.recordConsent).toHaveBeenCalledTimes(2);
  expect(actions.recordConsent).toHaveBeenNthCalledWith(2, "case/1", "new-head", "consent_granted", "");
});

it("offers only a repeat opt-out once the patient has opted out", async () => {
  await mount("current-head", optedOutContact());
  expect(offered()).toEqual(["Record opt-out"]);
  expect(container!.textContent).toContain("Consent can no longer be granted or denied here");
  expect(container!.textContent).not.toContain("Confirm consent granted");
  expect(container!.textContent).not.toContain("Confirm consent denied");

  actions.recordConsent.mockResolvedValue({ ok: true, status: 201, expectedHead: "current-head", contact: optedOutContact() });
  await act(async () => {
    confirmButton("Confirm opt-out").click();
  });
  expect(actions.recordConsent).toHaveBeenCalledWith("case/1", "current-head", "consent_opted_out", "");
});

it("reports the opt-out refusal accurately and keeps the repeat opt-out available", async () => {
  actions.recordConsent.mockResolvedValue({
    ok: false,
    status: 409,
    error: "patient has opted out; a later grant needs a new, separately recorded consent flow",
  });
  await mount("current-head");
  expect(offered()).toEqual(["Record consent granted", "Record consent denied", "Record opt-out"]);

  await act(async () => {
    confirmButton("Confirm consent denied").click();
  });
  const text = container!.textContent;
  expect(text).toContain("the patient has opted out of contact in this demo flow, so consent cannot be granted or denied");
  expect(text).not.toContain("Reload and review the latest case");
  expect(text).not.toContain("head signature is missing");
  expect(text).not.toContain("Consent actions are paused");
  expect(offered()).toEqual(["Record opt-out"]);
  expect(confirmButton("Confirm opt-out").disabled).toBe(false);

  await act(async () => {
    confirmButton("Confirm opt-out").click();
  });
  expect(actions.recordConsent).toHaveBeenLastCalledWith("case/1", "current-head", "consent_opted_out", "");
});
