import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, expect, it, vi } from "vitest";

// Each test renders dozens of pages of a large list; the 5 s default times out on a loaded CI runner.
vi.setConfig({ testTimeout: 30_000 });
import { ReviewState } from "@/lib/api/schema";
import { caseHref } from "@/lib/domain/ids";
import { questionAnchor } from "@/components/review/decision";
import { syntheticReviewDetails } from "./paging-fixtures";

const queue = vi.hoisted(() => ({ reviewQueue: vi.fn() }));
vi.mock("@/components/review/queue", () => queue);
vi.mock("@/lib/api/source", () => ({ apiMode: () => "live" }));
vi.mock("@/lib/api/reviewer-session", () => ({ reviewerToken: async () => "synthetic-viewer-token" }));
vi.mock("next/navigation", () => ({ redirect: (url: string) => { throw new Error(`TEST_REDIRECT:${url}`); }, usePathname: () => "/app/review" }));
afterEach(() => vi.clearAllMocks());
const props = (query: Record<string, string> = {}) => ({ params: Promise.resolve({}), searchParams: Promise.resolve(query) });
async function render(query: Record<string, string> = {}) {
  const { default: Page } = await import("@/app/app/review/page");
  return renderToStaticMarkup(await Page(props(query)));
}
const lis = (html: string) => html.match(/<li\b/g) ?? [];
const rowHrefs = (html: string) => [...html.matchAll(/href="(\/app\/review\/[^"#]+)"/g)].map((m) => m[1]);

it("mounts at most 25 of 1201 review rows but retains the complete waiting count", async () => {
  queue.reviewQueue.mockResolvedValue({ source: "open", cases: syntheticReviewDetails(1201) });
  const html = await render();
  expect(lis(html)).toHaveLength(25);
  expect(html).toContain("1201 cases wait");
  expect(html).toContain('aria-label="Pages above the list"'); expect(html).toContain('aria-label="Pages below the list"');
});

it("all 49 review pages concatenate in queue/oldest-first order, question links are bounded and case-bound", async () => {
  const cases = syntheticReviewDetails(1201).map((c) => ({ ...c, api_review_states: [ReviewState.parse({
    item: "synthetic-item", question: "evidence", state: "awaiting_r1", head: "", next_read: "r1", may_submit: false,
    reads: [], disagreement: false, canonical_unchanged: true, frozen: false,
    claim_actions: { claim: true, renew: false, release: false, takeover: false },
  })] }));
  queue.reviewQueue.mockResolvedValue({ source: "open", cases });
  const seen: string[] = [];
  for (let page = 1; page <= 49; page++) {
    const html = await render({ page: String(page) });
    expect(lis(html)).toHaveLength(page === 49 ? 1 : 25);
    const expected = cases.slice((page - 1) * 25, page * 25);
    expect(rowHrefs(html)).toEqual(expected.map((c) => caseHref("/app/review", c.case.id)));
    seen.push(...rowHrefs(html));
    expect(html.match(/data-queue-question=/g)).toHaveLength(expected.length);
    for (const c of expected) expect(html).toContain(`${caseHref("/app/review", c.case.id)}#${questionAnchor("synthetic-item", "evidence")}`);
    expect(html).toContain("1201 cases wait");
  }
  expect(seen).toEqual(cases.map((c) => caseHref("/app/review", c.case.id)));
});

it("incomplete review is fail-closed, paged without total/page-count/Last or a practice fallback", async () => {
  queue.reviewQueue.mockResolvedValue({ source: "incomplete", cases: syntheticReviewDetails(40), failures: [{ id: "synthetic-unreadable", status: 403 }] });
  const html = await render({ page: "2" });
  expect(lis(html)).toHaveLength(15); expect(html).toContain('role="alert"'); expect(html).toContain("queue is incomplete");
  expect(html).toContain("Showing 26–40"); expect(html).not.toContain("of 40"); expect(html).not.toContain("Page 2 of");
  expect(html).not.toContain(">Last<"); expect(html).not.toMatch(/cases? waits?/); expect(html).not.toContain("Practice queue");
  expect(queue.reviewQueue).toHaveBeenCalledExactlyOnceWith("synthetic-viewer-token");
  queue.reviewQueue.mockResolvedValue({ source: "incomplete", cases: syntheticReviewDetails(60), failures: [{ id: "synthetic-unreadable", status: 401 }] });
  expect(await render()).not.toContain(">Last<");
});

it("redirects over-range complete and empty incomplete pages, normalizes malformed and zero pages", async () => {
  queue.reviewQueue.mockResolvedValue({ source: "open", cases: syntheticReviewDetails(1201) });
  await expect(render({ page: "99" })).rejects.toThrow("TEST_REDIRECT:/app/review?page=49");
  for (const page of ["abc", "0"]) expect(rowHrefs(await render({ page }))[0]).toBe(caseHref("/app/review", syntheticReviewDetails(1)[0].case.id));
  queue.reviewQueue.mockResolvedValue({ source: "incomplete", cases: [], failures: [{ id: "synthetic-unreadable", status: 404 }] });
  await expect(render({ page: "3" })).rejects.toThrow("TEST_REDIRECT:/app/review");
});

it("metadata reads only searchParams, fetches nothing, and page rendering fetches once", async () => {
  const { generateMetadata } = await import("@/app/app/review/page");
  for (const [page, title] of [["3", "Review, page 3"], ["1", "Review"], ["abc", "Review"]]) {
    expect(await generateMetadata(props({ page }))).toEqual({ title });
  }
  expect(await generateMetadata(props())).toEqual({ title: "Review" });
  expect(queue.reviewQueue).not.toHaveBeenCalled();
  queue.reviewQueue.mockResolvedValue({ source: "open", cases: syntheticReviewDetails(40) });
  const html = await render({ page: "2", oidc_error: "1" });
  expect(queue.reviewQueue).toHaveBeenCalledTimes(1);
  expect(html.match(/rel="prev"/g)).toHaveLength(2);
  expect(html).not.toContain("oidc_error=1");
  expect(html).toContain('href="/app/review"');
});

it("rebuilt single-case full-row guard semantics survive without a pager", async () => {
  const cases = syntheticReviewDetails(1);
  queue.reviewQueue.mockResolvedValue({ source: "rebuilt", cases });
  const html = await render();
  expect(lis(html)).toHaveLength(1); expect(html).toContain("1 case waits");
  expect(html).toContain("Practice queue"); expect(html).toContain("no open review items");
  expect(html).toContain(`using its ${cases[0].review_items.length} review items`);
  const guard = cases[0].decisions.some((d) => Array.isArray(d.outreach) && d.outreach.some((o: { suppress?: string }) => o.suppress === "second_identifier_unconfirmed")) && cases[0].decisions.every((d) => d.consent_log.length === 0);
  expect(html.includes("second-identifier guard")).toBe(guard);
  expect(html).not.toContain("<nav");
});
