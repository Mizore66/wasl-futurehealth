import { renderToStaticMarkup } from "react-dom/server";
import { Window } from "happy-dom";
import { describe, expect, it, vi } from "vitest";
import Page from "@/app/app/patients/page";
import { parseCaseFilters } from "@/components/worklist/filters";
import { referralDate } from "@/components/patients/calendar";
import { CaseList, type CaseSummary } from "@/lib/api/schema";
import { matches, type CaseQuery } from "@/lib/api/source";
import { loadSample, sampleFiles } from "./samples";

const ALL: CaseSummary[] = (() => {
  const seen = new Map<string, CaseSummary>();
  for (const f of sampleFiles().filter((n) => /^cases_[a-z_]+_page1\.json$/.test(n)))
    for (const c of CaseList.parse(loadSample(f)).cases) seen.set(c.id, c);
  return [...seen.values()];
})();

vi.mock("@/lib/api/source", async (original) => ({
  ...(await original<typeof import("@/lib/api/source")>()),
  getCases: async (query: CaseQuery = {}) => {
    const hits = ALL.filter((c) => matches(c, query));
    const limit = query.limit ?? 100;
    const offset = query.offset ?? 0;
    return { meta: CaseList.parse(loadSample("cases_lung_ldct_page1.json")).meta, cases: hits.slice(offset, offset + limit), total: hits.length, limit, offset };
  },
}));
vi.mock("@/components/worklist/run-clock", () => ({ runClock: async () => 100, hasRecord: () => false }));

type Params = Record<string, string>;

async function render(params: Params): Promise<Document> {
  const element = await Page({ params: Promise.resolve({}), searchParams: Promise.resolve(params) });
  const window = new Window();
  window.document.body.innerHTML = renderToStaticMarkup(element);
  // A browser selects the option the server marked `selected`; happy-dom's innerHTML parse does not carry that into select.value.
  for (const select of window.document.querySelectorAll("select")) {
    const chosen = [...select.options].find((o) => o.hasAttribute("selected"));
    select.value = chosen ? chosen.value : "";
  }
  return window.document as unknown as Document;
}

/** What a browser would navigate to: the form's successful controls, serialised by FormData. */
function submit(doc: Document, form: HTMLFormElement): Params {
  const data = new (doc.defaultView as unknown as { FormData: typeof FormData }).FormData(form);
  const out: Params = {};
  for (const [k, v] of data.entries()) if (typeof v === "string" && v !== "") out[k] = v;
  return out;
}

const searchForm = (doc: Document) => doc.querySelector<HTMLInputElement>('input[name="q"][type="search"]')!.closest("form") as HTMLFormElement;
const refineForm = (doc: Document) => doc.querySelector<HTMLFormElement>('[data-testid="refine"] form')!;
const text = (doc: Document, id: string) => doc.querySelector(`[data-testid="${id}"]`)!.textContent!;

async function expectView(params: Params, closed: boolean) {
  const filters = parseCaseFilters(params);
  expect(filters.closed).toBe(closed);
  const doc = await render(params);
  const expected = ALL.filter((c) => matches(c, { ...(filters.programme && { programme: filters.programme }), ...(filters.provider && { provider: filters.provider }), ...(filters.overdue !== undefined && { overdue: filters.overdue }), ...(filters.review && { review: filters.review }), ...(filters.q && { q: filters.q }), closed }));
  expect(text(doc, "showing")).toContain(`${expected.length} ${expected.length === 1 ? "case" : "cases"}: `);
  expect(text(doc, "showing")).toContain(closed ? "closed" : "not closed");
  if (expected.length === 0) {
    expect(doc.querySelector('[data-testid="month-total"]')).toBeNull();
    return doc;
  }
  // The calendar opens on the latest month holding a match; its files are the cases referred that month.
  const month = expected.map(referralDate).sort().at(-1)!.slice(0, 7);
  const inMonth = expected.filter((c) => referralDate(c).startsWith(month));
  expect(text(doc, "month-total")).toBe(`${inMonth.length} ${inMonth.length === 1 ? "referral" : "referrals"}`);
  const files = await render({ ...params, files: month });
  const folders = [...files.querySelectorAll("[data-pf-case]")];
  expect(folders).toHaveLength(Math.min(inMonth.length, 25));
  for (const f of folders) expect(/Closed day/.test(f.textContent!)).toBe(closed);
  return doc;
}

describe("the Search and Refine GET forms keep the closed filter", () => {
  for (const closed of [true, false]) {
    const base: Params = { closed: String(closed), programme: "lung_ldct", provider: "lcs-centre", overdue: "false", q: "sr-1" };

    it(`Search submission keeps closed=${closed} with the other filters`, async () => {
      const doc = await render(base);
      const form = searchForm(doc);
      form.querySelector<HTMLInputElement>('input[name="q"]')!.value = "sr-11";
      const sent = submit(doc, form);
      expect(sent).toEqual({ ...base, q: "sr-11" });
      await expectView(sent, closed);
    });

    it(`Apply filters submission keeps closed=${closed} with the other filters`, async () => {
      const doc = await render(base);
      const form = refineForm(doc);
      expect(submit(doc, form)).toEqual({ ...base });
      const review = form.querySelector<HTMLSelectElement>('select[name="review"]')!;
      const option = [...review.options].find((o) => o.value !== "")!;
      review.value = option.value;
      const sent = submit(doc, form);
      expect(sent).toEqual({ ...base, review: option.value });
      await expectView(sent, closed);
    });
  }

  it("no closed control is sent when the filter is absent", async () => {
    const doc = await render({ programme: "lung_ldct" });
    expect(submit(doc, searchForm(doc))).toEqual({ programme: "lung_ldct" });
    expect(submit(doc, refineForm(doc))).toEqual({ programme: "lung_ldct" });
  });

  it("the Closed chip stays selected after a Search submission", async () => {
    const doc = await render({ closed: "true", programme: "lung_ldct" });
    const sent = submit(doc, searchForm(doc));
    const after = await render(sent);
    const chip = [...after.querySelectorAll('nav[aria-label="Filters"] a')].find((a) => a.textContent === "Closed")!;
    expect(chip.getAttribute("aria-current")).toBe("true");
  });
});
