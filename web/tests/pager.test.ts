import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import { Pager, PagerLinks } from "@/components/worklist/pager";
import { paginate } from "@/components/worklist/paging";
import { filtersHref } from "@/components/worklist/filters";
const rows = Array.from({ length: 1201 }, (_, id) => ({ id: String(id) }));
const hrefFor = (page: number) => filtersHref("/app/review", {}, page);
it("omits both variants entirely for single-page data", () => {
  const props = { paging: paginate([], 1), label: "Pages", testId: "range" };
  expect(renderToStaticMarkup(createElement(Pager, { ...props, onPage: () => {} }))).toBe("");
  expect(renderToStaticMarkup(createElement(PagerLinks, { ...props, hrefFor }))).toBe("");
});
it("button navigation has named landmarks, actual controls and focus hooks, with no duplicate live region", () => {
  for (const [page, names] of [[1, ["Next", "Last"]], [2, ["First", "Previous", "Next", "Last"]], [49, ["First", "Previous"]]] as const) {
    const html = renderToStaticMarkup(createElement(Pager, { paging: paginate(rows, page), label: "Your list pages", testId: "range", onPage: () => {} }));
    expect(html).toContain('aria-label="Your list pages"'); expect(html).toContain('data-testid="range"');
    expect([...html.matchAll(/<button[^>]*>([^<]+)<\/button>/g)].map((m) => m[1])).toEqual(names);
    for (const name of names) expect(html).toContain(`data-focus="pager-${name === "Previous" ? "prev" : name.toLowerCase()}"`);
    expect(html).not.toMatch(/aria-live|role="status"|aria-disabled/);
  }
});
it("server links use canonical page hrefs, rel directions, and never claim an incomplete total or Last", () => {
  const props = { paging: paginate(rows, 2), label: "Pages above the list", testId: "range", hrefFor };
  const html = renderToStaticMarkup(createElement(PagerLinks, props));
  expect(html).toContain('href="/app/review"'); expect(html).toContain('href="/app/review?page=3"');
  expect(html).toContain('rel="prev"'); expect(html).toContain('rel="next"'); expect(html).toContain("Showing 26–50 of 1201");
  const incomplete = renderToStaticMarkup(createElement(PagerLinks, { ...props, complete: false }));
  expect(incomplete).toContain("Showing 26–50"); expect(incomplete).not.toContain("of 1201"); expect(incomplete).not.toContain(">Last<");
});
