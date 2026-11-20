/**
 * Shared harness for the referral inbox's render tests (happy-dom). It stands in for the Next.js router only:
 *  - `useSearchParams` follows `location.search`, and the native `history.pushState`/`replaceState` the inbox uses for
 *    shallow moves re-render it, as Next.js does in the browser;
 *  - `router.push` (a server navigation, used for the case-id search) is recorded, not performed: tests pass the new
 *    server-filtered props themselves.
 * The inbox, its state and its URL handling are real.
 */
import { act, createElement, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { expect, vi } from "vitest";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { patchHistory, push, refresh } from "./referral-nav-mock";

export { push, refresh };

export type InboxProps = ComponentProps<typeof ReferralInbox>;

export function mountInbox() {
  patchHistory();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  // happy-dom cancels Web Animations on unmount with an unhandled AbortError; motion falls back to its JS driver without them.
  Object.defineProperty(Element.prototype, "animate", { value: undefined, configurable: true, writable: true });
  history.replaceState(null, "", "/app/referrals");
  sessionStorage.clear(); // fixtures-mode returns kept in this browser must not leak between tests
  push.mockReset();
  refresh.mockReset();
  const host = document.createElement("div");
  document.body.append(host);
  const root: Root = createRoot(host);
  let last: InboxProps | null = null;
  const h = {
    host,
    render(props: Partial<InboxProps>) {
      last = { referrals: [], window: 7, now: 120, q: "", scopeBar: null, ...last, ...props } as InboxProps;
      act(() => root.render(createElement(ReferralInbox, last!)));
    },
    unmount() {
      act(() => root.unmount());
      host.remove();
    },
    /** A shallow move, as a link in the inbox makes it. */
    go(url: string) {
      act(() => history.pushState(null, "", url));
    },
    url: () => `${location.pathname}${location.search}`,
    pane: () => host.querySelector<HTMLElement>('section[aria-label="Open referral"]'),
    /** The case id of the open referral, or null. */
    openId: () => host.querySelector<HTMLElement>("section[data-case]")?.dataset.case ?? null,
    text: () => host.textContent ?? "",
    paneText: () => h.pane()?.textContent ?? "",
    button(name: string | RegExp, scope: ParentNode = host) {
      const found = [...scope.querySelectorAll<HTMLButtonElement>("button")].filter((b) => {
        const visible = b.cloneNode(true) as HTMLElement;
        visible.querySelectorAll("[aria-hidden]").forEach((n) => n.remove());
        const label = (b.getAttribute("aria-label") ?? visible.textContent ?? "").trim();
        return typeof name === "string" ? label === name : name.test(label);
      });
      expect(found, `button ${String(name)}`).toHaveLength(1);
      return found[0];
    },
    click(name: string | RegExp, scope: ParentNode = host) {
      const b = h.button(name, scope);
      act(() => b.click());
    },
    /** Clicks a link by its accessible name: a shallow link moves the URL, as in the browser. */
    link(name: string | RegExp, scope: ParentNode = host) {
      const found = [...scope.querySelectorAll<HTMLAnchorElement>("a")].filter((a) => {
        const label = (a.getAttribute("aria-label") ?? a.textContent ?? "").trim();
        return typeof name === "string" ? label === name : name.test(label);
      });
      expect(found, `link ${String(name)}`).toHaveLength(1);
      act(() => found[0].dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 })));
      return found[0];
    },
    radio(label: string) {
      const input = [...host.querySelectorAll<HTMLLabelElement>("label")].find((l) => l.textContent?.trim() === label)?.querySelector<HTMLInputElement>('input[type="radio"]');
      expect(input, `radio ${label}`).toBeTruthy();
      act(() => input!.click());
    },
    /** Opens a referral by URL (what an inbox row link does). */
    open(id: string, extra = "") {
      h.go(`/app/referrals?${extra ? `${extra}&` : ""}case=${encodeURIComponent(id)}`);
      expect(h.openId()).toBe(id);
    },
    /** How many referrals the full inbox lists under a status filter ("" for all). */
    count(status = ""): number {
      const back = h.url();
      h.go(`/app/referrals?view=inbox${status ? `&status=${status}` : ""}`);
      const range = host.querySelector('[data-testid="page-range"]')?.textContent ?? "";
      h.go(back);
      return Number(/(\d+) referrals?$/.exec(range)?.[1] ?? NaN);
    },
    /** Rows of the full inbox page shown, by case id. */
    rows: () => [...host.querySelectorAll<HTMLAnchorElement>("tbody th a")].map((a) => a.textContent ?? ""),
    /** A case-id search: submits the field, checks the server navigation, then renders the server's filtered props. */
    search(q: string, source: InboxProps["referrals"]) {
      const field = host.querySelector<HTMLInputElement>('input[name="q"]');
      if (field) {
        act(() => {
          Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(field, q);
          field.dispatchEvent(new Event("input", { bubbles: true }));
        });
        act(() => field.form!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));
        const target = push.mock.lastCall![0] as string;
        expect(new URL(target, "http://x").searchParams.get("q") ?? "").toBe(q.trim());
        act(() => history.pushState(null, "", target));
      } else {
        const p = new URLSearchParams(location.search);
        p.delete("case");
        p.delete("page");
        if (q.trim()) p.set("q", q.trim());
        else p.delete("q");
        act(() => history.pushState(null, "", `/app/referrals${p.size ? `?${p}` : ""}`));
      }
      h.render({ referrals: source.filter((r) => r.id.includes(q.trim())), q: q.trim() });
    },
    async settled(cond: () => boolean, ms = 2000) {
      const end = Date.now() + ms;
      while (!cond() && Date.now() < end) await act(async () => void (await new Promise((r) => setTimeout(r, 20))));
      expect(cond()).toBe(true);
    },
  };
  return h;
}
export type Harness = ReturnType<typeof mountInbox>;
