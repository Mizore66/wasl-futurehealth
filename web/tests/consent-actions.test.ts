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

async function mount(expectedHead: string) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => {
    root!.render(
      createElement(ConsentActions, {
        caseId: "case/1",
        expectedHead,
        contact: detail().api_contact,
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

function confirmGrant() {
  const button = [...container!.querySelectorAll("button")].find((candidate) => candidate.textContent === "Confirm consent granted");
  expect(button).toBeDefined();
  return button!;
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
