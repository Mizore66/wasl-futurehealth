import { readFileSync } from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps, ReactElement } from "react";
import PatientsPage from "@/app/app/patients/page";
import { referralDate } from "@/components/patients/calendar";
import TodayPage from "@/app/app/today/page";
import { statusWords, workItem } from "@/components/worklist/model";
import { isActionable, ldctChildren, obligationWords, openObligation } from "@/components/worklist/obligation";
import { TodayBoard } from "@/components/worklist/today-board";
import { CaseList, DrFinding, type CaseSummary } from "@/lib/api/schema";
import { matches, type CaseQuery } from "@/lib/api/source";
import { formatDay } from "@/lib/domain/day";

const page = (p: string) => JSON.parse(readFileSync(path.join(__dirname, "..", "data", "api", `cases_${p}_page1.json`), "utf8"));
const parse = (raw: object, cases: object[]): CaseSummary[] => CaseList.parse({ ...raw, cases }).cases;
const rawDr = page("diabetic_retinopathy");
const rawLdct = page("lung_ldct");
type Raw = { id: string; dr?: object; ldct?: object };
const find = (raw: { cases: Raw[] }, id: string) => raw.cases.find((c) => c.id.endsWith(id))!;

// TestDRNextRecallOverdueAndSeen: R2M0 screen day 10, eye clinic day 20 says review in 91 days, due day 111.
const review = { kind: "clinic_review", from_day: 20, interval_days: 91, due_day: 111, basis: "note_review_interval" };
const drCase = (nextDue: object | undefined): CaseSummary => {
  const base = find(rawDr, "/PHC-sr-10");
  return parse(rawDr, [{ ...base, state: "closed", overdue: false, dropoff: "", owner: "Practitioner/ahs-mushrif-nurse-5", dr: { ...base.dr, next_due: nextDue } }])[0];
};

// TestLDCTFollowupLoops: 4A then a follow-up LDCT read LR3 closes the parent and raises followup_needs_tracking.
const child = parse(rawLdct, [find(rawLdct, "/LCS-sr-109")])[0];
const parent = (() => {
  const base = find(rawLdct, "/LCS-sr-11");
  const ldct = { ...base.ldct, scan_label: "LR4A", outcome: "followup_done", followup_report: child.ldct!.scan_report, followup_label: "LR3", followup_next_step: "LDCT in 6 months" };
  return parse(rawLdct, [{ ...base, state: "closed", closed: true, ldct }])[0];
})();

describe("the DR next-due contract", () => {
  it("keeps recall_overdue and the seen fields the API serves", () => {
    const dr = DrFinding.parse({ next_due: { ...review, recall_overdue: true, seen: true, seen_day: 108, seen_report: "EYE-dr-2" } });
    expect(dr.next_due).toMatchObject({ recall_overdue: true, seen: true, seen_day: 108, seen_report: "EYE-dr-2", from_day: 20, interval_days: 91 });
  });
});

describe("DR recall after closure", () => {
  it("day 120: the day-111 clinic review is an overdue obligation while the episode stays closed", () => {
    const c = drCase({ ...review, recall_overdue: true });
    const ob = openObligation(c, 120);
    expect(ob).toEqual({ kind: "dr_recall", state: "overdue", contact: "clinic_review", fromDay: 20, dueDay: 111 });
    expect(obligationWords(ob!)).toEqual({ status: `Closed · eye-clinic review overdue since ${formatDay(111)}`, next: "Arrange the eye-clinic review" });
    expect([c.closed, c.state, c.overdue, c.dropoff]).toEqual([true, "closed", false, ""]);
    const it = workItem(c, 120, true);
    expect([it.view, it.status, it.next, it.days, it.deadline, it.overdue]).toEqual(["call", obligationWords(ob!).status, "Arrange the eye-clinic review", 100, 91, true]);
  });

  it("the run clock alone makes it overdue when the decision predates the due day", () => {
    expect(openObligation(drCase(review), 112)).toMatchObject({ state: "overdue" });
    expect(openObligation(drCase(review), 111.5)).toMatchObject({ state: "due" });
  });

  it("day 110: the same obligation is upcoming and not in the queue", () => {
    const ob = openObligation(drCase(review), 110);
    expect(ob).toMatchObject({ kind: "dr_recall", state: "upcoming" });
    expect(isActionable(ob)).toBe(false);
    expect(workItem(drCase(review), 110, true).status).toBe("Closed");
  });

  it("a qualifying later visit clears it; the episode closure is unchanged", () => {
    const c = drCase({ ...review, seen: true, seen_day: 108, seen_report: "EYE-dr-2" });
    expect(openObligation(c, 120)).toBeNull();
    expect(statusWords(c)).toEqual({ status: "Closed", next: "Nothing to do" });
  });

  it("with -drnext off (no next_due) a closed DR case has no obligation", () => {
    const c = drCase(undefined);
    expect(openObligation(c, 400)).toBeNull();
    expect(workItem(c, 400, true)).toMatchObject({ status: "Closed", next: "Nothing to do", overdue: false });
  });
});

