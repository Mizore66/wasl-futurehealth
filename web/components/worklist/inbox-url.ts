import { STATUS_FILTERS, type ReferralStatus } from "./referrals";

/**
 * The referral inbox's state in the URL, so a reload, a shared link and "Back to inbox" restore it.
 *  - `view=inbox`: the full, paginated inbox; otherwise the hand-off of one referral.
 *  - `case`: the referral shown (hand-off) or last opened (inbox, where it marks its row and picks its page).
 *  - `from=inbox`: the hand-off was opened from the full inbox, so it offers "Back to inbox".
 *  - `q`: the case-id search (server filtered); `status`: the status filter; `page`: the inbox page.
 * The programme/provider scope (`programme`, `provider`) leads every URL.
 */
export type InboxUrl = { view: "inbox" | "case"; from: boolean; caseId: string | null; status: ReferralStatus | ""; page: number | null; q: string };

type Params = { get(name: string): string | null };

export function readInboxUrl(sp: Params, q = sp.get("q")?.trim() ?? ""): InboxUrl {
  const status = sp.get("status") ?? "";
  const page = Number(sp.get("page"));
  return {
    view: sp.get("view") === "inbox" ? "inbox" : "case",
    from: sp.get("from") === "inbox",
    caseId: sp.get("case") || null,
    status: (STATUS_FILTERS as readonly string[]).includes(status) ? (status as ReferralStatus) : "",
    page: Number.isSafeInteger(page) && page > 0 ? page : null,
    q,
  };
}

/** The URL of an inbox state; defaults are left out so the plain page stays `/app/referrals`. */
export function inboxHref(scope: string, s: Partial<InboxUrl>): string {
  const p = new URLSearchParams(scope);
  if (s.q) p.set("q", s.q);
  if (s.status) p.set("status", s.status);
  if (s.page && s.page > 1) p.set("page", String(s.page));
  if (s.view === "inbox") p.set("view", "inbox");
  if (s.view !== "inbox" && s.from) p.set("from", "inbox");
  if (s.caseId) p.set("case", s.caseId);
  const query = p.toString();
  return query ? `/app/referrals?${query}` : "/app/referrals";
}

/** A referrals URL this page may send the user back to; anything else is dropped. */
export function safeInboxBack(value: string | null | undefined): string | null {
  if (!value) return null;
  return value === "/app/referrals" || value.startsWith("/app/referrals?") ? value : null;
}

/**
 * Moves to another inbox state without a server round trip. Next.js syncs `useSearchParams` with the native history
 * API, so the page re-renders from the new URL and the session decisions held in the inbox survive.
 */
export function shallowGo(href: string, replace = false) {
  const h = globalThis.history;
  if (replace) h.replaceState(null, "", href);
  else h.pushState(null, "", href);
}
