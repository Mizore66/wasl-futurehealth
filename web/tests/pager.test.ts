import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import { Pager, PagerLinks } from "@/components/worklist/pager";
import { paginate } from "@/components/worklist/paging";
import { filtersHref } from "@/components/worklist/filters";
const rows = Array.from({ length: 1201 }, (_, id) => ({ id: String(id) }));
const hrefFor = (page: number) => filtersHref("/app/review", {}, page);
const labels = (html: string, tag: string) => [...html.matchAll(new RegExp(`<${tag}[^>]*aria-label="([^"]+)"`, "g"))].map((m) => m[1]);
it("omits both variants entirely for single-page data", () => {
  const props = { paging: paginate([], 1), label: "Pages", testId: "range" };
  expect(renderToStaticMarkup(createElement(Pager, { ...props, onPage: () => {} }))).toBe("");
  expect(renderToStaticMarkup(createElement(PagerLinks, { ...props, hrefFor }))).toBe("");
});
it("button navigation names its chevrons and pages, marks the current page, disables a chevron at its end, and adds no live region", () => {
  for (const [page, pages, disabled] of [[1, [1, 2, 3, 49], "Previous page"], [20, [1, 20, 49], null], [49, [1, 47, 48, 49], "Next page"]] as const) {
    const html = renderToStaticMarkup(createElement(Pager, { paging: paginate(rows, page), label: "Your list pages", testId: "range", onPage: () => {} }));
    expect(html).toContain('aria-label="Your list pages"'); expect(html).toContain('data-testid="range"');
    expect(labels(html, "button")).toEqual(["Previous page", ...pages.map((n) => `Page ${n}`), "Next page"]);
    expect([...html.matchAll(/<button[^>]*aria-label="([^"]+)"[^>]*disabled=""/g)].map((m) => m[1])).toEqual(disabled ? [disabled] : []);
    expect(html).toMatch(new RegExp(`aria-label="Page ${page}" aria-current="page"`));
    expect(html.match(/aria-current/g)).toHaveLength(1);
    expect(html).toContain(`data-focus="pager-${page}"`); expect(html).toContain('data-focus="pager-prev"'); expect(html).toContain('data-focus="pager-next"');
    expect(html).not.toMatch(/aria-live=|role="status"| aria-disabled=/);
  }
});
it("server links use canonical page hrefs and rel directions, and an incomplete list claims neither a total nor a last page", () => {
  const props = { paging: paginate(rows, 2), label: "Pages above the list", testId: "range", hrefFor };
  const html = renderToStaticMarkup(createElement(PagerLinks, props));
  expect(labels(html, "a")).toEqual(["Previous page", "Page 1", "Page 3", "Page 49", "Next page"]);
  expect(html).toContain('href="/app/review"'); expect(html).toContain('href="/app/review?page=3"');
  expect(html).toContain('rel="prev"'); expect(html).toContain('rel="next"'); expect(html).toContain("Showing 26–50 of 1201");
  expect(html).toMatch(/<span aria-current="page" aria-label="Page 2"/);
  const incomplete = renderToStaticMarkup(createElement(PagerLinks, { ...props, complete: false }));
  expect(incomplete).toContain("Showing 26–50"); expect(incomplete).not.toContain("of 1201");
  expect(labels(incomplete, "a")).toEqual(["Previous page", "Next page"]);
  const first = renderToStaticMarkup(createElement(PagerLinks, { ...props, paging: paginate(rows, 1) }));
  expect(labels(first, "a")).toEqual(["Page 2", "Page 3", "Page 49", "Next page"]);
});
