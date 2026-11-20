// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { INBOX_PAGE, pageNumbers } from "@/components/worklist/referral-inbox";
import { inboxHref, readInboxUrl, safeInboxBack } from "@/components/worklist/inbox-url";
import { allNewReferrals, syntheticReferrals } from "./paging-fixtures";
import { mountInbox, type Harness } from "./referral-inbox-harness";

// Each test pages through a large list; the 5 s default times out on a loaded CI runner.
vi.setConfig({ testTimeout: 60_000 });
vi.mock("next/navigation", async () => (await import("./referral-nav-mock")).navigationMock);

let h: Harness;
beforeEach(() => {
  h = mountInbox();
});
afterEach(() => h.unmount());

const range = () => h.host.querySelector('[data-testid="page-range"]')!.textContent;
const current = () => h.host.querySelector('nav[aria-label="Inbox pages"] [aria-current="page"]')?.textContent;
const short = (id: string) => id.split("/").pop()!;

it("pages through 1201 referrals: every row exactly once, in waiting order, with the page in the URL", () => {
  const referrals = allNewReferrals(1201);
  h.render({ referrals });
  h.link("View all 1201 referrals");
  expect(h.url()).toContain("view=inbox");
  expect(h.rows()).toHaveLength(INBOX_PAGE);
  expect(range()).toBe(`1–${INBOX_PAGE} of 1201 referrals`);
  const seen = [...h.rows()];
  const pages = Math.ceil(1201 / INBOX_PAGE);
  for (let page = 2; page <= pages; page++) {
    h.link("Next page");
    expect(new URLSearchParams(location.search).get("page")).toBe(String(page));
    expect(current()).toBe(String(page));
    seen.push(...h.rows());
  }
  expect(seen).toEqual([...referrals].sort((a, b) => b.waiting - a.waiting).map((r) => r.short));
  expect(new Set(seen).size).toBe(1201);
  expect(h.host.querySelector('nav[aria-label="Inbox pages"] a[aria-label="Next page"]')).toBeNull();
  h.link("Page 1");
  expect(h.rows()[0]).toBe(seen[0]);
  // The page holds no scroll box of its own: the page grows and pages, it does not scroll inside.
  expect(h.host.querySelector('[class*="overflow-y-auto"], [class*="overflow-auto"]')).toBeNull();
});

it("renders one page of a 5000-referral queue and reaches any page from the URL", () => {
  const referrals = allNewReferrals(5000);
  h.render({ referrals });
  h.go("/app/referrals?view=inbox&page=377");
  expect(h.rows()).toHaveLength(INBOX_PAGE);
  expect(range()).toBe("3761–3770 of 5000 referrals");
  expect(pageNumbers(377, 500)).toEqual([1, 376, 377, 378, 500]);
  h.go("/app/referrals?view=inbox&page=9999");
  expect(current()).toBe("500");
});

it("restores the page, search, status and the opened row when going back to the inbox", () => {
  const referrals = syntheticReferrals(1201);
  h.render({ referrals });
  h.go("/app/referrals?view=inbox&status=needs&page=3");
  const opened = h.rows()[4];
  const needs = Number(/of (\d+) referrals/.exec(range()!)![1]);
  expect(needs).toBe(referrals.filter((r) => r.state === "new").length);
  h.link(opened);
  expect(h.url()).toBe(`/app/referrals?status=needs&page=3&from=inbox&case=${encodeURIComponent(referrals.find((r) => r.short === opened)!.id)}`);
  expect(h.pane()).not.toBeNull();
  expect(short(h.openId()!)).toBe(opened);
  // The hand-off previews the three rows around the open one, and links back to the full inbox.
  const preview = [...h.host.querySelectorAll<HTMLAnchorElement>('section[aria-label="Referral inbox"] li a')];
  expect(preview).toHaveLength(3);
  expect(preview.filter((a) => a.getAttribute("aria-current") === "true").map((a) => a.textContent)).toEqual([expect.stringContaining(opened)]);

  h.link("Back to inbox");
  expect(current()).toBe("3");
  expect(h.host.querySelector('[aria-haspopup="listbox"]')!.textContent).toBe("Status: Needs response");
  const marked = h.host.querySelector('tbody a[aria-current="true"]')!;
  expect(marked.textContent).toBe(opened);
  expect(document.activeElement).toBe(marked);
});

it("a decision made from page 2 moves the referral between statuses and survives the way back", () => {
  const referrals = allNewReferrals(30);
  h.render({ referrals, window: 7 });
  h.go("/app/referrals?view=inbox&page=2");
  const opened = h.rows()[0];
  h.link(opened);
  h.click("Accept referral");
  expect(h.paneText()).toContain("Accepted. Your unit books her now.");
  h.link("Back to inbox");
  expect(current()).toBe("2");
  expect(h.count("needs")).toBe(29);
  expect(h.count("accepted")).toBe(1);
  h.go("/app/referrals?view=inbox&status=accepted");
  expect(h.rows()).toEqual([opened]);
});

it("a filtered-out page says nothing matches and offers to clear and says when nothing matches", () => {
  h.render({ referrals: allNewReferrals(25) });
  h.go("/app/referrals?view=inbox&page=3");
  expect(current()).toBe("3");
  h.go("/app/referrals?view=inbox&page=3&status=accepted");
  expect(h.text()).toContain("No matching referrals");
  const clear = [...h.host.querySelectorAll("a")].find((a) => a.textContent === "Clear filters")!;
  expect(clear.getAttribute("href")).toBe("/app/referrals?view=inbox");
});

it("reads and writes the inbox URL, keeping the programme scope first and refusing foreign back links", () => {
  const scope = "programme=lung_nodule&provider=rad-imaging";
  expect(inboxHref(scope, { q: "91", status: "waiting", page: 2, view: "inbox", caseId: "a/b" })).toBe(`/app/referrals?${scope}&q=91&status=waiting&page=2&view=inbox&case=a%2Fb`);
  expect(inboxHref("", {})).toBe("/app/referrals");
  expect(inboxHref("", { caseId: "x", from: true })).toBe("/app/referrals?from=inbox&case=x");
  const read = readInboxUrl(new URLSearchParams("view=inbox&status=bogus&page=-2&case=x"));
  expect(read).toEqual({ view: "inbox", from: false, caseId: "x", status: "", page: null, q: "" });
  expect(safeInboxBack("https://evil.example/app/referrals")).toBeNull();
  expect(safeInboxBack("/app/referrals?view=inbox")).toBe("/app/referrals?view=inbox");
});