describe("LDCT follow-up tracked by its own child loop", () => {
  it("links the closed parent to the child tracking request and puts no task on the parent", () => {
    const ob = openObligation(parent, 120, ldctChildren([parent, child]));
    expect(ob).toEqual({ kind: "ldct_tracking", followupReport: child.ldct!.scan_report, child: child.id });
    expect(isActionable(ob)).toBe(false);
    expect(openObligation(child, 120, ldctChildren([parent, child]))).toBeNull();
  });

  it("without a generated child the parent says a tracking request is needed", () => {
    expect(obligationWords(openObligation(parent, 120)!).next).toBe("Open a tracking request for the follow-up scan");
  });

  it("a closed case with no next-due or tracking obligation stays non-actionable", () => {
    const plain = parse(rawLdct, [find(rawLdct, "/LCS-sr-11")])[0];
    expect(openObligation(plain, 120)).toBeNull();
  });
});

const mocks = vi.hoisted(() => ({ all: vi.fn(), cases: [] as CaseSummary[] }));
vi.mock("@/lib/api/all-cases", () => ({ getAllCases: mocks.all }));
vi.mock("@/components/worklist/run-clock", () => ({ runClock: async () => 120, hasRecord: () => false }));
// The Patients filter menus and search are client components; static rendering has no app router to hand them.
vi.mock("next/navigation", async (original) => ({ ...(await original<typeof import("next/navigation")>()), useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/lib/api/source", async (original) => ({
  ...(await original<typeof import("@/lib/api/source")>()),
  getCases: async (query: CaseQuery = {}) => {
    const hits = mocks.cases.filter((c) => matches(c, query));
    const limit = query.limit ?? 100;
    const offset = query.offset ?? 0;
    return { meta: CaseList.parse(rawLdct).meta, cases: hits.slice(offset, offset + limit), total: hits.length, limit, offset };
  },
}));

const overdueDr = drCase({ ...review, recall_overdue: true });
const upcomingDr = parse(rawDr, [{ ...find(rawDr, "/PHC-sr-100"), state: "closed", owner: "Practitioner/ahs-mushrif-nurse-5", dr: { next_due: { ...review, due_day: 300 } } }])[0];
const seenDr = parse(rawDr, [{ ...find(rawDr, "/PHC-sr-1000"), state: "closed", dr: { next_due: { ...review, seen: true, seen_day: 108 } } }])[0];

beforeEach(() => {
  vi.clearAllMocks();
  mocks.cases = [overdueDr, upcomingDr, seenDr, parent, child];
  mocks.all.mockImplementation(async (q: CaseQuery = {}) => ({ cases: mocks.cases.filter((c) => matches(c, q)) }));
});

async function today(sp: Record<string, string>) {
  // Server component unit test with mocked reads; not served HTTP or browser evidence.
  // The page returns the board itself; its header is the board's `head` prop.
  const board = (await TodayPage({ params: Promise.resolve({}), searchParams: Promise.resolve(sp) })) as ReactElement<ComponentProps<typeof TodayBoard>>;
  return board.props;
}

describe("Today worklist, deliberately scoped to open cases and due recalls", () => {
  it("reads closed DR cases too, and queues only the overdue DR recall", async () => {
    const board = await today({ programme: "diabetic_retinopathy", provider: "" });
    expect(mocks.all.mock.calls).toEqual([[{ closed: false }], [{ closed: true, programme: "diabetic_retinopathy" }]]);
    expect(board.items.map((i) => i.id)).toEqual([overdueDr.id]);
    expect(board.items[0]).toMatchObject({ view: "call", next: "Arrange the eye-clinic review", deadline: 91 });
  });

  it("shows the LDCT child exactly once and never the closed parent", async () => {
    const board = await today({ programme: "lung_ldct", provider: "" });
    expect(mocks.all.mock.calls).toEqual([[{ closed: false }]]);
    expect(board.items.map((i) => i.id)).toEqual([child.id]);
  });
});

describe("Patients", () => {
  /** Every case's folder, read from the case files of each month the cases were referred in. */
  async function rows(): Promise<Map<string, { text: string; ob: string }>> {
    const text = (html: string) => html.replace(/<[^>]+>/g, "").replace(/&amp;/g, "&");
    const out = new Map<string, { text: string; ob: string }>();
    for (const month of new Set(mocks.cases.map((c) => referralDate(c).slice(0, 7)))) {
      const element = await PatientsPage({ params: Promise.resolve({}), searchParams: Promise.resolve({ files: month }) });
      const body = renderToStaticMarkup(element).split('aria-label="Case files"')[1].split("</ul>")[0];
      for (const li of body.split("<li").slice(1))
        out.set(/[A-Z]+-sr-\d+/.exec(text(li))![0], { text: text(li), ob: text(/data-testid="obligation">(.*?)<\/span>/.exec(li)?.[1] ?? "") });
    }
    return out;
  }
  const row = (all: Map<string, { text: string; ob: string }>, id: string) => all.get(id.slice(id.lastIndexOf("/") + 1))!;

  it("keeps closed episodes closed and shows each one's open obligation", async () => {
    const all = await rows();
    expect(row(all, overdueDr.id).text).toContain("Closed day");
    expect(row(all, overdueDr.id).text).toContain(`Clinic review ${formatDay(111)}`);
    expect(row(all, parent.id).text).toContain("Closed day");
    expect(row(all, overdueDr.id).ob).toBe(`eye-clinic review overdue since ${formatDay(111)} · Arrange the eye-clinic review`);
    expect(row(all, upcomingDr.id).ob).toBe(`eye-clinic review due ${formatDay(300)} · Nothing until ${formatDay(300)}`);
    expect(row(all, seenDr.id).ob).toBe("");
    expect(row(all, parent.id).ob).toBe(`Follow-up tracked on ${child.id.slice(child.id.lastIndexOf("/") + 1)}`);
    expect(row(all, child.id).ob).toBe("");
  });
});
